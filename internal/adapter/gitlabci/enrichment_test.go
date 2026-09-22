package gitlabci

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractErrorContextPreservesContextTailAndCap(t *testing.T) {
	lines := make([]string, 220)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%03d", i)
	}
	for i := 2; i < 180; i += 5 {
		lines[i] = fmt.Sprintf("Error: failure-%03d", i)
	}

	a := NewWithBaseURL("group/project", "", time.Second, nil, "")
	got := strings.Split(a.extractErrorContext(strings.Join(lines, "\n")), "\n")

	require.LessOrEqual(t, len(got), maxExtractedLogLines)
	assert.Contains(t, got, "line-000") // context before the first error
	for _, line := range lines[len(lines)-tailLogLines:] {
		assert.Contains(t, got, line, "tail line %q must always be retained", line)
	}
}

func TestExtractErrorContextPreservesDuplicateLinesByPosition(t *testing.T) {
	lines := make([]string, 180)
	for i := range lines {
		lines[i] = "repeated"
	}
	lines[50] = "Error: boom"

	a := NewWithBaseURL("group/project", "", time.Second, nil, "")
	got := strings.Split(a.extractErrorContext(strings.Join(lines, "\n")), "\n")

	assert.GreaterOrEqual(t, count(got, "repeated"), tailLogLines,
		"identical log lines at different positions must not be deduplicated")
}

func TestDefaultErrorPatterns(t *testing.T) {
	a := NewWithBaseURL("group/project", "", time.Second, nil, "")
	for _, pattern := range []string{
		"Error:", "FAIL", "panic:", "Uncaught Exception", "not found",
		"not defined", "exit code", "Test Files", "AssertionError", "TypeError",
	} {
		t.Run(pattern, func(t *testing.T) {
			assert.True(t, a.matchesErrorPattern("prefix "+pattern+" suffix"))
		})
	}
}

func TestFetchCommitDiffSmallDiff(t *testing.T) {
	server := diffServer(t, []gitlabDiffEntry{
		{OldPath: "old.go", NewPath: "new.go", Diff: "+small change"},
	})
	defer server.Close()

	a := NewWithBaseURL("group/project", "token", time.Second, nil, server.URL)
	files, diff := a.fetchCommitDiff(context.Background(), "abc123")

	assert.Equal(t, "new.go", files)
	assert.Contains(t, diff, "+small change")
}

func TestFetchCommitDiffLargeDiffFallsBackToCompleteFileList(t *testing.T) {
	server := diffServer(t, []gitlabDiffEntry{
		{OldPath: "first.go", NewPath: "first.go", Diff: strings.Repeat("x", 300*1024)},
		{OldPath: "second.go", NewPath: "second.go", Diff: "+small change"},
	})
	defer server.Close()

	a := NewWithBaseURL("group/project", "token", time.Second, nil, server.URL)
	files, diff := a.fetchCommitDiff(context.Background(), "abc123")

	assert.Equal(t, "first.go, second.go", files)
	assert.Empty(t, diff)
}

func TestDetectFlakeRequiresAlternation(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     bool
	}{
		{name: "alternating", statuses: []string{"success", "failed", "success", "failed"}, want: true},
		{name: "mixed but not alternating", statuses: []string{"success", "success", "failed", "success"}, want: false},
		{name: "all failures", statuses: []string{"failed", "failed", "failed"}, want: false},
		{name: "insufficient history", statuses: []string{"success", "failed"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := pipelineHistoryServer(t, 99, tt.statuses)
			defer server.Close()

			a := NewWithBaseURL("group/project", "token", time.Second, nil, server.URL)
			assert.Equal(t, tt.want, a.detectFlake(context.Background(), "main", 99))
		})
	}
}

func diffServer(t *testing.T, diffs []gitlabDiffEntry) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))
		assert.Equal(t, "token", r.Header.Get("PRIVATE-TOKEN"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(diffs))
	}))
}

func pipelineHistoryServer(t *testing.T, currentID int64, statuses []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pipelines := []gitlabPipeline{{ID: currentID, Status: "failed", Ref: "main"}}
		for i, status := range statuses {
			pipelines = append(pipelines, gitlabPipeline{ID: int64(i + 1), Status: status, Ref: "main"})
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(pipelines))
	}))
}

func count(lines []string, target string) int {
	total := 0
	for _, line := range lines {
		if line == target {
			total++
		}
	}
	return total
}
