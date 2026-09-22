package loop

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceHasChangesDetectsUntrackedAndSelfCommit(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{
		{"git", "-C", repo, "init"},
		{"git", "-C", repo, "config", "user.email", "test@test.com"},
		{"git", "-C", repo, "config", "user.name", "Test"},
		{"git", "-C", repo, "commit", "--allow-empty", "-m", "initial"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	baseOut, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	base := string(baseOut[:len(baseOut)-1])

	changed, err := workspaceHasChanges(repo, base)
	require.NoError(t, err)
	assert.False(t, changed)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new"), 0o644))
	changed, err = workspaceHasChanges(repo, base)
	require.NoError(t, err)
	assert.True(t, changed)

	for _, args := range [][]string{{"git", "-C", repo, "add", "new.txt"}, {"git", "-C", repo, "commit", "-m", "agent commit"}} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	changed, err = workspaceHasChanges(repo, base)
	require.NoError(t, err)
	assert.True(t, changed, "a clean agent-created commit must differ from the task base")
}
