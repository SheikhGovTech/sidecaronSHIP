package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Workspace     WorkspaceConfig      `yaml:"workspace"`
	Signals       []SignalConfig       `yaml:"signals"`
	Autonomy      AutonomyPolicy       `yaml:"autonomy"`
	Models        ModelConfig          `yaml:"models"`
	Scope         ScopeConfig          `yaml:"scope"`
	Embedding     EmbeddingConfig      `yaml:"embedding"`
	Notifications []NotificationConfig `yaml:"notifications"`
	Verification  VerificationConfig   `yaml:"verification"`
	Skills        SkillsConfig         `yaml:"skills"`
	Budget        BudgetConfig         `yaml:"budget"`
	Delivery      DeliveryConfig       `yaml:"delivery"`
	Observability ObservabilityConfig  `yaml:"observability"`
	Workflow      WorkflowConfig       `yaml:"workflow"`
}

type WorkflowConfig struct {
	Evaluator EvaluatorWorkflowConfig `yaml:"evaluator"`
}

type EvaluatorWorkflowConfig struct {
	MaxTurns int    `yaml:"max_turns"`
	OnError  string `yaml:"on_error"`
}

const (
	DefaultEvaluatorMaxTurns = 20
	MaxEvaluatorMaxTurns     = 50
)

func (c *Config) EvaluatorMaxTurns() int {
	if c.Workflow.Evaluator.MaxTurns == 0 {
		return DefaultEvaluatorMaxTurns
	}
	return c.Workflow.Evaluator.MaxTurns
}

func (c *Config) EvaluatorOnError() string {
	if c.Workflow.Evaluator.OnError == "" {
		return "suggest"
	}
	return c.Workflow.Evaluator.OnError
}

type ObservabilityConfig struct {
	AgentTraces AgentTraceConfig `yaml:"agent_traces"`
}

type AgentTraceConfig struct {
	Enabled              *bool  `yaml:"enabled"`
	CaptureAssistantText string `yaml:"capture_assistant_text"`
	CaptureToolArguments string `yaml:"capture_tool_arguments"`
	CaptureToolOutput    string `yaml:"capture_tool_output"`
	OutputLimit          int    `yaml:"output_limit"`
	RetentionDays        int    `yaml:"retention_days"`
	LogToolActivity      *bool  `yaml:"log_tool_activity"`
}

const (
	DefaultAgentTraceOutputLimit   = 16 * 1024
	MaxAgentTraceOutputLimit       = 64 * 1024
	DefaultAgentTraceRetentionDays = 30
	MaxAgentTraceRetentionDays     = 365
)

func (c *Config) EffectiveAgentTraces() AgentTraceConfig {
	cfg := c.Observability.AgentTraces
	if cfg.Enabled == nil {
		value := true
		cfg.Enabled = &value
	}
	if cfg.CaptureAssistantText == "" {
		cfg.CaptureAssistantText = "final-only"
	}
	if cfg.CaptureToolArguments == "" {
		cfg.CaptureToolArguments = "sanitized"
	}
	if cfg.CaptureToolOutput == "" {
		cfg.CaptureToolOutput = "bounded"
	}
	if cfg.OutputLimit == 0 {
		cfg.OutputLimit = DefaultAgentTraceOutputLimit
	}
	if cfg.RetentionDays == 0 {
		cfg.RetentionDays = DefaultAgentTraceRetentionDays
	}
	if cfg.LogToolActivity == nil {
		value := true
		cfg.LogToolActivity = &value
	}
	return cfg
}

func (c *Config) ValidateWorkflowAndObservability() error {
	if turns := c.Workflow.Evaluator.MaxTurns; turns < 0 || turns > MaxEvaluatorMaxTurns {
		return fmt.Errorf("workflow.evaluator.max_turns must be between 1 and %d when set", MaxEvaluatorMaxTurns)
	}
	if action := c.Workflow.Evaluator.OnError; action != "" && action != "suggest" && action != "fail" && action != "draft-change-request" {
		return fmt.Errorf("workflow.evaluator.on_error must be suggest, fail, or draft-change-request")
	}
	traces := c.EffectiveAgentTraces()
	if traces.CaptureAssistantText != "none" && traces.CaptureAssistantText != "final-only" && traces.CaptureAssistantText != "all-visible" {
		return fmt.Errorf("observability.agent_traces.capture_assistant_text is invalid")
	}
	if traces.CaptureToolArguments != "none" && traces.CaptureToolArguments != "sanitized" {
		return fmt.Errorf("observability.agent_traces.capture_tool_arguments is invalid")
	}
	if traces.CaptureToolOutput != "none" && traces.CaptureToolOutput != "metadata" && traces.CaptureToolOutput != "bounded" {
		return fmt.Errorf("observability.agent_traces.capture_tool_output is invalid")
	}
	if traces.OutputLimit <= 0 || traces.OutputLimit > MaxAgentTraceOutputLimit {
		return fmt.Errorf("observability.agent_traces.output_limit must be between 1 and %d bytes", MaxAgentTraceOutputLimit)
	}
	if traces.RetentionDays < 1 || traces.RetentionDays > MaxAgentTraceRetentionDays {
		return fmt.Errorf("observability.agent_traces.retention_days must be between 1 and %d", MaxAgentTraceRetentionDays)
	}
	return nil
}

// DeliveryConfig controls where approved pull-request changes are published.
// Empty fields are resolved from the configured Git remote when unambiguous.
type DeliveryConfig struct {
	Provider   string `yaml:"provider"`
	Repo       string `yaml:"repo"`
	Remote     string `yaml:"remote"`
	APIBaseURL string `yaml:"api_base_url"`
	Token      string `yaml:"token"`
	BaseBranch string `yaml:"base_branch"`
}

func (d DeliveryConfig) ResolveToken() string {
	if strings.HasPrefix(d.Token, "$") {
		return os.Getenv(strings.TrimPrefix(d.Token, "$"))
	}
	return d.Token
}

func (d DeliveryConfig) Validate() error {
	if d.Provider != "" && d.Provider != "github" && d.Provider != "gitlab" {
		return fmt.Errorf("delivery provider must be github or gitlab")
	}
	for field, value := range map[string]string{"repo": d.Repo, "remote": d.Remote, "base_branch": d.BaseBranch} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("delivery %s contains a newline", field)
		}
	}
	if d.Repo != "" && !regexp.MustCompile(`^[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+$`).MatchString(d.Repo) {
		return fmt.Errorf("delivery repo must be an owner/repository slug")
	}
	if d.Provider == "github" && d.Repo != "" && strings.Count(d.Repo, "/") != 1 {
		return fmt.Errorf("GitHub delivery repo must be owner/repository")
	}
	if d.Remote != "" && (!regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`).MatchString(d.Remote) || strings.Contains(d.Remote, "..")) {
		return fmt.Errorf("delivery remote is invalid")
	}
	if d.BaseBranch != "" && (strings.HasPrefix(d.BaseBranch, "-") || strings.ContainsAny(d.BaseBranch, " \t~^:?*[\\") || strings.Contains(d.BaseBranch, "..")) {
		return fmt.Errorf("delivery base_branch is invalid")
	}
	if d.Token == "$" {
		return fmt.Errorf("delivery token environment reference is empty")
	}
	if d.APIBaseURL != "" {
		u, err := url.Parse(d.APIBaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("delivery api_base_url must be an absolute HTTPS URL")
		}
	}
	return nil
}

// VerificationConfig controls deterministic commands and the adversarial
// evaluator gate. When Enabled is false, both are disabled.
type VerificationConfig struct {
	// Enabled gates auto-commit and pull-request changes behind the
	// evaluator. Pointer so an absent value defaults to true.
	Enabled  *bool                 `yaml:"enabled"`
	Commands []VerificationCommand `yaml:"commands"`
}

type VerificationCommand struct {
	Name             string   `yaml:"name"`
	Run              string   `yaml:"run"`
	RequiredTools    []string `yaml:"required_tools"`
	Timeout          string   `yaml:"timeout"`
	WorkingDirectory string   `yaml:"working_directory"`
	PassEnv          []string `yaml:"pass_env"`
}

const (
	DefaultVerificationTimeout = 10 * time.Minute
	MaxVerificationTimeout     = 30 * time.Minute
)

func (c VerificationCommand) ParsedTimeout() time.Duration {
	if c.Timeout == "" {
		return DefaultVerificationTimeout
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func (c *Config) ValidateVerification() error {
	seen := map[string]bool{}
	for i, cmd := range c.Verification.Commands {
		if strings.TrimSpace(cmd.Name) == "" {
			return fmt.Errorf("verification command %d: name is required", i)
		}
		if seen[cmd.Name] {
			return fmt.Errorf("verification command %q: duplicate name", cmd.Name)
		}
		seen[cmd.Name] = true
		if strings.TrimSpace(cmd.Run) == "" {
			return fmt.Errorf("verification command %q: run is required", cmd.Name)
		}
		d := cmd.ParsedTimeout()
		if d == 0 || d > MaxVerificationTimeout {
			return fmt.Errorf("verification command %q: timeout must be >0 and <=%s", cmd.Name, MaxVerificationTimeout)
		}
		wd := cmd.WorkingDirectory
		if filepath.IsAbs(wd) {
			return fmt.Errorf("verification command %q: working_directory must be relative", cmd.Name)
		}
		if wd != "" && wd != "." {
			for _, segment := range strings.FieldsFunc(wd, func(r rune) bool {
				return r == '/' || r == '\\'
			}) {
				if segment == ".." {
					return fmt.Errorf("verification command %q: working_directory contains '..'", cmd.Name)
				}
			}
			clean := filepath.Clean(wd)
			if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return fmt.Errorf("verification command %q: working_directory escapes workspace", cmd.Name)
			}
		}
		for _, tool := range cmd.RequiredTools {
			if strings.TrimSpace(tool) == "" || strings.TrimSpace(tool) != tool || filepath.Base(tool) != tool {
				return fmt.Errorf("verification command %q: invalid required_tool %q", cmd.Name, tool)
			}
		}
		for _, name := range cmd.PassEnv {
			if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name || strings.Contains(name, "=") {
				return fmt.Errorf("verification command %q: invalid pass_env name %q", cmd.Name, name)
			}
		}
	}
	return nil
}

// VerificationEnabled reports whether the evaluator gate is on. Defaults to
// true when unset — a default-off gate would not close the Nodding Loop.
func (c *Config) VerificationEnabled() bool {
	if c.Verification.Enabled == nil {
		return true
	}
	return *c.Verification.Enabled
}

// SkillsConfig points the loop at a directory of SKILL.md files in the target repo.
type SkillsConfig struct {
	Dir string `yaml:"dir"`
}

// SkillsDir returns the configured skills directory (relative to the repo root),
// defaulting to ".sidecar/skills".
func (c *Config) SkillsDir() string {
	if c.Skills.Dir == "" {
		return ".sidecar/skills"
	}
	return c.Skills.Dir
}

// BudgetConfig caps autonomous spend.
type BudgetConfig struct {
	// DailyTokens is the per-workspace per-UTC-day token ceiling
	// (input+output). 0 means unlimited.
	DailyTokens int `yaml:"daily_tokens"`
}

// DailyTokenBudget returns the daily token ceiling; 0 means unlimited.
func (c *Config) DailyTokenBudget() int {
	return c.Budget.DailyTokens
}

type NotificationConfig struct {
	Provider string      `yaml:"provider"` // "slack" | "webhook" | "email"
	Webhook  string      `yaml:"webhook"`  // Slack: literal URL or $ENV_VAR
	URL      string      `yaml:"url"`      // generic webhook: literal URL or $ENV_VAR
	Email    EmailConfig `yaml:"email"`    // email (SMTP) settings
	On       []string    `yaml:"on"`       // events: skipped, suggested, completed, failed, notified
}

type EmailConfig struct {
	SMTPHost string   `yaml:"smtp_host"`
	SMTPPort int      `yaml:"smtp_port"` // 587 = STARTTLS (default), 465 = implicit TLS
	Username string   `yaml:"username"`  // literal or $ENV_VAR
	Password string   `yaml:"password"`  // literal or $ENV_VAR
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
}

// ResolveUsername returns the SMTP username, expanding $ENV_VAR references.
func (e EmailConfig) ResolveUsername() string {
	if len(e.Username) > 0 && e.Username[0] == '$' {
		return os.Getenv(e.Username[1:])
	}
	return e.Username
}

// ResolvePassword returns the SMTP password, expanding $ENV_VAR references.
func (e EmailConfig) ResolvePassword() string {
	if len(e.Password) > 0 && e.Password[0] == '$' {
		return os.Getenv(e.Password[1:])
	}
	return e.Password
}

// ResolveWebhook returns the Slack webhook URL, expanding $ENV_VAR references.
func (n NotificationConfig) ResolveWebhook() string {
	if len(n.Webhook) > 0 && n.Webhook[0] == '$' {
		return os.Getenv(n.Webhook[1:])
	}
	return n.Webhook
}

// ResolveURL returns the generic webhook URL, expanding $ENV_VAR references.
func (n NotificationConfig) ResolveURL() string {
	if len(n.URL) > 0 && n.URL[0] == '$' {
		return os.Getenv(n.URL[1:])
	}
	return n.URL
}

type EmbeddingConfig struct {
	Provider   string `yaml:"provider"`    // "openai" | "voyage" | "cohere"
	Model      string `yaml:"model"`       // provider model, optional
	BaseURL    string `yaml:"base_url"`    // optional compatible gateway endpoint
	APIKeyEnv  string `yaml:"api_key_env"` // env var containing provider credential
	Dimensions int    `yaml:"dimensions"`  // default 1024
}

type LogsSignalConfig struct {
	Files     []LogFile     `yaml:"files"`
	Processes []LogProcess  `yaml:"processes"`
	Patterns  []LogPattern  `yaml:"patterns"`
	Rate      LogRateConfig `yaml:"rate"`
}

type LogFile struct {
	Path string `yaml:"path"`
}
type LogProcess struct {
	Command string `yaml:"command"`
}

type LogPattern struct {
	Match       string `yaml:"match"`
	QuietPeriod string `yaml:"quiet_period"`
}

type LogRateConfig struct {
	Window      string `yaml:"window"`
	Threshold   int    `yaml:"threshold"`
	QuietPeriod string `yaml:"quiet_period"`
}

type MetricsSignalConfig struct {
	Provider   string   `yaml:"provider"`    // "datadog" | "prometheus"
	Endpoint   string   `yaml:"endpoint"`    // Prometheus base URL, e.g. "http://localhost:9090"
	Tags       []string `yaml:"tags"`        // Datadog: filter monitors by tags
	AlertNames []string `yaml:"alert_names"` // optional allowlist; empty = all alerts
}

type UptimeSignalConfig struct {
	Endpoints []UptimeEndpoint `yaml:"endpoints"`
}

type UptimeEndpoint struct {
	URL          string             `yaml:"url"`
	Timeout      string             `yaml:"timeout"`       // e.g. "5s"; default 10s
	ExpectStatus int                `yaml:"expect_status"` // default 200
	ExpectMaxMs  int                `yaml:"expect_max_ms"` // latency threshold; 0 = disabled
	Diagnostics  []UptimeDiagnostic `yaml:"diagnostics"`   // empty = auto (dns, tcp, tls for https)
}

// UptimeDiagnostic defines one diagnostic check to run on failure.
// Built-in checks: dns, tcp, tls, ping, http, cross, shell.
type UptimeDiagnostic struct {
	Check   string `yaml:"check"`   // built-in check name or "shell" / "http"
	URL     string `yaml:"url"`     // for check: http — alternate URL to probe
	Command string `yaml:"command"` // for check: shell — command to run
}

type WorkspaceConfig struct {
	Name     string `yaml:"name"`
	Language string `yaml:"language"`
}

type SignalConfig struct {
	Adapter       string              `yaml:"adapter"`
	Watch         []string            `yaml:"watch"`
	Cron          string              `yaml:"cron"`
	Repo          string              `yaml:"repo"`           // owner/repo slug (github-ci adapter)
	Token         string              `yaml:"token"`          // literal or $ENV_VAR reference
	PollInterval  string              `yaml:"poll_interval"`  // e.g. "60s", default "60s"
	ErrorPatterns []string            `yaml:"error_patterns"` // extra patterns for CI log extraction
	Logs          LogsSignalConfig    `yaml:"logs"`
	Metrics       MetricsSignalConfig `yaml:"metrics"`
	Uptime        UptimeSignalConfig  `yaml:"uptime"`
}

type AutonomyPolicy struct {
	DependencyUpdates string `yaml:"dependency_updates"`
	TestFixes         string `yaml:"test_fixes"`
	BugFixes          string `yaml:"bug_fixes"`
	Refactoring       string `yaml:"refactoring"`
	SchemaChanges     string `yaml:"schema_changes"`
	LogFixes          string `yaml:"log_fixes"`
	MetricFixes       string `yaml:"metric_fixes"`
	UptimeFixes       string `yaml:"uptime_fixes"`
}

type ModelConfig struct {
	Planning  string `yaml:"planning"`
	Coding    string `yaml:"coding"`
	Triage    string `yaml:"triage"`
	Evaluator string `yaml:"evaluator"`
}

type ScopeConfig struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// ParsedPollInterval returns the poll interval as a time.Duration.
// Returns 60 seconds if PollInterval is empty or unparseable.
func (s SignalConfig) ParsedPollInterval() time.Duration {
	if s.PollInterval == "" {
		return 60 * time.Second
	}
	d, err := time.ParseDuration(s.PollInterval)
	if err != nil || d <= 0 {
		return 60 * time.Second
	}
	return d
}

// ResolveToken returns the resolved token value.
// If Token starts with "$", it is treated as an environment variable name
// and resolved via os.Getenv. Otherwise the literal value is returned.
func (s SignalConfig) ResolveToken() string {
	if len(s.Token) > 0 && s.Token[0] == '$' {
		return os.Getenv(s.Token[1:])
	}
	return s.Token
}

var validAutonomyLevels = map[string]bool{
	"auto-commit":  true,
	"pull-request": true,
	"suggest-only": true,
	"notify":       true,
}

func ValidAutonomyLevel(s string) bool {
	return validAutonomyLevels[s]
}

func (c *Config) ValidateAutonomy() error {
	levels := map[string]string{
		"dependency_updates": c.Autonomy.DependencyUpdates, "test_fixes": c.Autonomy.TestFixes,
		"bug_fixes": c.Autonomy.BugFixes, "refactoring": c.Autonomy.Refactoring,
		"schema_changes": c.Autonomy.SchemaChanges, "log_fixes": c.Autonomy.LogFixes,
		"metric_fixes": c.Autonomy.MetricFixes, "uptime_fixes": c.Autonomy.UptimeFixes,
	}
	for field, level := range levels {
		if level != "" && !ValidAutonomyLevel(level) {
			return fmt.Errorf("autonomy.%s has invalid level %q", field, level)
		}
	}
	return nil
}

// Load reads, parses, and validates configuration.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}
	if err := cfg.ValidateVerification(); err != nil {
		return nil, fmt.Errorf("validating config %q: %w", path, err)
	}
	if err := cfg.ValidateAutonomy(); err != nil {
		return nil, fmt.Errorf("validating config %q: %w", path, err)
	}
	if err := cfg.Delivery.Validate(); err != nil {
		return nil, fmt.Errorf("validating config %q: %w", path, err)
	}
	if err := cfg.ValidateWorkflowAndObservability(); err != nil {
		return nil, fmt.Errorf("validating config %q: %w", path, err)
	}
	return &cfg, nil
}
