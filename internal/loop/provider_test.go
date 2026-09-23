package loop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/sidecar/internal/adapter"
	"github.com/sausheong/sidecar/internal/agenttrace"
	"github.com/sausheong/sidecar/internal/completion"
	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/triage"
	"github.com/sausheong/sidecar/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicCompatibleEndpointPreservesModelAndCredential(t *testing.T) {
	const model = "bedrock.claude-sonnet-4-5"
	var requestPath, requestKey, requestModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestKey = r.Header.Get("X-Api-Key")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var payload struct {
			Model string `json:"model"`
		}
		require.NoError(t, json.Unmarshal(body, &payload))
		requestModel = payload.Model

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\""+model+"\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	t.Cleanup(server.Close)

	provider := newLLMProvider("gateway-token", server.URL)
	events, err := provider.ChatStream(context.Background(), llm.ChatRequest{
		Model:    model,
		Messages: []llm.Message{{Role: "user", Content: "test"}},
	})
	require.NoError(t, err)
	for range events {
	}

	assert.Equal(t, "/v1/messages", requestPath)
	assert.Equal(t, "gateway-token", requestKey)
	assert.Equal(t, model, requestModel)
}

func TestIncompleteHandoffIsStructuredAndBoundedForNotifications(t *testing.T) {
	taskID, traceID := uuid.New(), uuid.New()
	handoff := incompleteHandoff(taskID, completion.EvaluationIncomplete, strings.Repeat("x", 1024), "verdict missing", traceID,
		[]verification.Result{{Name: "tests", ExitCode: 0, Duration: time.Second}})
	assert.Equal(t, completion.EvaluationIncomplete, handoff["reason"])
	assert.Equal(t, taskID.String(), handoff["task_id"])
	assert.Equal(t, traceID.String(), handoff["trace_id"])
	assert.Contains(t, handoff["completed"], "deterministic verification passed: tests")
	assert.LessOrEqual(t, len(handoffNotification(handoff)), 4096)
}

func TestSanitizeVerificationEvidenceRedactsAndBoundsOutput(t *testing.T) {
	results := sanitizeVerificationEvidence([]verification.Result{{Name: "tests", Output: "token=secret-value\n" + strings.Repeat("a", 9000)}}, "secret-value")
	require.Len(t, results, 1)
	assert.NotContains(t, results[0].Output, "secret-value")
	assert.Contains(t, results[0].Output, "[REDACTED]")
	assert.LessOrEqual(t, len(results[0].Output), 4096)
}

func TestIncompleteDraftBodyProminentlyRequiresHumanReview(t *testing.T) {
	l := &Loop{cfg: &config.Config{}}
	body := l.prBody(adapter.Signal{Type: adapter.SignalCIFailure}, triage.TriageResult{ChangeType: "bug_fix", AutonomyLevel: "pull-request"},
		uuid.NewString(), agenttrace.Summary{}, agenttrace.Summary{}, []verification.Result{{Name: "tests", ExitCode: 0}}, "incomplete", "verdict missing")
	assert.Contains(t, body, "Human review required")
	assert.Contains(t, body, "not evaluator-approved")
}
