package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	SignalType  string
	Status      string
	Summary     string
	SignalKey   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (db *DB) CreateTask(ctx context.Context, t *Task) error {
	if t.Status == "" {
		t.Status = "pending"
	}
	row := db.pool.QueryRow(ctx, `
		INSERT INTO tasks (workspace_id, signal_type, status, summary, signal_key)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`,
		t.WorkspaceID, t.SignalType, t.Status, t.Summary, t.SignalKey,
	)
	return row.Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (db *DB) UpdateTaskStatus(ctx context.Context, id uuid.UUID, status string) error {
	tag, err := db.pool.Exec(ctx, `
		UPDATE tasks SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("updating task status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("task %s not found", id)
	}
	return nil
}

func (db *DB) ListTasks(ctx context.Context, workspaceID uuid.UUID, limit int) ([]*Task, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, workspace_id, signal_type, status, summary, signal_key, created_at, updated_at
		FROM tasks
		WHERE workspace_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.SignalType, &t.Status,
			&t.Summary, &t.SignalKey, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// TaskExistsBySignalKey reports whether a task with key exists in the workspace.
// Empty keys are intentionally never deduplicated.
func (db *DB) TaskExistsBySignalKey(ctx context.Context, workspaceID uuid.UUID, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	var exists bool
	err := db.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM tasks WHERE workspace_id = $1 AND signal_key = $2)`, workspaceID, key).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking task signal key: %w", err)
	}
	return exists, nil
}

// AppendTaskEvent records a single event for a task in the task_events table.
// payload is marshaled to JSONB.
func (db *DB) AppendTaskEvent(ctx context.Context, taskID uuid.UUID, eventType string, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling event payload: %w", err)
	}
	_, err = db.pool.Exec(ctx, `
		INSERT INTO task_events (task_id, type, payload)
		VALUES ($1, $2, $3)`,
		taskID, eventType, data,
	)
	if err != nil {
		return fmt.Errorf("appending task event: %w", err)
	}
	return nil
}

// SumWorkspaceTokensSince returns the total tokens recorded in "usage"
// task_events for the workspace since the given time. Used by the daily
// budget cap. Returns 0 when there are no matching events.
func (db *DB) SumWorkspaceTokensSince(ctx context.Context, workspaceID uuid.UUID, since time.Time) (int, error) {
	const q = `
		SELECT
			COALESCE((
				SELECT SUM((te.payload->>'total')::bigint)
				FROM task_events te
				JOIN tasks t ON t.id = te.task_id
				WHERE t.workspace_id = $1
				  AND te.type = 'usage'
				  AND te.created_at >= $2
			), 0)
			+
			COALESCE((
				SELECT SUM(COALESCE(aru.input_tokens, 0) + COALESCE(aru.output_tokens, 0))
				FROM agent_request_usage aru
				JOIN tasks t ON t.id = aru.task_id
				WHERE t.workspace_id = $1
				  AND aru.source = 'reported'
				  AND aru.created_at >= $2
			), 0)`
	var sum int64
	if err := db.pool.QueryRow(ctx, q, workspaceID, since).Scan(&sum); err != nil {
		return 0, fmt.Errorf("summing workspace tokens: %w", err)
	}
	return int(sum), nil
}
