package cli_test

import (
	"testing"

	"github.com/sausheong/sidecar/internal/cli"
	"github.com/stretchr/testify/assert"
)

func TestTaskCmd_RequiresDescription(t *testing.T) {
	root := cli.RootCmd()
	root.SetArgs([]string{"task"})
	err := root.Execute()
	assert.Error(t, err)
}

func TestTaskCmd_RequiresDBURL(t *testing.T) {
	t.Setenv("SIDECAR_DB_URL", "")
	root := cli.RootCmd()
	root.SetArgs([]string{"task", "fix the tests", "--repo", "/nonexistent"})
	err := root.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SIDECAR_DB_URL")
}

func TestTaskShowValidatesIDBeforeDatabaseAccess(t *testing.T) {
	root := cli.RootCmd()
	root.SetArgs([]string{"task", "show", "not-a-uuid"})
	err := root.Execute()
	assert.ErrorContains(t, err, "invalid task ID")
}

func TestTaskShowRequiresDBURL(t *testing.T) {
	t.Setenv("SIDECAR_DB_URL", "")
	root := cli.RootCmd()
	root.SetArgs([]string{"task", "show", "00000000-0000-0000-0000-000000000001"})
	err := root.Execute()
	assert.ErrorContains(t, err, "SIDECAR_DB_URL")
}
