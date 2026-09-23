package loop

import "github.com/sausheong/harness/llm"

// SetProviderForTest overrides the LLM provider. Test-only seam; not used in
// production. Kept in a normal file (not _test.go) so integration tests in
// package loop_test can call it.
func SetProviderForTest(l *Loop, p llm.LLMProvider) { l.provider = p }

// SetAfterCommitForTest injects a callback between commit creation and
// pre-publication integrity verification.
func SetAfterCommitForTest(l *Loop, fn func(string) error) { l.afterCommit = fn }
