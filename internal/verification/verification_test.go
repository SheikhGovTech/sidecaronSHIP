package verification_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "backend"), 0o755))

	got, err := verification.ResolveWorkingDirectory(root, "backend")
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(filepath.Join(root, "backend"))
	require.NoError(t, err)
	assert.Equal(t, want, got)

	_, err = verification.ResolveWorkingDirectory(root, "../outside")
	assert.ErrorContains(t, err, "escapes workspace")
}

func TestResolveWorkingDirectoryRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "escape")))

	_, err := verification.ResolveWorkingDirectory(root, "escape")
	assert.ErrorContains(t, err, "escapes workspace")
}

func TestPreflightRequiresAllowedPATH(t *testing.T) {
	root := t.TempDir()
	cmd := config.VerificationCommand{Name: "tests", RequiredTools: []string{"sh"}}
	_, err := verification.Preflight(root, []config.VerificationCommand{cmd})
	assert.Error(t, err)

	cmd.PassEnv = []string{"PATH"}
	_, err = verification.Preflight(root, []config.VerificationCommand{cmd})
	require.NoError(t, err)
}

func TestPreflightRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, "outside")))
	name, err := verification.Preflight(root, []config.VerificationCommand{{Name: "tests", WorkingDirectory: "outside"}})
	assert.Equal(t, "tests", name)
	assert.ErrorContains(t, err, "escapes workspace")
}

func TestRunSequentialAndStopsOnFailure(t *testing.T) {
	root := t.TempDir()
	commands := []config.VerificationCommand{
		{Name: "first", Run: "printf first >> order"},
		{Name: "second", Run: "printf second >> order; exit 7"},
		{Name: "third", Run: "printf third >> order"},
	}
	results := verification.Run(context.Background(), root, commands)
	require.Len(t, results, 2)
	assert.Equal(t, 7, results[1].ExitCode)
	contents, err := os.ReadFile(filepath.Join(root, "order"))
	require.NoError(t, err)
	assert.Equal(t, "firstsecond", string(contents))
}

func TestRunUsesMinimalEnvironmentAndRedactsAllowedSecret(t *testing.T) {
	t.Setenv("NOT_ALLOWED", "hidden")
	t.Setenv("TEST_SECRET_TOKEN", "very-secret")
	root := t.TempDir()
	result := verification.RunOne(context.Background(), root, config.VerificationCommand{
		Name:    "environment",
		Run:     `printf '%s|%s' "$NOT_ALLOWED" "$TEST_SECRET_TOKEN"`,
		PassEnv: []string{"TEST_SECRET_TOKEN"},
	})
	require.NoError(t, result.Err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "|[REDACTED]", result.Output)
	assert.NotContains(t, result.Output, "very-secret")
}

func TestRunBoundsOutput(t *testing.T) {
	result := verification.RunOne(context.Background(), t.TempDir(), config.VerificationCommand{
		Name: "large-output",
		Run:  `i=0; while [ "$i" -lt 70000 ]; do printf x; i=$((i+1)); done`,
	})
	require.NoError(t, result.Err)
	assert.True(t, result.Truncated)
	assert.Len(t, result.Output, verification.MaxOutputBytes)
}

func TestRunTimeoutKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	result := verification.RunOne(context.Background(), root, config.VerificationCommand{
		Name:    "timeout",
		Run:     `sleep 30 & echo $! > child.pid; wait`,
		Timeout: "100ms",
		PassEnv: []string{"PATH"},
	})
	assert.ErrorContains(t, result.Err, "timed out")

	raw, err := os.ReadFile(filepath.Join(root, "child.pid"))
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		return syscall.Kill(pid, 0) != nil
	}, time.Second, 20*time.Millisecond, fmt.Sprintf("child process %d still exists", pid))
}
