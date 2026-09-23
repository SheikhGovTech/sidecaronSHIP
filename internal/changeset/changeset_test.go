package changeset_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/sidecar/internal/changeset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		run(t, dir, args...)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.go"), []byte("package sample\n"), 0o600))
	run(t, dir, "add", "source.go")
	run(t, dir, "commit", "-m", "base")
	return dir, run(t, dir, "rev-parse", "HEAD")
}
func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func TestPrepareExcludesArtifactsAndNormalizesAgentCommit(t *testing.T) {
	dir, base := repo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.go"), []byte("package sample\n// fixed\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".harness", "spill"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".harness", "spill", "trace"), []byte("runtime"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".coverage"), []byte("coverage"), 0o600))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "agent commit")

	snap, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	require.Len(t, snap.Paths, 1)
	assert.Equal(t, "source.go", snap.Paths[0].Path)
	assert.ElementsMatch(t, []string{".coverage", ".harness/spill/trace"}, snap.Excluded)
	assert.Equal(t, base, run(t, dir, "rev-parse", "HEAD"))
	assert.NotEmpty(t, snap.Digest)
}

func TestPrepareNormalizesMultipleAgentCommits(t *testing.T) {
	dir, base := repo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "first.txt"), []byte("first\n"), 0o600))
	run(t, dir, "add", "first.txt")
	run(t, dir, "commit", "-m", "agent first")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second\n"), 0o600))
	run(t, dir, "add", "second.txt")
	run(t, dir, "commit", "-m", "agent second")

	snap, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	assert.Equal(t, base, run(t, dir, "rev-parse", "HEAD"))
	assert.Equal(t, []changeset.PathChange{{Status: "A", Path: "first.txt"}, {Status: "A", Path: "second.txt"}}, snap.Paths)
}

func TestSnapshotManifestOrderingAndDigestAreStable(t *testing.T) {
	dir, base := repo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "z.txt"), []byte("z\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o600))
	first, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	second, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)

	assert.Equal(t, []changeset.PathChange{{Status: "A", Path: "a.txt"}, {Status: "A", Path: "z.txt"}}, first.Paths)
	assert.Equal(t, first.Paths, second.Paths)
	assert.Equal(t, first.Digest, second.Digest)
}

func TestCommitAndVerifyRejectLaterArtifacts(t *testing.T) {
	dir, base := repo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.go"), []byte("package sample\n// fixed\n"), 0o600))
	snap, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "coverage.xml"), []byte("later"), 0o600))
	require.NoError(t, changeset.Commit(dir, snap, "sidecar fix"))
	require.NoError(t, changeset.VerifyCommitted(dir, snap))
	assert.Equal(t, "source.go", run(t, dir, "show", "--pretty=", "--name-only", "HEAD"))
}

func TestVerificationAndEvaluatorArtifactsRemainOutsideCommit(t *testing.T) {
	dir, base := repo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.go"), []byte("package sample\n// fixed\n"), 0o600))
	snap, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "verification.log"), []byte("generated\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "evaluator.log"), []byte("generated\n"), 0o600))
	require.NoError(t, changeset.VerifyPrepared(dir, snap))
	require.NoError(t, changeset.Commit(dir, snap, "sidecar fix"))

	assert.Equal(t, "source.go", run(t, dir, "show", "--pretty=", "--name-only", "HEAD"))
	assert.Contains(t, run(t, dir, "status", "--short"), "verification.log")
	assert.Contains(t, run(t, dir, "status", "--short"), "evaluator.log")
}

func TestPreparedIndexMutationFailsClosed(t *testing.T) {
	dir, base := repo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.go"), []byte("package sample\n// fixed\n"), 0o600))
	snap, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("unexpected"), 0o600))
	run(t, dir, "add", "extra.txt")
	assert.Error(t, changeset.Commit(dir, snap, "sidecar fix"))
}

func TestCommitControlsIdentityAndSigning(t *testing.T) {
	dir, base := repo(t)
	run(t, dir, "config", "user.name", "Agent")
	run(t, dir, "config", "user.email", "agent@example.com")
	run(t, dir, "config", "commit.gpgsign", "true")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.go"), []byte("package sample\n// fixed\n"), 0o600))
	snap, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	require.NoError(t, changeset.Commit(dir, snap, "sidecar fix"))

	assert.Equal(t, "Sidecar <sidecar@sidecar.dev>", run(t, dir, "show", "-s", "--format=%an <%ae>", "HEAD"))
	assert.Equal(t, "Sidecar <sidecar@sidecar.dev>", run(t, dir, "show", "-s", "--format=%cn <%ce>", "HEAD"))
	assert.Equal(t, "N", run(t, dir, "show", "-s", "--format=%G?", "HEAD"))
}

func TestAgentSelfCommitCannotBypassFilteringOrSigning(t *testing.T) {
	dir, base := repo(t)
	run(t, dir, "config", "user.name", "Agent")
	run(t, dir, "config", "user.email", "agent@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.go"), []byte("package sample\n// fixed\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".harness"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".harness", "agent.log"), []byte("secret runtime output\n"), 0o600))
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "agent bypass")

	snap, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	require.NoError(t, changeset.Commit(dir, snap, "sidecar fix"))

	assert.Equal(t, []changeset.PathChange{{Status: "M", Path: "source.go"}}, snap.Paths)
	assert.Equal(t, []string{".harness/agent.log"}, snap.Excluded)
	assert.Equal(t, "Sidecar <sidecar@sidecar.dev>", run(t, dir, "show", "-s", "--format=%an <%ae>", "HEAD"))
	assert.Equal(t, "source.go", run(t, dir, "show", "--pretty=", "--name-only", "HEAD"))
}

func TestCommittedMismatchFailsClosed(t *testing.T) {
	dir, base := repo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.go"), []byte("package sample\n// fixed\n"), 0o600))
	snap, err := changeset.Prepare(dir, base, nil)
	require.NoError(t, err)
	require.NoError(t, changeset.Commit(dir, snap, "sidecar fix"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("unexpected"), 0o600))
	run(t, dir, "add", "extra.txt")
	run(t, dir, "commit", "--amend", "--no-edit")
	assert.Error(t, changeset.VerifyCommitted(dir, snap))
}

func TestInvalidBaseDoesNotRewriteHistory(t *testing.T) {
	dir, _ := repo(t)
	head := run(t, dir, "rev-parse", "HEAD")
	_, err := changeset.Prepare(dir, "missing-base", nil)
	assert.Error(t, err)
	assert.Equal(t, head, run(t, dir, "rev-parse", "HEAD"))
}

func TestConfiguredExclusionAppliesToTrackedFile(t *testing.T) {
	dir, base := repo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "generated.txt"), []byte("base"), 0o600))
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "generated base")
	base = run(t, dir, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "generated.txt"), []byte("changed"), 0o600))
	snap, err := changeset.Prepare(dir, base, []string{"generated.txt"})
	require.NoError(t, err)
	assert.Empty(t, snap.Paths)
	assert.Equal(t, []string{"generated.txt"}, snap.Excluded)
}
