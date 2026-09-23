package agenttrace_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/sidecar/internal/agenttrace"
	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memorySink struct {
	events []*store.AgentTraceEvent
	usage  []store.AgentRequestUsage
}

type batchMemorySink struct {
	mu      sync.Mutex
	events  []*store.AgentTraceEvent
	usage   []store.AgentRequestUsage
	fail    bool
	batches int
}

func (s *batchMemorySink) AppendAgentTraceEvent(_ context.Context, event *store.AgentTraceEvent) error {
	return s.AppendAgentTraceEvents(context.Background(), []*store.AgentTraceEvent{event})
}

func (s *batchMemorySink) AppendAgentTraceEvents(_ context.Context, events []*store.AgentTraceEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("bounded trace write failed")
	}
	s.batches++
	for _, event := range events {
		copy := *event
		s.events = append(s.events, &copy)
	}
	return nil
}

func (s *batchMemorySink) RecordAgentRequestUsage(_ context.Context, usage store.AgentRequestUsage) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage = append(s.usage, usage)
	return true, nil
}

func (s *memorySink) AppendAgentTraceEvent(_ context.Context, event *store.AgentTraceEvent) error {
	copy := *event
	s.events = append(s.events, &copy)
	return nil
}

func (s *memorySink) RecordAgentRequestUsage(_ context.Context, usage store.AgentRequestUsage) (bool, error) {
	s.usage = append(s.usage, usage)
	return true, nil
}

func traceConfig() config.AgentTraceConfig {
	enabled, logs := true, false
	return config.AgentTraceConfig{Enabled: &enabled, CaptureAssistantText: "all-visible",
		CaptureToolArguments: "sanitized", CaptureToolOutput: "bounded", OutputLimit: 64,
		RetentionDays: 30, LogToolActivity: &logs}
}

func TestCollectorPersistsSanitizedToolAndUsage(t *testing.T) {
	sink := &memorySink{}
	collector := agenttrace.New(sink, uuid.New(), "evaluator", 1, "/tmp/work", traceConfig(), "top-secret")
	call := &llm.ToolCall{ID: "call-1", Name: "bash", Input: json.RawMessage(`{"command":"curl -H 'Authorization: Bearer top-secret' https://example"}`)}
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventToolCallReady, ToolCall: call}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventToolResult, ToolCall: call, Result: &tool.ToolResult{
		Output: "token=top-secret\n2612 passed", Metadata: map[string]any{"exit_code": 0},
	}}))
	request := &llm.RequestUsage{ID: "req-1", Model: "model", Category: llm.CallGeneration, Status: "success", Source: "reported", Usage: &llm.Usage{InputTokens: 100, OutputTokens: 20}}
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventRequestUsage, RequestUsage: request}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventError, Error: assert.AnError}))

	require.Len(t, sink.usage, 1)
	assert.Equal(t, "req-1", sink.usage[0].RequestID)
	assert.Equal(t, int64(100), *sink.usage[0].InputTokens)
	require.GreaterOrEqual(t, len(sink.events), 4)
	for _, event := range sink.events {
		encoded, err := json.Marshal(event.Payload)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "top-secret")
		assert.NotContains(t, string(encoded), "curl -H")
	}
	summary := collector.Summary()
	assert.Equal(t, 120, summary.InputTokens+summary.OutputTokens)
	assert.Equal(t, "error", summary.Terminal)
}

func TestCollectorOmitsFileBodiesAndBinaryOutput(t *testing.T) {
	sink := &memorySink{}
	collector := agenttrace.New(sink, uuid.New(), "coding", 1, "/tmp/work", traceConfig())
	call := &llm.ToolCall{ID: "write-1", Name: "write_file", Input: json.RawMessage(`{"path":"/tmp/work/pkg/a.go","content":"private source body"}`)}
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventToolCallReady, ToolCall: call}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventToolResult, ToolCall: call, Result: &tool.ToolResult{Output: string([]byte{0xff, 0xfe, 0xfd})}}))
	require.Len(t, sink.events, 2)
	encoded, err := json.Marshal(sink.events[0].Payload)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private source body")
	assert.Contains(t, string(encoded), `"path":"pkg/a.go"`)
	assert.Equal(t, true, sink.events[1].Payload["output_binary_omitted"])
	assert.NotContains(t, sink.events[1].Payload, "output")
}

func TestCollectorBoundsUTF8Safely(t *testing.T) {
	sink := &memorySink{}
	cfg := traceConfig()
	cfg.OutputLimit = 5
	collector := agenttrace.New(sink, uuid.New(), "coding", 1, "", cfg)
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventTextDelta, Text: "ab😀cd"}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventDone}))
	assert.Equal(t, "ab", sink.events[0].Payload["text"])
	assert.Equal(t, 8, sink.events[0].Payload["original_bytes"])
}

func TestRedactCredentialsInURLsAndQueries(t *testing.T) {
	value := agenttrace.Redact("postgres://user:pass@db/x https://api/x?token=abc&ok=1")
	assert.NotContains(t, value, "user:pass")
	assert.NotContains(t, value, "token=abc")
}

func TestCollectorRedactsNestedSecretsAndHandlesMalformedArguments(t *testing.T) {
	sink := &memorySink{}
	collector := agenttrace.New(sink, uuid.New(), "coding", 1, "", traceConfig())
	nested := &llm.ToolCall{ID: "nested", Name: "custom", Input: json.RawMessage(`{"options":{"api_key":"secret-value","safe":"yes"}}`)}
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventToolCallReady, ToolCall: nested}))
	malformed := &llm.ToolCall{ID: "bad", Name: "custom", Input: json.RawMessage(`{"broken"`)}
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventToolCallReady, ToolCall: malformed}))
	encoded, err := json.Marshal(sink.events[0].Payload)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret-value")
	assert.Contains(t, string(encoded), "[REDACTED]")
	assert.Equal(t, true, sink.events[1].Payload["arguments_parse_error"])
	assert.NotEmpty(t, sink.events[1].Payload["arguments_sha256"])
}

func TestCollectorDoesNotPersistThinkingOrUnboundedText(t *testing.T) {
	sink := &memorySink{}
	cfg := traceConfig()
	cfg.OutputLimit = 8
	collector := agenttrace.New(sink, uuid.New(), "coding", 1, "/tmp/work", cfg)
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventTextDelta, Text: "visible assistant message"}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventDone}))

	var found bool
	for _, event := range sink.events {
		if event.EventType == "assistant_text" {
			found = true
			assert.Equal(t, "visible ", event.Payload["text"])
			assert.Equal(t, true, event.Payload["truncated"])
		}
	}
	assert.True(t, found)
}

func TestCollectorRecordsUnavailableUsage(t *testing.T) {
	sink := &memorySink{}
	collector := agenttrace.New(sink, uuid.New(), "evaluator", 1, "", traceConfig())
	request := &llm.RequestUsage{ID: "req-unknown", Model: "model", Category: llm.CallRetry, Status: "error", Source: "unavailable"}
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventRequestUsage, RequestUsage: request}))
	require.Len(t, sink.usage, 1)
	assert.Nil(t, sink.usage[0].InputTokens)
	assert.Equal(t, "unavailable", sink.usage[0].Source)
}

func TestCollectorBatchesConcurrentToolsAndTerminalFlush(t *testing.T) {
	sink := &batchMemorySink{}
	collector := agenttrace.New(sink, uuid.New(), "evaluator", 1, "/tmp/work", traceConfig())
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("call-%d", i)
			call := &llm.ToolCall{ID: id, Name: "read_file", Input: json.RawMessage(fmt.Sprintf(`{"path":"/tmp/work/file-%d"}`, i))}
			errs <- collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventToolCallReady, ToolCall: call})
			errs <- collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventToolResult, ToolCall: call, Result: &tool.ToolResult{Output: "ok"}})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventDone}))
	require.NoError(t, collector.Flush(context.Background()))

	sink.mu.Lock()
	defer sink.mu.Unlock()
	assert.Len(t, sink.events, 25)
	assert.Equal(t, "agent_completed", sink.events[len(sink.events)-1].EventType)
	assert.Positive(t, sink.batches)
	for i, event := range sink.events {
		assert.Equal(t, int64(i+1), event.Sequence)
	}
}

func TestCollectorCompactionAbortAndMaximumTurnError(t *testing.T) {
	sink := &memorySink{}
	collector := agenttrace.New(sink, uuid.New(), "coding", 1, "", traceConfig())
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventCompactionStart}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventCompactionSkipped}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventError, Error: errors.New("agent exceeded maximum turns (20)")}))

	types := make([]string, 0, len(sink.events))
	for _, event := range sink.events {
		types = append(types, event.EventType)
	}
	assert.Equal(t, []string{"compaction_started", "compaction_skipped", "agent_error"}, types)
	assert.Equal(t, "error", collector.Summary().Terminal)
}

func TestCollectorRecordsCompactionCompletionAndAbort(t *testing.T) {
	sink := &memorySink{}
	collector := agenttrace.New(sink, uuid.New(), "reviewer", 1, "", traceConfig())
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventCompactionStart}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventCompactionDone}))
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventAborted}))
	require.Len(t, sink.events, 3)
	assert.Equal(t, "compaction_completed", sink.events[1].EventType)
	assert.Equal(t, "agent_aborted", sink.events[2].EventType)
	assert.Equal(t, "aborted", collector.Summary().Terminal)
}

func TestCollectorReportsBoundedPersistenceFailure(t *testing.T) {
	sink := &batchMemorySink{fail: true}
	collector := agenttrace.New(sink, uuid.New(), "coding", 1, "", traceConfig(), "secret-value")
	require.NoError(t, collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventTextDelta, Text: "secret-value"}))
	err := collector.Handle(context.Background(), runtime.AgentEvent{Type: runtime.EventDone})
	require.ErrorContains(t, err, "bounded trace write failed")
	assert.Error(t, collector.Summary().PersistenceFail)
	assert.Empty(t, sink.events)
}
