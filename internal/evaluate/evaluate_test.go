package evaluate_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/evaluate"
	"github.com/sausheong/sidecar/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVerdict_Pass(t *testing.T) {
	v, err := evaluate.ParseVerdict(`{"pass": true, "reasons": "tests pass"}`)
	require.NoError(t, err)
	assert.True(t, v.Pass)
}

func TestParseVerdict_Reject(t *testing.T) {
	v, err := evaluate.ParseVerdict(`{"pass": false, "reasons": "missing nil check"}`)
	require.NoError(t, err)
	assert.False(t, v.Pass)
	assert.Contains(t, v.Reasons, "nil check")
}

func TestParseVerdict_Fenced(t *testing.T) {
	v, err := evaluate.ParseVerdict("```json\n{\"pass\": true, \"reasons\": \"ok\"}\n```")
	require.NoError(t, err)
	assert.True(t, v.Pass)
}

func TestParseVerdict_Garbage(t *testing.T) {
	_, err := evaluate.ParseVerdict("not json at all")
	assert.Error(t, err)
}

func TestBuildEvalMessage_EmbedsDiffAndSummary(t *testing.T) {
	msg := evaluate.BuildEvalMessage("fix CI failure", "diff --git a/x b/x")
	assert.Contains(t, msg, "fix CI failure")
	assert.Contains(t, msg, "diff --git a/x b/x")
}

func TestSystemPrompt_IsAdversarial(t *testing.T) {
	sp := evaluate.SystemPrompt()
	assert.Contains(t, sp, "BROKEN")
	assert.Contains(t, sp, "pass")
}

func TestSystemPromptWithContextIncludesWorkspaceAndCommands(t *testing.T) {
	prompt := evaluate.SystemPromptWithContext("/tmp/task-worktree", []config.VerificationCommand{{
		Name: "tests", Run: "go test ./...",
	}})
	assert.Contains(t, prompt, "Workspace root: /tmp/task-worktree")
	assert.Contains(t, prompt, "Do not change to or guess")
	assert.Contains(t, prompt, "tests: go test ./...")
	assert.Contains(t, prompt, "directory: .")
	assert.Contains(t, prompt, "timeout: 10m0s")
}

type completionProvider struct {
	llmtest.Base
	mu       sync.Mutex
	requests []llm.ChatRequest
	respond  func(int, llm.ChatRequest) []llm.ChatEvent
}

func (p *completionProvider) ChatStream(_ context.Context, request llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	call := len(p.requests)
	p.mu.Unlock()
	events := p.respond(call, request)
	stream := make(chan llm.ChatEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func textDone(value string) []llm.ChatEvent {
	return []llm.ChatEvent{{Type: llm.EventTextDelta, Text: value}, {Type: llm.EventDone, Usage: &llm.Usage{InputTokens: 2, OutputTokens: 1}}}
}

func evaluatorRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"git", "-C", dir, "init"}, {"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "Test"}} {
		output, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(output))
	}
	require.NoError(t, os.WriteFile(dir+"/file.txt", []byte("before\n"), 0o600))
	output, err := exec.Command("git", "-C", dir, "add", "file.txt").CombinedOutput()
	require.NoError(t, err, string(output))
	output, err = exec.Command("git", "-C", dir, "commit", "-m", "initial").CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, os.WriteFile(dir+"/file.txt", []byte("after\n"), 0o600))
	return dir
}

func TestEvaluateFinalizesWithoutToolsAfterMalformedInvestigation(t *testing.T) {
	repo := evaluatorRepo(t)
	provider := &completionProvider{respond: func(call int, request llm.ChatRequest) []llm.ChatEvent {
		if call == 1 {
			assert.NotEmpty(t, request.Tools)
			assert.Contains(t, request.Messages[len(request.Messages)-1].Content, "exit_code: 0")
			return textDone("The checks look sufficient but this is not JSON.")
		}
		assert.Empty(t, request.Tools, "reserved finalization request must be tool-free")
		return textDone(`{"outcome":"pass","reasons":"verified","evidence":["verification:tests"]}`)
	}}
	result := verification.Result{Name: "tests", ExitCode: 0, Duration: time.Second, Output: "all tests passed"}
	verdict, _, err := evaluate.EvaluateObservedWithEvidence(context.Background(), provider, "model", repo, "HEAD", "fix", nil, []verification.Result{result}, 20, nil)
	require.NoError(t, err)
	assert.True(t, verdict.Pass)
	assert.Equal(t, "pass", verdict.Outcome)
	assert.Len(t, provider.requests, 2)
}

func TestEvaluateStopsOnEarlyStructuredVerdict(t *testing.T) {
	repo := evaluatorRepo(t)
	provider := &completionProvider{respond: func(_ int, _ llm.ChatRequest) []llm.ChatEvent {
		return textDone(`{"outcome":"reject","reasons":"material regression","evidence":["diff:file.txt"]}`)
	}}
	verdict, _, err := evaluate.EvaluateObservedWithEvidence(context.Background(), provider, "model", repo, "HEAD", "fix", nil, nil, 20, nil)
	require.NoError(t, err)
	assert.False(t, verdict.Pass)
	assert.Equal(t, "reject", verdict.Outcome)
	assert.Len(t, provider.requests, 1)
}

func TestEvaluateMalformedFinalVerdictIsIncomplete(t *testing.T) {
	repo := evaluatorRepo(t)
	provider := &completionProvider{respond: func(_ int, _ llm.ChatRequest) []llm.ChatEvent { return textDone("not a verdict") }}
	_, _, err := evaluate.EvaluateObservedWithEvidence(context.Background(), provider, "model", repo, "HEAD", "fix", nil, nil, 20, nil)
	require.Error(t, err)
	assert.True(t, evaluate.IsIncomplete(err))
}

func TestEvaluateReusesEquivalentVerificationAndFinalizes(t *testing.T) {
	repo := evaluatorRepo(t)
	marker := repo + "/must-not-run"
	command := fmt.Sprintf("touch %s", marker)
	provider := &completionProvider{respond: func(call int, request llm.ChatRequest) []llm.ChatEvent {
		if len(request.Tools) == 0 {
			return textDone(`{"outcome":"pass","reasons":"existing evidence is sufficient","evidence":["verification:tests"]}`)
		}
		toolCall := &llm.ToolCall{ID: fmt.Sprintf("call-%d", call), Name: "bash", Input: []byte(fmt.Sprintf(`{"command":%q}`, command))}
		return []llm.ChatEvent{{Type: llm.EventToolCallStart, ToolCall: toolCall}, {Type: llm.EventToolCallDone, ToolCall: toolCall},
			{Type: llm.EventDone, StopReason: "tool_use", Usage: &llm.Usage{InputTokens: 1, OutputTokens: 1}}}
	}}
	commands := []config.VerificationCommand{{Name: "tests", Run: command}}
	evidence := []verification.Result{{Name: "tests", ExitCode: 0, Output: "already passed"}}
	verdict, _, err := evaluate.EvaluateObservedWithEvidence(context.Background(), provider, "model", repo, "HEAD", "fix", commands, evidence, 4, nil)
	require.NoError(t, err)
	assert.True(t, verdict.Pass)
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "equivalent deterministic command must not execute")
	assert.True(t, strings.Contains(systemText(provider.requests[len(provider.requests)-1]), "verdict finalization") || len(provider.requests[len(provider.requests)-1].Tools) == 0)
}

func TestEvaluateCanaryPatternFinalizesBeforeTwentyTurnHardLimit(t *testing.T) {
	repo := evaluatorRepo(t)
	provider := &completionProvider{respond: func(call int, request llm.ChatRequest) []llm.ChatEvent {
		if len(request.Tools) == 0 {
			return textDone(`{"outcome":"pass","reasons":"material checks complete","evidence":["verification:tests"]}`)
		}
		toolCall := &llm.ToolCall{ID: fmt.Sprintf("investigate-%d", call), Name: "bash",
			Input: []byte(fmt.Sprintf(`{"command":"git status --short && printf %d >/dev/null"}`, call))}
		return []llm.ChatEvent{{Type: llm.EventToolCallStart, ToolCall: toolCall}, {Type: llm.EventToolCallDone, ToolCall: toolCall},
			{Type: llm.EventDone, StopReason: "tool_use", Usage: &llm.Usage{InputTokens: 1, OutputTokens: 1}}}
	}}
	evidence := []verification.Result{{Name: "tests", ExitCode: 0, Output: "2612 passed"}}
	verdict, _, err := evaluate.EvaluateObservedWithEvidence(context.Background(), provider, "model", repo, "HEAD", "restore intended model", nil, evidence, 20, nil)
	require.NoError(t, err)
	assert.True(t, verdict.Pass)
	assert.Less(t, len(provider.requests), 20, "Sidecar must reserve capacity and finalize before Harness's hard limit")
	assert.Empty(t, provider.requests[len(provider.requests)-1].Tools)
}

func systemText(request llm.ChatRequest) string {
	if len(request.SystemPromptParts) > 0 {
		return llm.JoinSystemPromptParts(request.SystemPromptParts)
	}
	return request.SystemPrompt
}
