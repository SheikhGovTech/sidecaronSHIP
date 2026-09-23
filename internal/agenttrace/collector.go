package agenttrace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/store"
)

type Sink interface {
	AppendAgentTraceEvent(context.Context, *store.AgentTraceEvent) error
	RecordAgentRequestUsage(context.Context, store.AgentRequestUsage) (bool, error)
}

type toolStart struct {
	started         time.Time
	requestSequence int
}

type Summary struct {
	TraceID         uuid.UUID
	Enabled         bool
	Role            string
	Attempt         int
	Requests        int
	InputTokens     int
	OutputTokens    int
	ToolCalls       int
	RepeatedTools   map[string]int
	Terminal        string
	TerminalError   string
	Text            string
	TextTruncated   bool
	PersistenceFail error
}

type Collector struct {
	mu        sync.Mutex
	sink      Sink
	taskID    uuid.UUID
	traceID   uuid.UUID
	role      string
	attempt   int
	workspace string
	cfg       config.AgentTraceConfig
	secrets   []string
	sequence  int64
	request   int
	tools     map[string]toolStart
	repeats   map[string]int
	text      strings.Builder
	summary   Summary
}

func New(sink Sink, taskID uuid.UUID, role string, attempt int, workspace string, cfg config.AgentTraceConfig, secrets ...string) *Collector {
	if attempt <= 0 {
		attempt = 1
	}
	traceID := uuid.New()
	return &Collector{
		sink: sink, taskID: taskID, traceID: traceID, role: role, attempt: attempt,
		workspace: workspace, cfg: cfg, secrets: nonEmpty(secrets),
		tools: map[string]toolStart{}, repeats: map[string]int{},
		summary: Summary{TraceID: traceID, Enabled: cfg.Enabled == nil || *cfg.Enabled, Role: role, Attempt: attempt, RepeatedTools: map[string]int{}},
	}
}

func (c *Collector) Handle(ctx context.Context, ev runtime.AgentEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.summary.PersistenceFail != nil {
		return c.summary.PersistenceFail
	}
	if !c.summary.Enabled {
		if ev.Type == runtime.EventRequestUsage {
			return c.requestUsage(ctx, ev.RequestUsage)
		}
		return nil
	}

	switch ev.Type {
	case runtime.EventTextDelta:
		if c.cfg.CaptureAssistantText != "none" {
			c.text.WriteString(ev.Text)
		}
		return nil
	case runtime.EventToolCallReady:
		return c.toolCall(ctx, ev)
	case runtime.EventToolResult:
		return c.toolResult(ctx, ev)
	case runtime.EventRequestUsage:
		return c.requestUsage(ctx, ev.RequestUsage)
	case runtime.EventCompactionStart:
		return c.append(ctx, "compaction_started", "", "", map[string]any{})
	case runtime.EventCompactionDone, runtime.EventCompactionSkipped:
		payload := map[string]any{}
		if ev.Compaction != nil {
			payload = map[string]any{
				"compacted": ev.Compaction.Compacted, "reason": string(ev.Compaction.Reason),
				"skipped": ev.Compaction.Skipped, "turns_compacted": ev.Compaction.TurnsCompacted,
				"tokens_before": ev.Compaction.TokensBefore, "tokens_after": ev.Compaction.TokensAfter,
				"duration_ms": ev.Compaction.DurationMs,
			}
		}
		kind := "compaction_completed"
		if ev.Type == runtime.EventCompactionSkipped {
			kind = "compaction_skipped"
		}
		return c.append(ctx, kind, "", "", payload)
	case runtime.EventDone:
		if err := c.flushText(ctx); err != nil {
			return err
		}
		c.summary.Terminal = "completed"
		return c.append(ctx, "agent_completed", "", "", aggregateUsage(ev.Usage))
	case runtime.EventAborted:
		if err := c.flushText(ctx); err != nil {
			return err
		}
		c.summary.Terminal = "aborted"
		return c.append(ctx, "agent_aborted", "", "", map[string]any{})
	case runtime.EventError:
		if err := c.flushText(ctx); err != nil {
			return err
		}
		message := ""
		if ev.Error != nil {
			message = c.redact(ev.Error.Error())
		}
		c.summary.Terminal, c.summary.TerminalError = "error", message
		return c.append(ctx, "agent_error", "", "", map[string]any{"category": errorCategory(message), "message": message})
	default:
		return nil
	}
}

func (c *Collector) toolCall(ctx context.Context, ev runtime.AgentEvent) error {
	if ev.ToolCall == nil {
		return nil
	}
	c.tools[ev.ToolCall.ID] = toolStart{started: time.Now(), requestSequence: c.request + 1}
	c.summary.ToolCalls++
	hash := hashBytes(ev.ToolCall.Input)
	c.repeats[ev.ToolCall.Name+":"+hash]++
	c.summary.RepeatedTools[ev.ToolCall.Name+":"+hash] = c.repeats[ev.ToolCall.Name+":"+hash]
	if c.cfg.Enabled != nil && !*c.cfg.Enabled {
		return nil
	}
	payload := map[string]any{"request_sequence": c.request + 1, "arguments_sha256": hash, "argument_bytes": len(ev.ToolCall.Input)}
	if c.cfg.CaptureToolArguments == "sanitized" {
		var value any
		if err := json.Unmarshal(ev.ToolCall.Input, &value); err != nil {
			payload["arguments_parse_error"] = true
		} else {
			payload["arguments"] = c.sanitizeToolArguments(value)
		}
	}
	return c.append(ctx, "tool_call", ev.ToolCall.ID, ev.ToolCall.Name, payload)
}

func (c *Collector) toolResult(ctx context.Context, ev runtime.AgentEvent) error {
	if ev.ToolCall == nil {
		return nil
	}
	start := c.tools[ev.ToolCall.ID]
	delete(c.tools, ev.ToolCall.ID)
	duration := int64(0)
	if !start.started.IsZero() {
		duration = time.Since(start.started).Milliseconds()
	}
	payload := map[string]any{"request_sequence": start.requestSequence, "duration_ms": duration}
	if ev.Result != nil {
		payload["success"] = ev.Result.Error == ""
		payload["error"] = c.redact(ev.Result.Error)
		payload["metadata"] = c.sanitize(ev.Result.Metadata)
		payload["output_bytes"] = len(ev.Result.Output)
		if !utf8.ValidString(ev.Result.Output) {
			payload["output_binary_omitted"] = true
		} else if c.cfg.CaptureToolOutput == "bounded" {
			payload["output"], payload["output_truncated"] = bound(c.redact(ev.Result.Output), c.cfg.OutputLimit)
		}
	}
	if c.cfg.LogToolActivity != nil && *c.cfg.LogToolActivity {
		slog.Info("agent tool completed", "task", c.taskID, "trace", c.traceID, "role", c.role,
			"tool", ev.ToolCall.Name, "duration_ms", duration, "success", payload["success"],
			"exit_code", metadataValue(ev.Result, "exit_code"), "truncated", metadataValue(ev.Result, "truncated"))
	}
	if c.cfg.Enabled != nil && !*c.cfg.Enabled {
		return nil
	}
	return c.append(ctx, "tool_result", ev.ToolCall.ID, ev.ToolCall.Name, payload)
}

func metadataValue(result *tool.ToolResult, key string) any {
	if result == nil || result.Metadata == nil {
		return nil
	}
	return result.Metadata[key]
}

func (c *Collector) requestUsage(ctx context.Context, request *llm.RequestUsage) error {
	if request == nil {
		return nil
	}
	c.request++
	c.summary.Requests = c.request
	usage := store.AgentRequestUsage{
		TaskID: c.taskID, TraceID: c.traceID, RequestID: request.ID, Role: c.role,
		Model: request.Model, Category: string(request.Category), Status: request.Status, Source: request.Source,
	}
	if usage.RequestID == "" {
		usage.RequestID = fmt.Sprintf("%s-%d", c.traceID, c.request)
	}
	if request.Usage != nil {
		usage.InputTokens = int64ptr(request.Usage.InputTokens)
		usage.OutputTokens = int64ptr(request.Usage.OutputTokens)
		usage.CacheCreationInputTokens = int64ptr(request.Usage.CacheCreationInputTokens)
		usage.CacheReadInputTokens = int64ptr(request.Usage.CacheReadInputTokens)
		c.summary.InputTokens += request.Usage.InputTokens
		c.summary.OutputTokens += request.Usage.OutputTokens
	}
	if _, err := c.sink.RecordAgentRequestUsage(ctx, usage); err != nil {
		return c.fail(err)
	}
	if c.cfg.CaptureAssistantText == "all-visible" {
		if err := c.flushText(ctx); err != nil {
			return err
		}
	}
	if c.cfg.Enabled != nil && !*c.cfg.Enabled {
		return nil
	}
	payload := map[string]any{"request_id": usage.RequestID, "request_sequence": c.request, "model": usage.Model,
		"category": usage.Category, "status": usage.Status, "source": usage.Source}
	if request.Usage != nil {
		payload["input_tokens"], payload["output_tokens"] = request.Usage.InputTokens, request.Usage.OutputTokens
		payload["cache_creation_input_tokens"] = request.Usage.CacheCreationInputTokens
		payload["cache_read_input_tokens"] = request.Usage.CacheReadInputTokens
	}
	return c.append(ctx, "request_usage", "", "", payload)
}

func (c *Collector) flushText(ctx context.Context) error {
	if c.text.Len() == 0 || c.cfg.CaptureAssistantText == "none" {
		return nil
	}
	originalBytes := c.text.Len()
	value, truncated := bound(c.redact(c.text.String()), c.cfg.OutputLimit)
	c.summary.Text, c.summary.TextTruncated = value, truncated
	c.text.Reset()
	if c.cfg.Enabled != nil && !*c.cfg.Enabled {
		return nil
	}
	return c.append(ctx, "assistant_text", "", "", map[string]any{
		"request_sequence": c.request, "text": value, "original_bytes": originalBytes, "truncated": truncated,
	})
}

func (c *Collector) append(ctx context.Context, eventType, callID, toolName string, payload map[string]any) error {
	c.sequence++
	err := c.sink.AppendAgentTraceEvent(ctx, &store.AgentTraceEvent{TaskID: c.taskID, TraceID: c.traceID,
		Role: c.role, Attempt: c.attempt, Sequence: c.sequence, EventType: eventType,
		ToolCallID: callID, ToolName: toolName, Payload: payload})
	if err != nil {
		return c.fail(err)
	}
	return nil
}

func (c *Collector) fail(err error) error {
	c.summary.PersistenceFail = err
	return err
}

func (c *Collector) Summary() Summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := c.summary
	copy.RepeatedTools = map[string]int{}
	for key, value := range c.summary.RepeatedTools {
		copy.RepeatedTools[key] = value
	}
	return copy
}

func (c *Collector) sanitize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if secretKey(key) {
				out[key] = "[REDACTED]"
			} else if key == "path" || key == "file" || key == "file_path" {
				out[key] = c.relativePath(fmt.Sprint(item))
			} else {
				out[key] = c.sanitize(item)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = c.sanitize(item)
		}
		return out
	case string:
		value, truncated := bound(c.redact(typed), c.cfg.OutputLimit)
		if truncated {
			return value + " [TRUNCATED]"
		}
		return value
	default:
		return value
	}
}

func (c *Collector) sanitizeToolArguments(value any) any {
	typed, ok := value.(map[string]any)
	if !ok {
		return c.sanitize(value)
	}
	out := make(map[string]any, len(typed))
	for key, item := range typed {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		if normalized == "command" || normalized == "content" || normalized == "body" ||
			normalized == "data" || normalized == "old_text" || normalized == "new_text" || normalized == "patch" {
			out[key+"_omitted"] = true
			out[key+"_bytes"] = len(fmt.Sprint(item))
			continue
		}
		if normalized == "path" || normalized == "file" || normalized == "file_path" {
			out[key] = c.relativePath(fmt.Sprint(item))
			continue
		}
		out[key] = c.sanitize(item)
	}
	return out
}

func (c *Collector) relativePath(path string) string {
	if path == "" || c.workspace == "" || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(c.workspace, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "[OUTSIDE_WORKSPACE]"
	}
	return filepath.ToSlash(rel)
}

func (c *Collector) redact(value string) string {
	return Redact(value, c.secrets...)
}

// Redact removes configured secret values and common credential forms from
// text before it is persisted or published as audit evidence.
func Redact(value string, secrets ...string) string {
	for _, secret := range nonEmpty(secrets) {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	value = regexp.MustCompile(`(?i)(authorization|private-token|api[-_]?key|password|token|secret)\s*[:=]\s*[^\s,;]+`).ReplaceAllString(value, "$1=[REDACTED]")
	value = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^/@\s]+@`).ReplaceAllStringFunc(value, func(match string) string {
		return match[:strings.Index(match, "://")+3] + "[REDACTED]@"
	})
	value = regexp.MustCompile(`(?i)([?&](?:token|key|secret|password|signature|credential)=)[^&\s]+`).ReplaceAllString(value, "$1[REDACTED]")
	value = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`).ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
	return value
}

func secretKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, part := range []string{"token", "password", "secret", "authorization", "cookie", "api_key", "dsn", "database_url", "private_key", "env"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func aggregateUsage(usage *llm.Usage) map[string]any {
	if usage == nil {
		return map[string]any{"usage_source": "unavailable"}
	}
	return map[string]any{"usage_source": "reported", "input_tokens": usage.InputTokens,
		"output_tokens": usage.OutputTokens, "cache_creation_input_tokens": usage.CacheCreationInputTokens,
		"cache_read_input_tokens": usage.CacheReadInputTokens}
}

func errorCategory(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "maximum turns") || strings.Contains(lower, "max turns"):
		return "max_turns"
	case strings.Contains(lower, "context canceled"):
		return "canceled"
	case strings.Contains(lower, "deadline") || strings.Contains(lower, "timeout"):
		return "timeout"
	default:
		return "runtime"
	}
}

func bound(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value, true
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func int64ptr(value int) *int64 { result := int64(value); return &result }

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
