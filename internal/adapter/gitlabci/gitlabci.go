package gitlabci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sausheong/sidecar/internal/adapter"
)

const (
	defaultBaseURL       = "https://sgts.gitlab-dedicated.com"
	maxExtractedLogLines = 150
	tailLogLines         = 30
	fallbackLogLines     = 100
	maxCommitDiffBytes   = 32 * 1024
)

var defaultErrorPatterns = []string{
	"Error:", "ERROR:", "error:",
	"FAIL", "FAILED",
	"fatal:", "Fatal:",
	"panic:", "PANIC:",
	"Uncaught Exception",
	"AssertionError", "TypeError", "ReferenceError",
	"No \"", "not found", "not defined",
	"exit code", "exit status",
	"Test Files", "Tests ", "Errors ",
}

type GitLabCIAdapter struct {
	repo          string
	token         string
	pollInterval  time.Duration
	watch         []string
	baseURL       string
	errorPatterns []string

	seen     map[int64]bool
	seenMu   sync.Mutex
	stopOnce sync.Once
	stopCh   chan struct{}
	client   *http.Client
}

func New(repo, token string, pollInterval time.Duration, watch []string) *GitLabCIAdapter {
	return NewWithBaseURL(repo, token, pollInterval, watch, defaultBaseURL)
}

func NewWithBaseURL(repo, token string, pollInterval time.Duration, watch []string, baseURL string) *GitLabCIAdapter {
	return &GitLabCIAdapter{
		repo:          repo,
		token:         token,
		pollInterval:  pollInterval,
		watch:         watch,
		baseURL:       baseURL,
		errorPatterns: defaultErrorPatterns,
		seen:          make(map[int64]bool),
		stopCh:        make(chan struct{}),
		client:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *GitLabCIAdapter) SetErrorPatterns(extra []string) {
	if len(extra) > 0 {
		a.errorPatterns = append(a.errorPatterns, extra...)
	}
}

func (a *GitLabCIAdapter) Name() string { return "gitlab-ci" }

func (a *GitLabCIAdapter) Start(ctx context.Context, out chan<- adapter.Signal) error {
	go func() {
		ticker := time.NewTicker(a.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-a.stopCh:
				return
			case <-ticker.C:
				a.poll(ctx, out)
			}
		}
	}()
	return nil
}

func (a *GitLabCIAdapter) Stop() error {
	a.stopOnce.Do(func() { close(a.stopCh) })
	return nil
}

// ── Data types ──────────────────────────────────────────────────────────

type gitlabPipeline struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
	WebURL string `json:"web_url"`
}

type gitlabJob struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	WebURL string `json:"web_url"`
}

type gitlabDiffEntry struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Diff    string `json:"diff"`
}

// ── Poll and enrich ─────────────────────────────────────────────────────

func (a *GitLabCIAdapter) poll(ctx context.Context, out chan<- adapter.Signal) {
	pipelines, err := a.fetchPipelines(ctx)
	if err != nil {
		slog.Warn("gitlab-ci poll failed", "repo", a.repo, "err", err)
		return
	}
	for _, p := range pipelines {
		if !a.isWatched(p.Status) {
			continue
		}
		a.seenMu.Lock()
		already := a.seen[p.ID]
		if !already {
			a.seen[p.ID] = true
		}
		a.seenMu.Unlock()
		if already {
			continue
		}

		failedJob, jobLog := a.fetchFailedJobLogs(ctx, p.ID)
		changedFiles, commitDiff := a.fetchCommitDiff(ctx, p.SHA)
		isFlake := a.detectFlake(ctx, p.Ref, p.ID)

		select {
		case <-a.stopCh:
			return
		case out <- adapter.Signal{
			Type:   adapter.SignalCIFailure,
			Source: "gitlab-ci",
			Payload: map[string]any{
				"pipeline_id":   p.ID,
				"workflow_name": p.Ref,
				"conclusion":    p.Status,
				"html_url":      p.WebURL,
				"head_sha":      p.SHA,
				"repo":          a.repo,
				"is_flake":      isFlake,
				"failed_job":    failedJob,
				"job_log":       jobLog,
				"changed_files": changedFiles,
				"commit_diff":   commitDiff,
			},
		}:
		}
	}
}

// ── Fetch pipelines ─────────────────────────────────────────────────────

func (a *GitLabCIAdapter) fetchPipelines(ctx context.Context) ([]gitlabPipeline, error) {
	encoded := url.PathEscape(a.repo)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/pipelines?order_by=id&sort=desc&per_page=10",
		a.baseURL, encoded)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	if a.token != "" {
		req.Header.Set("PRIVATE-TOKEN", a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab api status %d", resp.StatusCode)
	}

	var pipelines []gitlabPipeline
	if err := json.NewDecoder(resp.Body).Decode(&pipelines); err != nil {
		return nil, err
	}
	return pipelines, nil
}

// ── Fetch failed job logs ───────────────────────────────────────────────

func (a *GitLabCIAdapter) fetchFailedJobLogs(ctx context.Context, pipelineID int64) (string, string) {
	encoded := url.PathEscape(a.repo)
	jobsURL := fmt.Sprintf("%s/api/v4/projects/%s/pipelines/%d/jobs?per_page=20",
		a.baseURL, encoded, pipelineID)

	jobs, err := a.getJSON(ctx, jobsURL)
	if err != nil {
		return "", ""
	}

	var jobList []gitlabJob
	if err := json.Unmarshal(jobs, &jobList); err != nil {
		return "", ""
	}

	for _, j := range jobList {
		if j.Status != "failed" {
			continue
		}
		trace := a.fetchJobTrace(ctx, encoded, j.ID)
		return j.Name, a.extractErrorContext(trace)
	}
	return "", ""
}

func (a *GitLabCIAdapter) fetchJobTrace(ctx context.Context, encodedProject string, jobID int64) string {
	traceURL := fmt.Sprintf("%s/api/v4/projects/%s/jobs/%d/trace",
		a.baseURL, encodedProject, jobID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, traceURL, nil)
	if err != nil {
		return ""
	}
	if a.token != "" {
		req.Header.Set("PRIVATE-TOKEN", a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return ""
	}
	return string(data)
}

// ── Smart error extraction ──────────────────────────────────────────────

func (a *GitLabCIAdapter) extractErrorContext(trace string) string {
	lines := strings.Split(trace, "\n")
	if len(lines) <= fallbackLogLines {
		return trace
	}

	selected := make([]bool, len(lines))
	var contextIndices []int

	for i, line := range lines {
		if a.matchesErrorPattern(line) {
			start := i - 2
			if start < 0 {
				start = 0
			}
			end := i + 2
			if end >= len(lines) {
				end = len(lines) - 1
			}
			for j := start; j <= end; j++ {
				if !selected[j] {
					selected[j] = true
					contextIndices = append(contextIndices, j)
				}
			}
		}
	}

	if len(contextIndices) == 0 {
		start := len(lines) - fallbackLogLines
		return strings.Join(lines[start:], "\n")
	}

	// Reserve space for the tail before applying the total line cap. Tracking
	// line positions (rather than text values) preserves repeated log lines.
	tailStart := len(lines) - tailLogLines
	if tailStart < 0 {
		tailStart = 0
	}
	keep := make([]bool, len(lines))
	kept := 0
	for i := tailStart; i < len(lines); i++ {
		keep[i] = true
		kept++
	}
	for _, i := range contextIndices {
		if kept >= maxExtractedLogLines {
			break
		}
		if !keep[i] {
			keep[i] = true
			kept++
		}
	}

	errorLines := make([]string, 0, kept)
	for i, line := range lines {
		if keep[i] {
			errorLines = append(errorLines, line)
		}
	}
	return strings.Join(errorLines, "\n")
}

func (a *GitLabCIAdapter) matchesErrorPattern(line string) bool {
	for _, p := range a.errorPatterns {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}

// ── Fetch commit diff ───────────────────────────────────────────────────

func (a *GitLabCIAdapter) fetchCommitDiff(ctx context.Context, sha string) (string, string) {
	encoded := url.PathEscape(a.repo)
	diffURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/commits/%s/diff?per_page=100",
		a.baseURL, encoded, sha)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, diffURL, nil)
	if err != nil {
		return "", ""
	}
	if a.token != "" {
		req.Header.Set("PRIVATE-TOKEN", a.token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}

	var files []string
	var fullDiff strings.Builder
	diffTooLarge := false
	dec := json.NewDecoder(resp.Body)
	token, err := dec.Token()
	if err != nil || token != json.Delim('[') {
		return "", ""
	}
	for dec.More() {
		var d gitlabDiffEntry
		if err := dec.Decode(&d); err != nil {
			return "", ""
		}
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		files = append(files, path)

		if !diffTooLarge {
			entry := fmt.Sprintf("--- a/%s\n+++ b/%s\n%s\n", d.OldPath, d.NewPath, d.Diff)
			if fullDiff.Len()+len(entry) > maxCommitDiffBytes {
				diffTooLarge = true
				fullDiff.Reset()
			} else {
				fullDiff.WriteString(entry)
			}
		}
	}
	if _, err := dec.Token(); err != nil {
		return "", ""
	}

	changedFiles := strings.Join(files, ", ")
	if diffTooLarge {
		return changedFiles, ""
	}
	return changedFiles, fullDiff.String()
}

// ── Flake detection ─────────────────────────────────────────────────────

func (a *GitLabCIAdapter) detectFlake(ctx context.Context, ref string, currentPipelineID int64) bool {
	encoded := url.PathEscape(a.repo)
	historyURL := fmt.Sprintf("%s/api/v4/projects/%s/pipelines?ref=%s&order_by=id&sort=desc&per_page=6",
		a.baseURL, encoded, url.QueryEscape(ref))

	data, err := a.getJSON(ctx, historyURL)
	if err != nil {
		return false
	}

	var pipelines []gitlabPipeline
	if err := json.Unmarshal(data, &pipelines); err != nil {
		return false
	}

	var statuses []string
	for _, p := range pipelines {
		if p.ID == currentPipelineID {
			continue
		}
		if p.Status == "success" || p.Status == "failed" {
			statuses = append(statuses, p.Status)
		}
		if len(statuses) >= 5 {
			break
		}
	}

	if len(statuses) < 3 {
		return false
	}

	for i := 1; i < len(statuses); i++ {
		if statuses[i] == statuses[i-1] {
			return false
		}
	}
	return true
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (a *GitLabCIAdapter) getJSON(ctx context.Context, apiURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	if a.token != "" {
		req.Header.Set("PRIVATE-TOKEN", a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab api status %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 256*1024))
}

func (a *GitLabCIAdapter) isWatched(status string) bool {
	for _, w := range a.watch {
		if w == status {
			return true
		}
	}
	return false
}
