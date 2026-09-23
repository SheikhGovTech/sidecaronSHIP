package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AgentTraceEvent struct {
	ID         uuid.UUID
	TaskID     uuid.UUID
	TraceID    uuid.UUID
	Role       string
	Attempt    int
	Sequence   int64
	EventType  string
	ToolCallID string
	ToolName   string
	Payload    map[string]any
	CreatedAt  time.Time
}

type AgentRequestUsage struct {
	TaskID                   uuid.UUID
	TraceID                  uuid.UUID
	RequestID                string
	Role                     string
	Model                    string
	Category                 string
	Status                   string
	Source                   string
	InputTokens              *int64
	OutputTokens             *int64
	CacheCreationInputTokens *int64
	CacheReadInputTokens     *int64
}

// AgentTraceQuery selects one bounded page of sanitized trace events. A
// sequence cursor is meaningful only within a single trace and is rejected
// unless TraceID is set.
type AgentTraceQuery struct {
	TaskID        uuid.UUID
	TraceID       *uuid.UUID
	Role          string
	AfterSequence int64
	Limit         int
}

func validAgentRole(role string) bool {
	return role == "coding" || role == "evaluator" || role == "reviewer"
}

func (db *DB) AppendAgentTraceEvent(ctx context.Context, event *AgentTraceEvent) error {
	return db.AppendAgentTraceEvents(ctx, []*AgentTraceEvent{event})
}

// AppendAgentTraceEvents writes a bounded collector batch atomically.
func (db *DB) AppendAgentTraceEvents(ctx context.Context, events []*AgentTraceEvent) error {
	if len(events) == 0 {
		return nil
	}
	if len(events) > 256 {
		return fmt.Errorf("agent trace batch exceeds 256 events")
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning agent trace batch: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck -- no-op after commit
	for _, event := range events {
		if event == nil {
			return fmt.Errorf("agent trace event is nil")
		}
		if !validAgentRole(event.Role) {
			return fmt.Errorf("invalid agent trace role %q", event.Role)
		}
		data, err := json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("marshaling agent trace payload: %w", err)
		}
		if event.ID == uuid.Nil {
			event.ID = uuid.New()
		}
		if event.Attempt == 0 {
			event.Attempt = 1
		}
		_, err = tx.Exec(ctx, `
		INSERT INTO agent_trace_events
			(id, task_id, trace_id, role, attempt, sequence, event_type, tool_call_id, tool_name, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10)`,
			event.ID, event.TaskID, event.TraceID, event.Role, event.Attempt,
			event.Sequence, event.EventType, event.ToolCallID, event.ToolName, data)
		if err != nil {
			return fmt.Errorf("appending agent trace event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing agent trace batch: %w", err)
	}
	return nil
}

func (db *DB) ListAgentTraceEvents(ctx context.Context, taskID uuid.UUID, limit int) ([]*AgentTraceEvent, error) {
	return db.ListAgentTraceEventsPage(ctx, AgentTraceQuery{TaskID: taskID, Limit: limit})
}

// ListAgentTraceEventsPage returns a bounded, ordered trace page under one
// task. Payloads have already crossed the collector's sanitization boundary;
// this API never has access to raw provider events.
func (db *DB) ListAgentTraceEventsPage(ctx context.Context, query AgentTraceQuery) ([]*AgentTraceEvent, error) {
	if query.TaskID == uuid.Nil {
		return nil, fmt.Errorf("task ID is required")
	}
	if query.Role != "" && !validAgentRole(query.Role) {
		return nil, fmt.Errorf("invalid agent trace role %q", query.Role)
	}
	if query.AfterSequence > 0 && query.TraceID == nil {
		return nil, fmt.Errorf("trace ID is required with a sequence cursor")
	}
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := db.pool.Query(ctx, `
		SELECT id, task_id, trace_id, role, attempt, sequence, event_type,
		       COALESCE(tool_call_id,''), COALESCE(tool_name,''), payload, created_at
		FROM agent_trace_events
		WHERE task_id=$1
		  AND ($2::uuid IS NULL OR trace_id=$2)
		  AND ($3='' OR role=$3)
		  AND ($4=0 OR sequence>$4)
		ORDER BY created_at, trace_id, sequence LIMIT $5`, query.TaskID, query.TraceID, query.Role, query.AfterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("listing agent trace events: %w", err)
	}
	defer rows.Close()
	var result []*AgentTraceEvent
	for rows.Next() {
		e := &AgentTraceEvent{}
		var payload []byte
		if err := rows.Scan(&e.ID, &e.TaskID, &e.TraceID, &e.Role, &e.Attempt,
			&e.Sequence, &e.EventType, &e.ToolCallID, &e.ToolName, &payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &e.Payload); err != nil {
			return nil, fmt.Errorf("decoding agent trace payload: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// RecordAgentRequestUsage inserts one provider attempt exactly once.
func (db *DB) RecordAgentRequestUsage(ctx context.Context, usage AgentRequestUsage) (bool, error) {
	tag, err := db.pool.Exec(ctx, `
		INSERT INTO agent_request_usage
			(task_id, trace_id, request_id, role, model, category, status, source,
			 input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (task_id, request_id) DO NOTHING`,
		usage.TaskID, usage.TraceID, usage.RequestID, usage.Role, usage.Model,
		usage.Category, usage.Status, usage.Source, usage.InputTokens,
		usage.OutputTokens, usage.CacheCreationInputTokens, usage.CacheReadInputTokens)
	if err != nil {
		return false, fmt.Errorf("recording agent request usage: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (db *DB) DeleteAgentTraceEventsBefore(ctx context.Context, workspaceID uuid.UUID, before time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	tag, err := db.pool.Exec(ctx, `
		DELETE FROM agent_trace_events WHERE id IN (
			SELECT ate.id FROM agent_trace_events ate
			JOIN tasks t ON t.id = ate.task_id
			WHERE t.workspace_id = $1 AND ate.created_at < $2
			ORDER BY ate.created_at LIMIT $3
		)`, workspaceID, before, limit)
	if err != nil {
		return 0, fmt.Errorf("deleting expired agent traces: %w", err)
	}
	return tag.RowsAffected(), nil
}
