package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/sidecar/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	yaml := `
workspace:
  name: my-service
  language: go
signals:
  - adapter: git
    watch: [push, pr]
  - adapter: schedule
    cron: "0 2 * * *"
autonomy:
  dependency_updates: auto-commit
  test_fixes: auto-commit
  bug_fixes: pull-request
  refactoring: suggest-only
  schema_changes: suggest-only
models:
  planning: anthropic/claude-sonnet-4-6
  coding: anthropic/claude-sonnet-4-6
  triage: anthropic/claude-haiku-4-5
scope:
  include: [src/, tests/]
  exclude: [secrets/]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0644))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "my-service", cfg.Workspace.Name)
	assert.Equal(t, "go", cfg.Workspace.Language)
	assert.Len(t, cfg.Signals, 2)
	assert.Equal(t, "git", cfg.Signals[0].Adapter)
	assert.Equal(t, []string{"push", "pr"}, cfg.Signals[0].Watch)
	assert.Equal(t, "schedule", cfg.Signals[1].Adapter)
	assert.Equal(t, "0 2 * * *", cfg.Signals[1].Cron)
	assert.Equal(t, "auto-commit", cfg.Autonomy.DependencyUpdates)
	assert.Equal(t, "pull-request", cfg.Autonomy.BugFixes)
	assert.Equal(t, "anthropic/claude-haiku-4-5", cfg.Models.Triage)
	assert.Equal(t, []string{"src/", "tests/"}, cfg.Scope.Include)
	assert.Equal(t, []string{"secrets/"}, cfg.Scope.Exclude)
}

func TestLoadRepositoryExample(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "sidecar.yaml"))
	require.NoError(t, err)
	assert.Equal(t, 20, cfg.EvaluatorMaxTurns())
	assert.True(t, *cfg.EffectiveAgentTraces().Enabled)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/does/not/exist/sidecar.yaml")
	assert.Error(t, err)
}

func TestLoad_GitHubCI(t *testing.T) {
	yaml := `
signals:
  - adapter: github-ci
    repo: myorg/payment-service
    token: $GITHUB_TOKEN
    poll_interval: 60s
    watch: [failure]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0644))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	require.Len(t, cfg.Signals, 1)
	s := cfg.Signals[0]
	assert.Equal(t, "github-ci", s.Adapter)
	assert.Equal(t, "myorg/payment-service", s.Repo)
	assert.Equal(t, "$GITHUB_TOKEN", s.Token)
	assert.Equal(t, "60s", s.PollInterval)
	assert.Equal(t, []string{"failure"}, s.Watch)
}

func TestAutonomyLevel_Valid(t *testing.T) {
	cases := []string{"auto-commit", "pull-request", "suggest-only"}
	for _, c := range cases {
		assert.True(t, config.ValidAutonomyLevel(c), "expected %q to be valid", c)
	}
	assert.False(t, config.ValidAutonomyLevel("invalid"))
}

func TestSignalConfig_ParsedPollInterval(t *testing.T) {
	assert.Equal(t, 60*time.Second, config.SignalConfig{}.ParsedPollInterval())                        // empty → default
	assert.Equal(t, 30*time.Second, config.SignalConfig{PollInterval: "30s"}.ParsedPollInterval())     // valid
	assert.Equal(t, 60*time.Second, config.SignalConfig{PollInterval: "invalid"}.ParsedPollInterval()) // bad → default
	assert.Equal(t, 60*time.Second, config.SignalConfig{PollInterval: "-5s"}.ParsedPollInterval())     // negative → default
}

func TestSignalConfig_ResolveToken(t *testing.T) {
	t.Setenv("MY_TEST_TOKEN", "secret123")
	assert.Equal(t, "secret123", config.SignalConfig{Token: "$MY_TEST_TOKEN"}.ResolveToken())    // env var
	assert.Equal(t, "literal-token", config.SignalConfig{Token: "literal-token"}.ResolveToken()) // literal
	assert.Equal(t, "", config.SignalConfig{Token: ""}.ResolveToken())                           // empty
}

func TestLoad_Embedding(t *testing.T) {
	yaml := `
embedding:
  provider: openai
  model: text-embedding-3-small
`
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0644))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "openai", cfg.Embedding.Provider)
	assert.Equal(t, "text-embedding-3-small", cfg.Embedding.Model)
}

func TestLoad_Embedding_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.yaml")
	require.NoError(t, os.WriteFile(path, []byte("workspace:\n  name: test\n"), 0644))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "", cfg.Embedding.Provider)
}

func TestVerificationEnabled_DefaultsTrueWhenAbsent(t *testing.T) {
	cfg := &config.Config{}
	assert.True(t, cfg.VerificationEnabled())
}

func TestVerificationEnabled_RespectsExplicitFalse(t *testing.T) {
	f := false
	cfg := &config.Config{Verification: config.VerificationConfig{Enabled: &f}}
	assert.False(t, cfg.VerificationEnabled())
}

func TestVerificationEnabled_RespectsExplicitTrue(t *testing.T) {
	tr := true
	cfg := &config.Config{Verification: config.VerificationConfig{Enabled: &tr}}
	assert.True(t, cfg.VerificationEnabled())
}

func TestAgentTraceDefaults(t *testing.T) {
	cfg := (&config.Config{}).EffectiveAgentTraces()
	require.NotNil(t, cfg.Enabled)
	assert.True(t, *cfg.Enabled)
	assert.Equal(t, "final-only", cfg.CaptureAssistantText)
	assert.Equal(t, "sanitized", cfg.CaptureToolArguments)
	assert.Equal(t, "bounded", cfg.CaptureToolOutput)
	assert.Equal(t, 16*1024, cfg.OutputLimit)
	assert.Equal(t, 30, cfg.RetentionDays)
}

func TestWorkflowEvaluatorDefaults(t *testing.T) {
	cfg := &config.Config{}
	assert.Equal(t, 20, cfg.EvaluatorMaxTurns())
	assert.Equal(t, "suggest", cfg.EvaluatorOnError())
}

func TestValidateWorkflowAndObservability(t *testing.T) {
	tests := []config.Config{
		{Workflow: config.WorkflowConfig{Evaluator: config.EvaluatorWorkflowConfig{MaxTurns: 51}}},
		{Workflow: config.WorkflowConfig{Evaluator: config.EvaluatorWorkflowConfig{OnError: "approve"}}},
		{Observability: config.ObservabilityConfig{AgentTraces: config.AgentTraceConfig{CaptureAssistantText: "thoughts"}}},
		{Observability: config.ObservabilityConfig{AgentTraces: config.AgentTraceConfig{OutputLimit: 65 * 1024}}},
		{Observability: config.ObservabilityConfig{AgentTraces: config.AgentTraceConfig{RetentionDays: 366}}},
	}
	for _, cfg := range tests {
		assert.Error(t, cfg.ValidateWorkflowAndObservability())
	}
	assert.NoError(t, (&config.Config{}).ValidateWorkflowAndObservability())
}

func TestValidateAutonomyRejectsUnknownLevel(t *testing.T) {
	cfg := &config.Config{Autonomy: config.AutonomyPolicy{BugFixes: "ship-it"}}
	err := cfg.ValidateAutonomy()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "autonomy.bug_fixes")
}

func TestLoad_VerificationCommandsAndDefaults(t *testing.T) {
	yaml := `
verification:
  enabled: true
  commands:
    - name: backend-tests
      run: go test ./...
      required_tools: [go]
      working_directory: backend
      pass_env: [PATH, HOME]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Verification.Commands, 1)
	cmd := cfg.Verification.Commands[0]
	assert.Equal(t, config.DefaultVerificationTimeout, cmd.ParsedTimeout())
	assert.Equal(t, "backend", cmd.WorkingDirectory)
	assert.Equal(t, []string{"go"}, cmd.RequiredTools)
}

func TestValidateVerificationRejectsInvalidCommands(t *testing.T) {
	tests := map[string][]config.VerificationCommand{
		"empty name":      {{Run: "true"}},
		"blank name":      {{Name: "  ", Run: "true"}},
		"empty run":       {{Name: "test"}},
		"duplicate name":  {{Name: "test", Run: "true"}, {Name: "test", Run: "true"}},
		"bad duration":    {{Name: "test", Run: "true", Timeout: "forever"}},
		"too long":        {{Name: "test", Run: "true", Timeout: "31m"}},
		"absolute path":   {{Name: "test", Run: "true", WorkingDirectory: "/tmp"}},
		"parent path":     {{Name: "test", Run: "true", WorkingDirectory: "../outside"}},
		"embedded parent": {{Name: "test", Run: "true", WorkingDirectory: "backend/../frontend"}},
		"tool path":       {{Name: "test", Run: "true", RequiredTools: []string{"/bin/go"}}},
		"invalid env":     {{Name: "test", Run: "true", PassEnv: []string{"A=B"}}},
	}
	for name, commands := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{Verification: config.VerificationConfig{Commands: commands}}
			assert.Error(t, cfg.ValidateVerification())
		})
	}
}

func TestSkillsDir_DefaultWhenEmpty(t *testing.T) {
	cfg := &config.Config{}
	assert.Equal(t, ".sidecar/skills", cfg.SkillsDir())
}

func TestSkillsDir_RespectsConfigured(t *testing.T) {
	cfg := &config.Config{Skills: config.SkillsConfig{Dir: "ops/skills"}}
	assert.Equal(t, "ops/skills", cfg.SkillsDir())
}

func TestDailyTokenBudget_DefaultsZero(t *testing.T) {
	cfg := &config.Config{}
	assert.Equal(t, 0, cfg.DailyTokenBudget())
}

func TestDailyTokenBudget_RespectsConfigured(t *testing.T) {
	cfg := &config.Config{Budget: config.BudgetConfig{DailyTokens: 500000}}
	assert.Equal(t, 500000, cfg.DailyTokenBudget())
}

func TestLoadDelivery(t *testing.T) {
	t.Setenv("DELIVERY_TEST_TOKEN", "secret")
	yaml := `
delivery:
  provider: gitlab
  repo: group/project
  remote: origin
  api_base_url: https://gitlab.example.gov/api/v4
  token: $DELIVERY_TEST_TOKEN
  base_branch: main
`
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "gitlab", cfg.Delivery.Provider)
	assert.Equal(t, "secret", cfg.Delivery.ResolveToken())
}

func TestDeliveryValidation(t *testing.T) {
	assert.Error(t, (config.DeliveryConfig{Provider: "bitbucket"}).Validate())
	assert.Error(t, (config.DeliveryConfig{APIBaseURL: "http://insecure.example"}).Validate())
	assert.Error(t, (config.DeliveryConfig{Remote: "origin\nmalicious"}).Validate())
	assert.Error(t, (config.DeliveryConfig{Repo: "missing-owner"}).Validate())
	assert.Error(t, (config.DeliveryConfig{Remote: "-bad"}).Validate())
	assert.Error(t, (config.DeliveryConfig{BaseBranch: "../main"}).Validate())
	assert.NoError(t, (config.DeliveryConfig{Provider: "github", APIBaseURL: "https://github.example/api/v3"}).Validate())
}
