package output

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentAskpassContainsNoCredential(t *testing.T) {
	path, err := writeEnvironmentAskpass()
	require.NoError(t, err)
	defer os.Remove(path)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "SIDECAR_GIT_TOKEN")
	assert.NotContains(t, string(contents), "very-secret")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}
