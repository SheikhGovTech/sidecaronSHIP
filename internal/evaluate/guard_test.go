package evaluate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepeatedFileAndGitHistoryCallsRequestFinalization(t *testing.T) {
	for _, test := range []struct {
		name  string
		tool  string
		input string
	}{
		{name: "file", tool: "read_file", input: `{"path":"file.go"}`},
		{name: "git history", tool: "bash", input: `{"command":"git log -5 --oneline"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			guard := newToolGuard(nil, nil)
			_, allowed := guard.check(test.tool, []byte(test.input))
			assert.True(t, allowed)
			_, allowed = guard.check(test.tool, []byte(test.input))
			assert.False(t, allowed)
			_, allowed = guard.check(test.tool, []byte(test.input))
			assert.False(t, allowed)
			assert.True(t, guard.finalizationRequested())
		})
	}
}
