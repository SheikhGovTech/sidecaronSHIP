package output_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/sidecar/internal/changeset"
	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRemoteURL(t *testing.T) {
	tests := []struct{ raw, host, repo string }{
		{"https://github.com/acme/service.git", "github.com", "acme/service"},
		{"ssh://git@gitlab.example.gov/group/team/service.git", "gitlab.example.gov", "group/team/service"},
		{"git@gitlab.com:group/service.git", "gitlab.com", "group/service"},
	}
	for _, tt := range tests {
		host, repo, err := output.ParseRemoteURL(tt.raw)
		require.NoError(t, err)
		assert.Equal(t, tt.host, host)
		assert.Equal(t, tt.repo, repo)
	}
	_, _, err := output.ParseRemoteURL("/local/repo")
	assert.Error(t, err)
}

func deliveryRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	repo := filepath.Join(root, "repo")
	for _, args := range [][]string{
		{"git", "init", "--bare", bare},
		{"git", "init", repo},
		{"git", "-C", repo, "config", "user.email", "test@test.com"},
		{"git", "-C", repo, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("initial\n"), 0o644))
	for _, args := range [][]string{
		{"git", "-C", repo, "add", "README.md"},
		{"git", "-C", repo, "commit", "-m", "initial"},
		{"git", "-C", repo, "remote", "add", "origin", bare},
		{"git", "-C", repo, "push", "-u", "origin", "HEAD:main"},
		{"git", "-C", repo, "checkout", "-b", "sidecar/task-1"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return repo, bare
}

func deliveryGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func TestResolveDeliveryExplicitGitLab(t *testing.T) {
	repo, _ := deliveryRepo(t)
	t.Setenv("TEST_GITLAB_TOKEN", "secret")
	target, err := output.ResolveDelivery(config.DeliveryConfig{
		Provider: "gitlab", Repo: "group/project", BaseBranch: "main", Token: "$TEST_GITLAB_TOKEN",
	}, repo, "schedule", "", "")
	require.NoError(t, err)
	assert.Equal(t, "gitlab", target.Provider)
	assert.Equal(t, "group/project", target.Repo)
	assert.Equal(t, "secret", target.Token)
	assert.Equal(t, "https://gitlab.com/api/v4", target.APIBaseURL)
}

func TestGitLabPublisherCreatesMergeRequest(t *testing.T) {
	repo, bare := deliveryRepo(t)
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		assert.Equal(t, "token", r.Header.Get("PRIVATE-TOKEN"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"web_url":"https://gitlab.example/group/team/project/-/merge_requests/1"}`))
	}))
	defer server.Close()

	target := output.DeliveryTarget{Provider: "gitlab", Repo: "group/team/project", Remote: "origin", RemoteURL: bare, APIBaseURL: server.URL, Token: "token", BaseBranch: "main"}
	result, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{RepoPath: repo, Branch: "sidecar/task-1", Title: "fix", Body: "body"})
	require.NoError(t, err)
	assert.True(t, result.Pushed)
	assert.False(t, result.Reused)
	assert.NotEmpty(t, result.URL)
	assert.Equal(t, []string{"GET /projects/group%2Fteam%2Fproject/merge_requests", "POST /projects/group%2Fteam%2Fproject/merge_requests"}, calls)
}

func TestGitLabPublisherReusesMergeRequest(t *testing.T) {
	repo, bare := deliveryRepo(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`[{"web_url":"https://gitlab.example/mr/7"}]`))
	}))
	defer server.Close()
	target := output.DeliveryTarget{Provider: "gitlab", Repo: "group/project", Remote: "origin", RemoteURL: bare, APIBaseURL: server.URL, BaseBranch: "main"}
	result, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{RepoPath: repo, Branch: "sidecar/task-1"})
	require.NoError(t, err)
	assert.True(t, result.Reused)
	assert.Equal(t, "https://gitlab.example/mr/7", result.URL)
}

func TestGitHubPublisherCreatesPullRequest(t *testing.T) {
	repo, bare := deliveryRepo(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"html_url":"https://github.example/group/project/pull/1"}`))
	}))
	defer server.Close()
	target := output.DeliveryTarget{Provider: "github", Repo: "group/project", Remote: "origin", RemoteURL: bare, APIBaseURL: server.URL, Token: "token", BaseBranch: "main"}
	result, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{RepoPath: repo, Branch: "sidecar/task-1", Title: "fix"})
	require.NoError(t, err)
	assert.True(t, result.Pushed)
	assert.Equal(t, "https://github.example/group/project/pull/1", result.URL)
}

func TestMatchingApprovedCommitProceedsToProviderDelivery(t *testing.T) {
	repo, bare := deliveryRepo(t)
	base := deliveryGitOutput(t, repo, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "fix.go"), []byte("package sample\n"), 0o644))
	snapshot, err := changeset.Prepare(repo, base, nil)
	require.NoError(t, err)
	require.NoError(t, changeset.Commit(repo, snapshot, "sidecar: fix"))
	require.NoError(t, changeset.VerifyCommitted(repo, snapshot))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"html_url":"https://github.example/org/repo/pull/1"}`))
	}))
	defer server.Close()
	target := output.DeliveryTarget{Provider: "github", Repo: "org/repo", Remote: "origin", RemoteURL: bare, APIBaseURL: server.URL, BaseBranch: "main"}
	result, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{
		RepoPath: repo, Branch: "sidecar/task-1", Title: "sidecar: fix",
	})
	require.NoError(t, err)
	assert.True(t, result.Pushed)
	assert.Equal(t, "https://github.example/org/repo/pull/1", result.URL)
	assert.NotEmpty(t, deliveryGitOutput(t, bare, "show-ref", "refs/heads/sidecar/task-1"))
}

func TestGitHubPublisherCreatesDraftAndAppliesLabel(t *testing.T) {
	repo, bare := deliveryRepo(t)
	var draft bool
	var labels []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			var body struct {
				Draft bool `json:"draft"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			draft = body.Draft
			_, _ = w.Write([]byte(`{"html_url":"https://github.example/pull/1","number":1}`))
		case strings.HasSuffix(r.URL.Path, "/issues/1/labels"):
			var body struct {
				Labels []string `json:"labels"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			labels = body.Labels
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()
	target := output.DeliveryTarget{Provider: "github", Repo: "group/project", Remote: "origin", RemoteURL: bare, APIBaseURL: server.URL, BaseBranch: "main"}
	_, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{
		RepoPath: repo, Branch: "sidecar/task-1", Title: "fix", Draft: true, Labels: []string{"sidecar:evaluation-error"},
	})
	require.NoError(t, err)
	assert.True(t, draft)
	assert.Equal(t, []string{"sidecar:evaluation-error"}, labels)
}

func TestGitLabPublisherMarksDraftAndLabels(t *testing.T) {
	repo, bare := deliveryRepo(t)
	var title, labels string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		title, labels = body["title"], body["labels"]
		_, _ = w.Write([]byte(`{"web_url":"https://gitlab.example/mr/1"}`))
	}))
	defer server.Close()
	target := output.DeliveryTarget{Provider: "gitlab", Repo: "group/project", Remote: "origin", RemoteURL: bare, APIBaseURL: server.URL, BaseBranch: "main"}
	_, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{
		RepoPath: repo, Branch: "sidecar/task-1", Title: "fix", Draft: true, Labels: []string{"sidecar:evaluation-error"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Draft: fix", title)
	assert.Equal(t, "sidecar:evaluation-error", labels)
}

func TestGitHubPublisherReusesPullRequest(t *testing.T) {
	repo, bare := deliveryRepo(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`[{"html_url":"https://github.example/group/project/pull/9"}]`))
	}))
	defer server.Close()
	target := output.DeliveryTarget{Provider: "github", Repo: "group/project", Remote: "origin", RemoteURL: bare, APIBaseURL: server.URL, BaseBranch: "main"}
	result, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{RepoPath: repo, Branch: "sidecar/task-1"})
	require.NoError(t, err)
	assert.True(t, result.Reused)
	assert.Equal(t, "https://github.example/group/project/pull/9", result.URL)
}

func TestGitLabPublisherReportsLookupFailureAfterPush(t *testing.T) {
	repo, bare := deliveryRepo(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	target := output.DeliveryTarget{Provider: "gitlab", Repo: "group/project", Remote: "origin", RemoteURL: bare, APIBaseURL: server.URL, BaseBranch: "main"}
	result, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{RepoPath: repo, Branch: "sidecar/task-1"})
	assert.True(t, result.Pushed)
	var deliveryErr *output.DeliveryError
	require.ErrorAs(t, err, &deliveryErr)
	assert.Equal(t, "lookup", deliveryErr.Phase)
}

func TestPublisherReportsPushFailurePhase(t *testing.T) {
	repo, _ := deliveryRepo(t)
	target := output.DeliveryTarget{Provider: "github", Repo: "group/project", Remote: "missing", RemoteURL: "git@github.com:group/project.git", BaseBranch: "main"}
	_, err := output.NewPublisher(target).Publish(context.Background(), output.PublishRequest{RepoPath: repo, Branch: "sidecar/task-1"})
	var deliveryErr *output.DeliveryError
	require.ErrorAs(t, err, &deliveryErr)
	assert.Equal(t, "push", deliveryErr.Phase)
	assert.NotContains(t, strings.ToLower(err.Error()), "token")
}
