package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sausheong/sidecar/internal/config"
)

const gitlabAPIBase = "https://gitlab.com/api/v4"

type DeliveryTarget struct {
	Provider   string
	Repo       string
	Remote     string
	RemoteURL  string
	APIBaseURL string
	Token      string
	BaseBranch string
}

type PublishRequest struct {
	RepoPath string
	Branch   string
	Title    string
	Body     string
}

type PublishResult struct {
	Provider string
	URL      string
	Pushed   bool
	Reused   bool
}

type DeliveryError struct {
	Phase string
	Err   error
}

func (e *DeliveryError) Error() string { return fmt.Sprintf("delivery %s: %v", e.Phase, e.Err) }
func (e *DeliveryError) Unwrap() error { return e.Err }

type Publisher interface {
	Publish(context.Context, PublishRequest) (PublishResult, error)
}

func ResolveDelivery(cfg config.DeliveryConfig, repoPath, signalSource, fallbackRepo, fallbackToken string) (DeliveryTarget, error) {
	remote := cfg.Remote
	if remote == "" {
		remote = "origin"
	}
	remoteOut, remoteErr := exec.Command("git", "-C", repoPath, "remote", "get-url", remote).Output()
	remoteURL := strings.TrimSpace(string(remoteOut))
	host, remoteRepo, parseErr := ParseRemoteURL(remoteURL)

	provider := cfg.Provider
	if provider == "" && parseErr == nil {
		provider = providerForHost(host)
	}
	if provider == "" {
		switch signalSource {
		case "github-ci":
			provider = "github"
		case "gitlab-ci":
			provider = "gitlab"
		}
	}
	if provider != "github" && provider != "gitlab" {
		return DeliveryTarget{}, &DeliveryError{Phase: "resolve", Err: fmt.Errorf("cannot resolve delivery provider")}
	}

	repo := cfg.Repo
	if repo == "" && remoteRepo != "" {
		repo = remoteRepo
	}
	if repo == "" {
		repo = fallbackRepo
	}
	if repo == "" || strings.HasPrefix(repo, "/") || !strings.Contains(repo, "/") {
		return DeliveryTarget{}, &DeliveryError{Phase: "resolve", Err: fmt.Errorf("cannot resolve delivery repository")}
	}
	if provider == "github" && strings.Count(repo, "/") != 1 {
		return DeliveryTarget{}, &DeliveryError{Phase: "resolve", Err: fmt.Errorf("GitHub repository must be owner/repository")}
	}
	if remoteErr != nil {
		return DeliveryTarget{}, &DeliveryError{Phase: "resolve", Err: fmt.Errorf("resolving git remote %q: %w", remote, remoteErr)}
	}

	base := cfg.BaseBranch
	if base == "" {
		out, err := exec.Command("git", "-C", repoPath, "symbolic-ref", "refs/remotes/"+remote+"/HEAD", "--short").Output()
		if err != nil {
			return DeliveryTarget{}, &DeliveryError{Phase: "resolve", Err: fmt.Errorf("cannot resolve default branch; configure delivery.base_branch")}
		}
		base = strings.TrimSpace(string(out))
		base = strings.TrimPrefix(base, remote+"/")
	}
	if base == "" {
		return DeliveryTarget{}, &DeliveryError{Phase: "resolve", Err: fmt.Errorf("empty delivery base branch")}
	}

	apiBase := strings.TrimRight(cfg.APIBaseURL, "/")
	if apiBase == "" {
		if provider == "github" {
			apiBase = githubAPIBase
		} else {
			apiBase = gitlabAPIBase
		}
	}
	token := cfg.ResolveToken()
	if token == "" {
		token = fallbackToken
	}
	if token == "" {
		if provider == "github" {
			token = os.Getenv("GITHUB_TOKEN")
		} else {
			token = os.Getenv("GITLAB_TOKEN")
		}
	}

	return DeliveryTarget{Provider: provider, Repo: repo, Remote: remote, RemoteURL: remoteURL, APIBaseURL: apiBase, Token: token, BaseBranch: base}, nil
}

// ParseRemoteURL extracts host and owner/repository from HTTPS, ssh://, and
// scp-style Git remote URLs.
func ParseRemoteURL(raw string) (host, repo string, err error) {
	if raw == "" {
		return "", "", fmt.Errorf("empty remote URL")
	}
	path := ""
	if strings.Contains(raw, "://") {
		u, parseErr := url.Parse(raw)
		if parseErr != nil || u.Hostname() == "" {
			return "", "", fmt.Errorf("invalid remote URL")
		}
		host, path = strings.ToLower(u.Hostname()), u.Path
	} else if at := strings.LastIndex(raw, "@"); at >= 0 {
		rest := raw[at+1:]
		colon := strings.Index(rest, ":")
		if colon <= 0 {
			return "", "", fmt.Errorf("invalid scp-style remote URL")
		}
		host, path = strings.ToLower(rest[:colon]), rest[colon+1:]
	} else {
		return "", "", fmt.Errorf("unsupported remote URL")
	}
	path = strings.Trim(strings.TrimSuffix(path, ".git"), "/")
	if !strings.Contains(path, "/") {
		return "", "", fmt.Errorf("remote URL has no repository owner")
	}
	return host, path, nil
}

func providerForHost(host string) string {
	switch {
	case strings.Contains(host, "github"):
		return "github"
	case strings.Contains(host, "gitlab"):
		return "gitlab"
	default:
		return ""
	}
}

func NewPublisher(target DeliveryTarget) Publisher {
	base := providerPublisher{target: target, client: &http.Client{Timeout: 30 * time.Second}}
	if target.Provider == "gitlab" {
		return &gitLabPublisher{providerPublisher: base}
	}
	return &gitHubPublisher{providerPublisher: base}
}

type providerPublisher struct {
	target DeliveryTarget
	client *http.Client
}

func (p *providerPublisher) push(ctx context.Context, req PublishRequest) error {
	args := []string{"-C", req.RepoPath, "push", p.target.Remote, req.Branch}
	cmd := exec.CommandContext(ctx, "git", args...)
	if strings.HasPrefix(p.target.RemoteURL, "http://") || strings.HasPrefix(p.target.RemoteURL, "https://") {
		helper, err := writeEnvironmentAskpass()
		if err != nil {
			return err
		}
		defer os.Remove(helper)
		username := "x-access-token"
		if p.target.Provider == "gitlab" {
			username = "oauth2"
		}
		cmd.Env = append(os.Environ(), "GIT_ASKPASS="+helper, "GIT_TERMINAL_PROMPT=0", "SIDECAR_GIT_USERNAME="+username, "SIDECAR_GIT_TOKEN="+p.target.Token)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push failed: %w: %s", err, p.redact(strings.TrimSpace(string(out))))
	}
	return nil
}

func (p *providerPublisher) redact(value string) string {
	if p.target.Token != "" {
		value = strings.ReplaceAll(value, p.target.Token, "[REDACTED]")
	}
	return regexp.MustCompile(`https?://[^/@[:space:]]+@`).ReplaceAllString(value, "https://[REDACTED]@")
}

func writeEnvironmentAskpass() (string, error) {
	f, err := os.CreateTemp("", "sidecar-delivery-askpass-*.sh")
	if err != nil {
		return "", err
	}
	name := f.Name()
	script := "#!/bin/sh\ncase \"$1\" in\n  *Username*) printf '%s\\n' \"$SIDECAR_GIT_USERNAME\" ;;\n  *) printf '%s\\n' \"$SIDECAR_GIT_TOKEN\" ;;\nesac\n"
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Chmod(name, 0o700); err != nil {
		os.Remove(name)
		return "", err
	}
	return filepath.Clean(name), nil
}

type gitHubPublisher struct{ providerPublisher }

func (p *gitHubPublisher) Publish(ctx context.Context, req PublishRequest) (PublishResult, error) {
	result := PublishResult{Provider: "github"}
	if err := p.push(ctx, req); err != nil {
		return result, &DeliveryError{Phase: "push", Err: err}
	}
	result.Pushed = true
	owner := strings.SplitN(p.target.Repo, "/", 2)[0]
	endpoint := fmt.Sprintf("%s/repos/%s/pulls?state=open&head=%s%%3A%s&base=%s", p.target.APIBaseURL, p.target.Repo, url.QueryEscape(owner), url.QueryEscape(req.Branch), url.QueryEscape(p.target.BaseBranch))
	var existing []struct {
		HTMLURL string `json:"html_url"`
	}
	if err := p.api(ctx, http.MethodGet, endpoint, nil, &existing); err != nil {
		return result, &DeliveryError{Phase: "lookup", Err: err}
	}
	if len(existing) > 0 && existing[0].HTMLURL != "" {
		result.URL, result.Reused = existing[0].HTMLURL, true
		return result, nil
	}
	body := map[string]string{"title": req.Title, "body": req.Body, "head": req.Branch, "base": p.target.BaseBranch}
	var created struct {
		HTMLURL string `json:"html_url"`
	}
	if err := p.api(ctx, http.MethodPost, fmt.Sprintf("%s/repos/%s/pulls", p.target.APIBaseURL, p.target.Repo), body, &created); err != nil {
		return result, &DeliveryError{Phase: "create", Err: err}
	}
	result.URL = created.HTMLURL
	if result.URL == "" {
		return result, &DeliveryError{Phase: "create", Err: fmt.Errorf("provider returned an empty pull-request URL")}
	}
	return result, nil
}

func (p *gitHubPublisher) api(ctx context.Context, method, endpoint string, body any, dst any) error {
	return providerAPI(ctx, p.client, method, endpoint, body, dst, "Authorization", "Bearer "+p.target.Token)
}

type gitLabPublisher struct{ providerPublisher }

func (p *gitLabPublisher) Publish(ctx context.Context, req PublishRequest) (PublishResult, error) {
	result := PublishResult{Provider: "gitlab"}
	if err := p.push(ctx, req); err != nil {
		return result, &DeliveryError{Phase: "push", Err: err}
	}
	result.Pushed = true
	project := url.PathEscape(p.target.Repo)
	query := url.Values{"state": {"opened"}, "source_branch": {req.Branch}, "target_branch": {p.target.BaseBranch}}
	endpoint := fmt.Sprintf("%s/projects/%s/merge_requests?%s", p.target.APIBaseURL, project, query.Encode())
	var existing []struct {
		WebURL string `json:"web_url"`
	}
	if err := p.api(ctx, http.MethodGet, endpoint, nil, &existing); err != nil {
		return result, &DeliveryError{Phase: "lookup", Err: err}
	}
	if len(existing) > 0 && existing[0].WebURL != "" {
		result.URL, result.Reused = existing[0].WebURL, true
		return result, nil
	}
	body := map[string]string{"title": req.Title, "description": req.Body, "source_branch": req.Branch, "target_branch": p.target.BaseBranch}
	var created struct {
		WebURL string `json:"web_url"`
	}
	if err := p.api(ctx, http.MethodPost, fmt.Sprintf("%s/projects/%s/merge_requests", p.target.APIBaseURL, project), body, &created); err != nil {
		return result, &DeliveryError{Phase: "create", Err: err}
	}
	result.URL = created.WebURL
	if result.URL == "" {
		return result, &DeliveryError{Phase: "create", Err: fmt.Errorf("provider returned an empty merge-request URL")}
	}
	return result, nil
}

func (p *gitLabPublisher) api(ctx context.Context, method, endpoint string, body any, dst any) error {
	return providerAPI(ctx, p.client, method, endpoint, body, dst, "PRIVATE-TOKEN", p.target.Token)
}

func providerAPI(ctx context.Context, client *http.Client, method, endpoint string, body any, dst any, authHeader, authValue string) error {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if authValue != "" {
		req.Header.Set(authHeader, authValue)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider API returned status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decoding provider response: %w", err)
	}
	return nil
}
