package evaluate_test

import (
	"testing"

	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/evaluate"
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
