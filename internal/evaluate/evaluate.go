// Package evaluate implements the generator/evaluator split: a fresh skeptic
// runtime reviews the generator's diff and can REJECT it before any commit.
// This closes the "Nodding Loop" — an agent grading its own work.
package evaluate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/bash"
	"github.com/sausheong/harness/tools/file"
	"github.com/sausheong/sidecar/internal/changeset"
	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/verification"
)

// Verdict is the evaluator's decision on a diff.
type Verdict struct {
	Outcome  string
	Pass     bool
	Reasons  string
	Evidence []string
}

// IncompleteError means the evaluator runtime remained healthy but did not
// return a valid verdict within Sidecar's completion contract.
type IncompleteError struct{ Reason string }

func (e *IncompleteError) Error() string { return "evaluator incomplete: " + e.Reason }

func IsIncomplete(err error) bool {
	var target *IncompleteError
	return errors.As(err, &target)
}

const evaluatorSystemPrompt = `You are an adversarial code reviewer for an autonomous engineering system.
A generator agent just produced a diff. ASSUME THIS DIFF IS BROKEN until proven otherwise.
Do NOT praise. Do NOT trust the author's intent. Your job is to find what fails.

Check, in order:
1. Does the candidate satisfy the stated task?
2. Did deterministic verification pass, and is there concrete evidence it is unreliable?
3. Are changed paths in scope and free of a material correctness or safety regression?
4. Is there one targeted uncovered risk that requires an additional bounded check?

Treat supplied deterministic verification as authoritative evidence. Do not rerun
an equivalent command unless you identify a concrete inconsistency or uncovered
material risk. Do not investigate billing, unrelated history, or unrelated code.
As soon as the checks above are resolved, stop using tools and return the verdict.

Respond with ONLY valid JSON, no prose:
{"outcome":"reject","reasons":"one or two sentences citing the specific failure","evidence":["diff:path","verification:name"]}
outcome must be exactly "pass" or "reject". Keep reasons and evidence bounded.`

// SystemPrompt returns the evaluator system prompt. Exposed for tests.
func SystemPrompt() string { return evaluatorSystemPrompt }

// SystemPromptWithContext binds the evaluator to the same resolved workspace
// and deterministic commands as the coding agent.
func SystemPromptWithContext(workDir string, commands []config.VerificationCommand) string {
	prompt := evaluatorSystemPrompt
	if workDir != "" {
		prompt += fmt.Sprintf(`

Workspace root: %s
All filesystem and Bash tools execute from this directory.
Use relative paths. Do not change to or guess another repository path.
Run pwd before investigating.`, workDir)
	}
	if block := verification.PromptBlock(commands); block != "" {
		prompt += "\n\n" + block
	}
	return prompt
}

// BuildEvalMessage builds the user-turn message: the task plus the diff to judge.
func BuildEvalMessage(taskSummary, diff string) string {
	return BuildEvalMessageWithEvidence(taskSummary, diff, nil)
}

// BuildEvalMessageWithEvidence includes the already-executed deterministic
// checks so the evaluator can assess rather than repeat them.
func BuildEvalMessageWithEvidence(taskSummary, diff string, results []verification.Result) string {
	return BuildPreparedEvalMessage(taskSummary, diff, nil, results)
}

// BuildPreparedEvalMessage includes the authoritative path manifest and the
// already-executed deterministic checks alongside the staged patch.
func BuildPreparedEvalMessage(taskSummary, diff string, manifest []changeset.PathChange, results []verification.Result) string {
	evidence := VerificationEvidenceBlock(results)
	if evidence == "" {
		evidence = "No configured deterministic verification evidence was supplied."
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		manifestJSON = []byte("[]")
	}
	return fmt.Sprintf(`Task the generator was asked to do: %s

Approved changed-path manifest:
%s

Deterministic verification evidence:
%s

Review the candidate diff against the task and evidence. Run only a materially
different targeted check when a concrete unresolved risk requires it. Return
your verdict immediately once material checks are resolved.

--- DIFF ---
%s
--- END DIFF ---`, taskSummary, manifestJSON, evidence, diff)
}

// VerificationEvidenceBlock renders bounded structured evidence. Callers are
// responsible for redacting configured secrets before passing results here.
func VerificationEvidenceBlock(results []verification.Result) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	for _, result := range results {
		output := result.Output
		if len(output) > 4096 {
			output = output[:4096] + "\n[TRUNCATED FOR EVALUATOR]"
		}
		fmt.Fprintf(&b, "- name: %s\n  exit_code: %d\n  duration_ms: %d\n  truncated: %t\n  output: |\n",
			result.Name, result.ExitCode, result.Duration.Milliseconds(), result.Truncated)
		for _, line := range strings.Split(output, "\n") {
			fmt.Fprintf(&b, "    %s\n", line)
		}
		if b.Len() > 16*1024 {
			return b.String()[:16*1024] + "\n[VERIFICATION EVIDENCE TRUNCATED]"
		}
	}
	return strings.TrimSpace(b.String())
}

// ParseVerdict extracts the JSON verdict from the evaluator's response,
// tolerating code fences and surrounding prose.
func ParseVerdict(raw string) (Verdict, error) {
	raw = strings.TrimSpace(raw)
	if start := strings.Index(raw, "{"); start != -1 {
		if end := strings.LastIndex(raw, "}"); end != -1 && end >= start {
			raw = raw[start : end+1]
		}
	}
	var resp struct {
		Outcome  string   `json:"outcome"`
		Pass     *bool    `json:"pass"`
		Reasons  string   `json:"reasons"`
		Evidence []string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return Verdict{}, fmt.Errorf("parsing verdict JSON: %w", err)
	}
	outcome := resp.Outcome
	if outcome == "" && resp.Pass != nil { // backward-compatible provider response
		if *resp.Pass {
			outcome = "pass"
		} else {
			outcome = "reject"
		}
	}
	if outcome != "pass" && outcome != "reject" {
		return Verdict{}, fmt.Errorf("parsing verdict JSON: outcome must be pass or reject")
	}
	if len(resp.Reasons) > 4096 {
		resp.Reasons = resp.Reasons[:4096]
	}
	if len(resp.Evidence) > 20 {
		resp.Evidence = resp.Evidence[:20]
	}
	return Verdict{Outcome: outcome, Pass: outcome == "pass", Reasons: resp.Reasons, Evidence: resp.Evidence}, nil
}

// Evaluate builds a fresh skeptic runtime over the diff in workDir and returns
// its verdict. It does NOT inherit the generator's conversation — the evaluator
// must carry none of the generator's self-persuasion.
//
// The diff is taken against baseRef so it captures BOTH uncommitted changes and
// any commits the generator agent made on top of base (the agent has bash and
// can self-commit; diffing against HEAD would then yield an empty diff and
// silently bypass this gate). If baseRef is "" it falls back to "HEAD" to
// preserve the in-repo (non-worktree) behavior.
//
// The caller prepares the authoritative staged change set before evaluation.
// Evaluation is read-only with respect to the index and reviews exactly
// `git diff --cached <base>`.
//
// On any setup/run error the caller should fail closed (treat as REJECT); this
// function returns the error so the caller can record it.
//
// It also returns the token usage consumed by the evaluator runtime so the
// caller can meter it toward the daily budget; usage is zero on all early
// returns (git errors, trivial empty-diff pass, build/run errors, parse error).
func Evaluate(ctx context.Context, provider llm.LLMProvider, model, workDir, baseRef, taskSummary string) (Verdict, llm.Usage, error) {
	return EvaluateWithCommands(ctx, provider, model, workDir, baseRef, taskSummary, nil)
}

// EvaluateWithCommands evaluates the diff and gives the skeptic the same
// workspace and required verification contract as the coding agent.
func EvaluateWithCommands(ctx context.Context, provider llm.LLMProvider, model, workDir, baseRef, taskSummary string, commands []config.VerificationCommand) (Verdict, llm.Usage, error) {
	return EvaluateObserved(ctx, provider, model, workDir, baseRef, taskSummary, commands, 20, nil)
}

// EventObserver receives every evaluator runtime event before EvaluateObserved
// acts on it. Observers should durably persist the event and return an error if
// that persistence fails.
type EventObserver func(context.Context, runtime.AgentEvent) error

// EvaluateObserved evaluates a change while exposing the complete runtime
// event stream. It drains the stream after errors so request-level usage that
// precedes a terminal error is not lost.
func EvaluateObserved(ctx context.Context, provider llm.LLMProvider, model, workDir, baseRef, taskSummary string, commands []config.VerificationCommand, maxTurns int, observe EventObserver) (Verdict, llm.Usage, error) {
	return EvaluateObservedWithEvidence(ctx, provider, model, workDir, baseRef, taskSummary, commands, nil, maxTurns, observe)
}

// EvaluateObservedWithEvidence enforces two phases: a bounded investigation
// with guarded tools and one reserved, tool-free verdict request.
func EvaluateObservedWithEvidence(ctx context.Context, provider llm.LLMProvider, model, workDir, baseRef, taskSummary string, commands []config.VerificationCommand, evidence []verification.Result, maxTurns int, observe EventObserver) (Verdict, llm.Usage, error) {
	return EvaluatePreparedObservedWithEvidence(ctx, provider, model, workDir, baseRef, taskSummary, nil, commands, evidence, maxTurns, observe)
}

// EvaluatePreparedObservedWithEvidence evaluates the authoritative staged
// patch together with its approved changed-path manifest.
func EvaluatePreparedObservedWithEvidence(ctx context.Context, provider llm.LLMProvider, model, workDir, baseRef, taskSummary string, manifest []changeset.PathChange, commands []config.VerificationCommand, evidence []verification.Result, maxTurns int, observe EventObserver) (Verdict, llm.Usage, error) {
	if baseRef == "" {
		baseRef = "HEAD"
	}
	if maxTurns <= 0 {
		maxTurns = 20
	}
	diffOut, err := exec.Command("git", "-C", workDir, "diff", "--cached", baseRef).CombinedOutput()
	if err != nil {
		return Verdict{}, llm.Usage{}, fmt.Errorf("git diff: %w\n%s", err, strings.TrimSpace(string(diffOut)))
	}
	diff := strings.TrimSpace(string(diffOut))
	if diff == "" {
		// No diff to judge — nothing was shipped; treat as a trivial pass.
		return Verdict{Outcome: "pass", Pass: true, Reasons: "no changes to evaluate"}, llm.Usage{}, nil
	}

	message := BuildPreparedEvalMessage(taskSummary, diff, manifest, evidence)
	investigationTurns := maxTurns - 1
	if investigationTurns > 8 {
		investigationTurns = 8
	}
	if investigationTurns < 1 {
		investigationTurns = 1
	}

	guard := newToolGuard(commands, evidence)
	reg := tool.NewRegistry()
	reg.Register(&guardedTool{Tool: &file.ReadFileTool{WorkDir: workDir}, guard: guard})
	reg.Register(&guardedTool{Tool: &bash.BashTool{WorkDir: workDir}, guard: guard})
	investigation, err := runPhase(ctx, provider, model, workDir, "evaluate-investigate-"+sessionSuffix(taskSummary), SystemPromptWithContext(workDir, commands), message, reg, investigationTurns, guard, observe)
	usage := investigation.usage
	if err != nil {
		return Verdict{}, usage, err
	}
	if verdict, parseErr := ParseVerdict(investigation.text); parseErr == nil {
		return verdict, usage, nil
	}
	if investigation.eventErr != nil && !isBoundaryError(investigation.eventErr) && !guard.finalizationRequested() {
		return Verdict{}, usage, fmt.Errorf("evaluator event error: %w", investigation.eventErr)
	}

	finalPrompt := `You are an adversarial code reviewer in verdict finalization. Investigative tools are disabled.
Use only the supplied diff, deterministic evidence, and bounded visible investigation notes.
Return exactly one JSON object now with outcome pass or reject. Do not request tools.`
	finalMessage := message
	if notes := strings.TrimSpace(investigation.text); notes != "" {
		if len(notes) > 8192 {
			notes = notes[:8192] + "\n[INVESTIGATION NOTES TRUNCATED]"
		}
		finalMessage += "\n\nVisible investigation notes:\n" + notes
	}
	finalization, err := runPhase(ctx, provider, model, workDir, "evaluate-finalize-"+sessionSuffix(taskSummary), finalPrompt, finalMessage, tool.NewRegistry(), 1, nil, observe)
	addUsage(&usage, finalization.usage)
	if err != nil {
		return Verdict{}, usage, err
	}
	if finalization.eventErr != nil {
		if isBoundaryError(finalization.eventErr) {
			return Verdict{}, usage, &IncompleteError{Reason: "reserved verdict request ended without a verdict"}
		}
		return Verdict{}, usage, fmt.Errorf("evaluator finalization error: %w", finalization.eventErr)
	}
	verdict, parseErr := ParseVerdict(finalization.text)
	if parseErr != nil {
		return Verdict{}, usage, &IncompleteError{Reason: "malformed or missing final verdict"}
	}
	return verdict, usage, nil
}

type phaseResult struct {
	text     string
	usage    llm.Usage
	eventErr error
}

func runPhase(ctx context.Context, provider llm.LLMProvider, model, workDir, sessionID, systemPrompt, message string, tools tool.Executor, maxTurns int, guard *toolGuard, observe EventObserver) (phaseResult, error) {
	rt, err := runtime.BuildRuntime(runtime.RuntimeDeps{}, runtime.RuntimeInputs{
		Provider: provider, Tools: tools, Session: session.NewSession(sessionID, "main"),
	}, runtime.AgentSpec{ID: sessionID, Name: "Evaluator", Model: model, Workspace: workDir, SystemPrompt: systemPrompt, MaxTurns: maxTurns})
	if err != nil {
		return phaseResult{}, fmt.Errorf("building evaluator runtime: %w", err)
	}
	defer rt.Close()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if guard != nil {
		guard.setCancel(cancel)
	}
	events, err := rt.Run(runCtx, message, nil)
	if err != nil {
		return phaseResult{}, fmt.Errorf("running evaluator: %w", err)
	}
	var result phaseResult
	var text strings.Builder
	var observerErr error
	requestUsageSeen := false
	for event := range events {
		// An investigation-boundary error is an internal phase transition, not
		// the terminal outcome of the complete evaluator run.
		boundary := event.Type == runtime.EventError && isBoundaryError(event.Error)
		if observe != nil && observerErr == nil && !boundary {
			observerErr = observe(ctx, event)
			if observerErr != nil {
				cancel()
			}
		}
		if event.Type == runtime.EventTextDelta {
			text.WriteString(event.Text)
		}
		if event.Type == runtime.EventRequestUsage && event.RequestUsage != nil {
			requestUsageSeen = true
			if event.RequestUsage.Usage != nil {
				addUsage(&result.usage, *event.RequestUsage.Usage)
			}
		}
		if event.Type == runtime.EventDone && event.Usage != nil && !requestUsageSeen {
			addUsage(&result.usage, *event.Usage)
		}
		if event.Type == runtime.EventError && event.Error != nil && result.eventErr == nil {
			result.eventErr = event.Error
		}
	}
	if observerErr != nil {
		return result, fmt.Errorf("persisting evaluator trace: %w", observerErr)
	}
	result.text = text.String()
	return result, nil
}

func addUsage(dst *llm.Usage, src llm.Usage) {
	dst.InputTokens += src.InputTokens
	dst.OutputTokens += src.OutputTokens
	dst.CacheCreationInputTokens += src.CacheCreationInputTokens
	dst.CacheReadInputTokens += src.CacheReadInputTokens
}

func isBoundaryError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "maximum turns") || errors.Is(err, context.Canceled)
}

type toolGuard struct {
	mu                   sync.Mutex
	seen                 map[string]int
	verificationCommands map[string]string
	evidence             map[string]string
	finalize             bool
	cancel               context.CancelFunc
}

func newToolGuard(commands []config.VerificationCommand, evidence []verification.Result) *toolGuard {
	guard := &toolGuard{seen: map[string]int{}, verificationCommands: map[string]string{}, evidence: map[string]string{}}
	for _, command := range commands {
		normalized := normalizeCommand(command.Run)
		guard.verificationCommands[normalized] = command.Name
	}
	for _, result := range evidence {
		guard.evidence[result.Name] = fmt.Sprintf("existing verification %q: exit=%d duration=%s truncated=%t\n%s", result.Name, result.ExitCode, result.Duration, result.Truncated, result.Output)
	}
	return guard
}

func (g *toolGuard) check(name string, input json.RawMessage) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	hash := fmt.Sprintf("%x", sha256.Sum256(input))
	key := name + ":" + hash
	g.seen[key]++
	if g.seen[key] >= 2 {
		if g.seen[key] >= 3 {
			g.finalize = true
			if g.cancel != nil {
				g.cancel()
			}
		}
		return "Equivalent tool call already completed. Use the existing evidence and return a verdict, or identify one materially different risk.", false
	}
	if name == "bash" {
		var args struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &args) == nil {
			if commandName, ok := g.verificationCommands[normalizeCommand(args.Command)]; ok {
				if existing, found := g.evidence[commandName]; found {
					return existing + "\nEquivalent command was not executed again. Return a verdict or identify a different material risk.", false
				}
			}
		}
	}
	return "", true
}

func (g *toolGuard) setCancel(cancel context.CancelFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cancel = cancel
}

func (g *toolGuard) finalizationRequested() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.finalize
}

func normalizeCommand(command string) string { return strings.Join(strings.Fields(command), " ") }

type guardedTool struct {
	tool.Tool
	guard *toolGuard
}

func (t *guardedTool) Execute(ctx context.Context, input json.RawMessage) (tool.ToolResult, error) {
	if message, allowed := t.guard.check(t.Name(), input); !allowed {
		return tool.ToolResult{Output: message, Metadata: map[string]any{"sidecar_reused_evidence": true}}, nil
	}
	return t.Tool.Execute(ctx, input)
}

// sessionSuffix derives a short stable-ish suffix for the session id.
func sessionSuffix(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 16 {
		s = s[:16]
	}
	return strings.ReplaceAll(s, " ", "-")
}
