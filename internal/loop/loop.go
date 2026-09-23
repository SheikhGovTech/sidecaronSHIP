package loop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/providers/anthropic"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
	harnessmem "github.com/sausheong/harness/tool/memory"
	"github.com/sausheong/harness/tools/bash"
	"github.com/sausheong/harness/tools/file"
	"github.com/sausheong/sidecar/internal/adapter"
	"github.com/sausheong/sidecar/internal/agenttrace"
	"github.com/sausheong/sidecar/internal/completion"
	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/evaluate"
	"github.com/sausheong/sidecar/internal/memory"
	"github.com/sausheong/sidecar/internal/notify"
	"github.com/sausheong/sidecar/internal/output"
	"github.com/sausheong/sidecar/internal/store"
	"github.com/sausheong/sidecar/internal/triage"
	"github.com/sausheong/sidecar/internal/verification"
	"github.com/sausheong/sidecar/internal/worktree"
)

// Task status constants.
const (
	StatusPending     = "pending"
	StatusRunning     = "running"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
	StatusSkipped     = "skipped"      // triage decided no action needed
	StatusSuggested   = "suggested"    // suggest-only output, no code committed
	StatusNotified    = "notified"     // notify autonomy level — notifications sent, no agent run
	StatusNeedsReview = "needs_review" // draft change request requires human review
)

// Models holds the resolved model names for each agent role.
type Models struct {
	Coding    string
	Triage    string
	Evaluator string
}

// ResolveModels returns the effective model names from cfg, falling back to
// built-in defaults when the config fields are empty.
func ResolveModels(cfg *config.Config) Models {
	m := Models{
		Coding: "anthropic/claude-sonnet-4-6",
		Triage: "anthropic/claude-haiku-4-5-20251001",
	}
	if cfg.Models.Coding != "" {
		m.Coding = cfg.Models.Coding
	}
	if cfg.Models.Triage != "" {
		m.Triage = cfg.Models.Triage
	}
	m.Evaluator = m.Coding
	if cfg.Models.Evaluator != "" {
		m.Evaluator = cfg.Models.Evaluator
	}
	return m
}

// ShipsCode reports whether an autonomy level results in committed code and
// therefore must pass the evaluator gate.
func ShipsCode(autonomyLevel string) bool {
	return autonomyLevel == "auto-commit" || autonomyLevel == "pull-request"
}

// GateAllowsCommit reports whether the evaluator verdict permits committing.
// Fails closed: any evaluator error blocks the commit regardless of verdict.
func GateAllowsCommit(verdict evaluate.Verdict, evalErr error) bool {
	return evalErr == nil && verdict.Pass
}

// UsageTotals accumulates token usage across an agent run's events.
type UsageTotals struct {
	Input  int
	Output int
}

// SignalKey returns the persistent idempotency key for signals that represent
// a naturally unique event. Signals without a stable identity are not deduped.
func SignalKey(sig adapter.Signal) string {
	switch sig.Type {
	case adapter.SignalCIFailure:
		if id := signalValue(sig.Payload, "pipeline_id"); id != "" {
			return "ci.failure:" + id
		}
		if id := signalValue(sig.Payload, "run_id"); id != "" {
			return "ci.failure:" + id
		}
	case adapter.SignalGitCommit:
		if hash := signalValue(sig.Payload, "hash"); hash != "" {
			return "git.commit:" + hash
		}
	}
	return ""
}

func signalValue(payload map[string]any, field string) string {
	if payload == nil || payload[field] == nil {
		return ""
	}
	switch v := payload[field].(type) {
	case string:
		return v
	case int:
		return fmt.Sprint(v)
	case int64:
		return fmt.Sprint(v)
	case int32:
		return fmt.Sprint(v)
	case uint:
		return fmt.Sprint(v)
	case uint64:
		return fmt.Sprint(v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprint(v)
	}
}

// Total returns the combined input+output tokens.
func (u UsageTotals) Total() int { return u.Input + u.Output }

// AccumulateUsage adds an event's reported usage to dst. Only EventDone
// events carrying a non-nil Usage contribute; others are ignored.
func AccumulateUsage(dst *UsageTotals, ev runtime.AgentEvent) {
	if ev.Type == runtime.EventDone && ev.Usage != nil {
		dst.Input += ev.Usage.InputTokens
		dst.Output += ev.Usage.OutputTokens
	}
}

// errString returns the error message, or "" if err is nil. Used for JSON event payloads.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Loop is the core improvement loop that wraps a Harness runtime invocation
// for a single incoming Signal.
type Loop struct {
	db          *store.DB
	workspace   *store.Workspace
	cfg         *config.Config
	repoPath    string
	provider    llm.LLMProvider
	embedding   memory.EmbeddingProvider // nil when memory is not configured
	memTool     *harnessmem.MemoryTool   // nil when embedding is nil
	dispatcher  *notify.Dispatcher       // nil when no notifications configured
	skills      runtime.SkillProvider    // nil when no skills dir present
	retentionMu sync.Mutex
	retainedAt  time.Time
}

// New constructs a Loop. Pass nil for embedding to disable memory retrieval and reviewer-driven memory writes.
func New(db *store.DB, workspace *store.Workspace, cfg *config.Config, repoPath string, embedding memory.EmbeddingProvider) *Loop {
	var memTool *harnessmem.MemoryTool
	if embedding != nil {
		adapter := memory.NewHarnessStoreAdapter(db, embedding, workspace.ID)
		memTool = &harnessmem.MemoryTool{Store: adapter}
	}
	skills := buildSkillsProvider(repoPath, cfg)
	return &Loop{
		db:         db,
		workspace:  workspace,
		cfg:        cfg,
		repoPath:   repoPath,
		provider:   newLLMProvider(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ANTHROPIC_BASE_URL")),
		embedding:  embedding,
		memTool:    memTool,
		dispatcher: notify.NewDispatcher(cfg.Notifications),
		skills:     skills,
	}
}

// newLLMProvider centralizes construction of the Anthropic-compatible client.
// Model identifiers are deliberately not interpreted here: compatible
// gateways receive the exact model string selected by the caller.
func newLLMProvider(apiKey, baseURL string) llm.LLMProvider {
	return anthropic.NewAnthropicProvider(apiKey, baseURL)
}

// Run executes the improvement loop for the given signal:
//  1. Creates a Task record in the database.
//  2. Runs triage to decide whether and how to act.
//  3. Builds a Harness runtime with tools gated by autonomy level.
//  4. Routes output to suggestion, PR, or auto-commit based on triage result.
//  5. Updates task status in the database.
func (l *Loop) Run(ctx context.Context, sig adapter.Signal) error {
	key := SignalKey(sig)
	if key != "" {
		exists, err := l.db.TaskExistsBySignalKey(ctx, l.workspace.ID, key)
		if err != nil {
			slog.Warn("signal dedup check failed; allowing run (fail open)", "err", err, "signal_key", key)
		} else if exists {
			slog.Info("sidecar skipping duplicate signal", "signal_key", key)
			return nil
		}
	}
	task := &store.Task{
		WorkspaceID: l.workspace.ID,
		SignalType:  string(sig.Type),
		Summary:     summarize(sig),
	}
	if key != "" {
		task.SignalKey = &key
	}
	if err := l.db.CreateTask(ctx, task); err != nil {
		return fmt.Errorf("creating task: %w", err)
	}
	traceCfg := l.cfg.EffectiveAgentTraces()
	l.runTraceRetention(ctx, task.ID, traceCfg)

	// ── Budget gate ──────────────────────────────────────────────────────────
	// Checked before any LLM spend. Fails OPEN: a metering error allows the run
	// (the cap is a cost guard, not a safety gate — unlike the evaluator).
	if budget := l.cfg.DailyTokenBudget(); budget > 0 {
		startOfDay := time.Now().UTC().Truncate(24 * time.Hour)
		spent, bErr := l.db.SumWorkspaceTokensSince(ctx, l.workspace.ID, startOfDay)
		if bErr != nil {
			slog.Warn("budget check failed; allowing run (fail open)", "err", bErr, "task", task.ID)
		} else if spent >= budget {
			slog.Info("sidecar: daily token budget reached; skipping", "spent", spent, "budget", budget, "task", task.ID)
			_ = l.db.AppendTaskEvent(ctx, task.ID, "budget_exceeded", map[string]any{"spent": spent, "budget": budget})
			_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusSkipped)
			l.dispatcher.Fire(ctx, notify.EventBudgetExceeded, sig, task)
			return nil
		}
	}

	// ── Triage ──────────────────────────────────────────────────────────────
	models := ResolveModels(l.cfg)
	tr, triageUsage, err := triage.Triage(ctx, l.provider, models.Triage, sig, l.cfg)
	if err != nil {
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		return fmt.Errorf("triage: %w", err)
	}
	_ = l.db.AppendTaskEvent(ctx, task.ID, "triage", map[string]any{
		"should_act":     tr.ShouldAct,
		"change_type":    tr.ChangeType,
		"autonomy_level": tr.AutonomyLevel,
		"reason":         tr.Reason,
	})
	if triageUsage.InputTokens+triageUsage.OutputTokens > 0 {
		_ = l.db.AppendTaskEvent(ctx, task.ID, "usage", map[string]any{
			"input":  triageUsage.InputTokens,
			"output": triageUsage.OutputTokens,
			"total":  triageUsage.InputTokens + triageUsage.OutputTokens,
			"model":  models.Triage,
			"role":   "triage",
		})
	}
	if !tr.ShouldAct {
		slog.Info("sidecar skipping signal", "reason", tr.Reason, "task", task.ID)
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusSkipped)
		l.dispatcher.Fire(ctx, notify.EventSkipped, sig, task)
		return nil
	}
	if !config.ValidAutonomyLevel(tr.AutonomyLevel) {
		_ = l.db.AppendTaskEvent(ctx, task.ID, "invalid_autonomy", map[string]any{"level": tr.AutonomyLevel})
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
		return fmt.Errorf("invalid autonomy level %q", tr.AutonomyLevel)
	}

	// ── Notify-only autonomy ─────────────────────────────────────────────────
	// When autonomy is "notify", skip the coding agent entirely and fire
	// notifications. This is the right choice for infra/network failures where
	// no code change will help — a human needs to be alerted instead.
	if tr.AutonomyLevel == "notify" {
		slog.Info("sidecar notifying (no agent run)", "task", task.ID, "change_type", tr.ChangeType)
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusNotified)
		l.dispatcher.Fire(ctx, notify.EventNotified, sig, task)
		return nil
	}

	// ── Memory retrieval ─────────────────────────────────────────────────────────
	var memoryBlock string
	if l.embedding != nil {
		block, mErr := memory.Retrieve(ctx, l.embedding, l.db, l.workspace, task.Summary, 5)
		if mErr != nil {
			slog.Warn("memory retrieval failed", "err", mErr, "task", task.ID)
		} else {
			memoryBlock = block
		}
	}

	// ── Coding agent ─────────────────────────────────────────────────────────
	if err := l.db.UpdateTaskStatus(ctx, task.ID, StatusRunning); err != nil {
		return err
	}

	// Code-shipping autonomy levels run in an isolated worktree so concurrent
	// signals never share a working tree. suggest-only/notify run in-repo.
	workDir := l.repoPath
	var wtCleanup func() error
	var wt *worktree.Worktree
	if ShipsCode(tr.AutonomyLevel) {
		w, cleanup, wErr := worktree.Create(l.repoPath, task.ID.String())
		if wErr != nil {
			_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
			l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
			return fmt.Errorf("preparing isolated workspace: %w", wErr)
		} else {
			wt, wtCleanup, workDir = w, cleanup, w.Path
			defer func() {
				if wtCleanup != nil {
					if cErr := wtCleanup(); cErr != nil {
						slog.Warn("worktree cleanup failed", "err", cErr, "task", task.ID)
					}
				}
			}()
		}
	}

	workspaceEvent := map[string]any{"kind": "attached"}
	if wt != nil {
		workspaceEvent = map[string]any{"kind": "worktree", "branch": wt.Branch, "base": wt.Base}
	}
	_ = l.db.AppendTaskEvent(ctx, task.ID, "workspace_prepared", workspaceEvent)

	verificationCommands := []config.VerificationCommand(nil)
	if ShipsCode(tr.AutonomyLevel) && l.cfg.VerificationEnabled() {
		verificationCommands = l.cfg.Verification.Commands
		if command, err := verification.Preflight(workDir, verificationCommands); err != nil {
			payload := verificationEvent(verification.Result{Name: command, ExitCode: -1, Err: err})
			payload["phase"] = "preflight"
			_ = l.db.AppendTaskEvent(ctx, task.ID, "verification_failed", payload)
			_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
			l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
			l.discardWorktree(task.ID.String(), wt, &wtCleanup)
			return err
		}
	}

	reg := tool.NewRegistry()
	reg.Register(&file.ReadFileTool{WorkDir: workDir})
	if tr.AutonomyLevel != "suggest-only" {
		reg.Register(&file.WriteFileTool{WorkDir: workDir})
		reg.Register(&file.EditFileTool{WorkDir: workDir})
		reg.Register(&bash.BashTool{WorkDir: workDir})
	}
	if l.memTool != nil {
		reg.Register(l.memTool)
	}

	systemPrompt := BuildSystemPromptWithContext(sig, workDir, verificationCommands)
	if memoryBlock != "" {
		systemPrompt = memoryBlock + "\n\n" + systemPrompt
	}

	taskCopy := *task // capture by value for the OnStop goroutine
	var rt *runtime.Runtime

	spec := runtime.AgentSpec{
		ID:           task.ID.String(),
		Name:         "Sidecar",
		Model:        models.Coding,
		Workspace:    workDir,
		SystemPrompt: systemPrompt,
		MaxTurns:     20,
		Loop: runtime.LoopConfig{
			Hooks: runtime.LifecycleHooks{
				OnStop: func(_ context.Context, reason string) {
					if reason != "completed" || l.memTool == nil {
						return
					}
					// Fire-and-forget reviewer. The goroutine outlives this hook
					// (and the surrounding Loop.Run, which will defer rt.Close()).
					// Safe today: Runtime.Close only releases MCP clients, and sidecar
					// has none. If sidecar later wires MCP servers via AgentSpec.MCPServers,
					// this needs a WaitGroup so Close waits for the reviewer to finish.
					go l.runReview(rt, taskCopy)
				},
			},
		},
	}

	var buildErr error
	rt, buildErr = runtime.BuildRuntime(
		runtime.RuntimeDeps{Skills: l.skills},
		runtime.RuntimeInputs{
			Provider: l.provider,
			Tools:    reg,
			Session:  session.NewSession(task.ID.String(), "main"),
		},
		spec,
	)
	if buildErr != nil {
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
		l.discardWorktree(task.ID.String(), wt, &wtCleanup)
		return fmt.Errorf("building runtime: %w", buildErr)
	}
	defer rt.Close()

	agentCtx, cancelAgent := context.WithCancel(ctx)
	defer cancelAgent()
	events, err := rt.Run(agentCtx, userMessage(sig), nil)
	if err != nil {
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
		l.discardWorktree(task.ID.String(), wt, &wtCleanup)
		return err
	}

	codingTrace := agenttrace.New(l.db, task.ID, "coding", 1, workDir, traceCfg, l.traceSecrets()...)
	var agentErr, traceErr error
	codingTerminal := ""
	var textBuf strings.Builder
	var codingUsage UsageTotals
	for ev := range events {
		if traceErr == nil {
			traceErr = codingTrace.Handle(ctx, ev)
			if traceErr != nil {
				cancelAgent()
			}
		}
		if ev.Type == runtime.EventError {
			agentErr = ev.Error
			codingTerminal = "error"
		} else if ev.Type == runtime.EventDone {
			codingTerminal = "completed"
		} else if ev.Type == runtime.EventAborted {
			codingTerminal = "aborted"
		}
		if ev.Type == runtime.EventTextDelta {
			textBuf.WriteString(ev.Text)
		}
		AccumulateUsage(&codingUsage, ev)
	}
	if flushErr := codingTrace.Flush(ctx); traceErr == nil && flushErr != nil {
		traceErr = flushErr
	}
	codingEvidence := codingTrace.Summary()
	safeCodingSummary := codingEvidence.Text
	if safeCodingSummary == "" {
		safeCodingSummary = boundEvidence(agenttrace.Redact(textBuf.String(), l.traceSecrets()...), 32*1024)
	}
	if codingEvidence.Requests == 0 && codingUsage.Total() > 0 {
		_ = l.db.AppendTaskEvent(ctx, task.ID, "usage", map[string]any{
			"input": codingUsage.Input, "output": codingUsage.Output,
			"total": codingUsage.Total(), "model": models.Coding, "role": "coding",
		})
	}
	if traceErr != nil {
		_ = l.db.AppendTaskEvent(ctx, task.ID, "trace_persistence_failed", map[string]any{"role": "coding", "error": agenttrace.Redact(traceErr.Error(), l.traceSecrets()...)})
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
		l.discardWorktree(task.ID.String(), wt, &wtCleanup)
		return fmt.Errorf("persisting coding-agent trace: %w", traceErr)
	}
	if agentErr != nil && !isTurnExhaustion(agentErr) {
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
		l.discardWorktree(task.ID.String(), wt, &wtCleanup)
		return fmt.Errorf("agent error: %w", agentErr)
	}
	codingBaseRef := ""
	if wt != nil {
		codingBaseRef = wt.Base
	}
	codingChanged := false
	if ShipsCode(tr.AutonomyLevel) {
		var changeErr error
		codingChanged, changeErr = workspaceHasChanges(workDir, codingBaseRef)
		if changeErr != nil {
			return l.failVerification(ctx, sig, task, wt, &wtCleanup, "change_detection", verification.Result{Err: changeErr})
		}
	}
	if codingTerminal != "completed" || (codingChanged && strings.TrimSpace(safeCodingSummary) == "") || (!codingChanged && strings.TrimSpace(safeCodingSummary) == "") {
		stopReason := codingTerminal
		if agentErr != nil {
			stopReason = agenttrace.Redact(agentErr.Error(), l.traceSecrets()...)
		}
		return l.handleCodingIncomplete(ctx, sig, task, wt, &wtCleanup, safeCodingSummary, stopReason, codingEvidence)
	}
	codingOutcome := completion.CodingNoChange
	if codingChanged {
		codingOutcome = completion.CodingChanged
	}
	_ = l.db.AppendTaskEvent(ctx, task.ID, "coding_completed", (completion.StageResult{Outcome: codingOutcome,
		StopReason: codingTerminal, Attempt: 1, RequestsUsed: codingEvidence.Requests,
		TraceID: codingEvidence.TraceID, Summary: safeCodingSummary}).Payload())

	var evaluatorEvidence agenttrace.Summary
	var verificationEvidence []verification.Result
	evaluationStatus := "not_run"
	evaluationReasons := ""
	draftChangeRequest := false

	// ── Deterministic verification and adversarial evaluation gates ─────────
	if ShipsCode(tr.AutonomyLevel) && l.cfg.VerificationEnabled() {
		// Anchor the diff to the worktree base ref so the evaluator sees the
		// agent's changes even if it self-committed (HEAD would have moved).
		// In the in-repo fallback (wt == nil) baseRef is "" → Evaluate uses HEAD.
		baseRef := codingBaseRef
		changed, changeErr := workspaceHasChanges(workDir, baseRef)
		if changeErr != nil {
			return l.failVerification(ctx, sig, task, wt, &wtCleanup, "change_detection", verification.Result{Err: changeErr})
		}
		if !changed {
			slog.Info("sidecar: no changes; skipping verification and evaluator", "task", task.ID)
		} else {
			for _, command := range verificationCommands {
				_ = l.db.AppendTaskEvent(ctx, task.ID, "verification_started", map[string]any{"command": command.Name})
				result := verification.RunOne(ctx, workDir, command)
				if result.Err != nil || result.ExitCode != 0 {
					return l.failVerification(ctx, sig, task, wt, &wtCleanup, command.Name, result)
				}
				verificationEvidence = append(verificationEvidence, result)
				_ = l.db.AppendTaskEvent(ctx, task.ID, "verification_succeeded", verificationEvent(result))
			}

			evaluatorTrace := agenttrace.New(l.db, task.ID, "evaluator", 1, workDir, traceCfg, l.traceSecrets()...)
			evaluatorInput := sanitizeVerificationEvidence(verificationEvidence, l.traceSecrets()...)
			verdict, evalUsage, evalErr := evaluate.EvaluateObservedWithEvidence(ctx, l.provider, models.Evaluator, workDir, baseRef,
				task.Summary, verificationCommands, evaluatorInput, l.cfg.EvaluatorMaxTurns(), evaluatorTrace.Handle)
			if flushErr := evaluatorTrace.Flush(ctx); flushErr != nil && evalErr == nil {
				evalErr = fmt.Errorf("persisting evaluator trace: %w", flushErr)
			}
			evaluatorEvidence = evaluatorTrace.Summary()
			safeEvalErr := ""
			if evalErr != nil {
				safeEvalErr = agenttrace.Redact(evalErr.Error(), l.traceSecrets()...)
			}
			// Record evaluator token spend even on error — the tokens were consumed.
			if evaluatorEvidence.Requests == 0 && evalUsage.InputTokens+evalUsage.OutputTokens > 0 {
				_ = l.db.AppendTaskEvent(ctx, task.ID, "usage", map[string]any{
					"input":  evalUsage.InputTokens,
					"output": evalUsage.OutputTokens,
					"total":  evalUsage.InputTokens + evalUsage.OutputTokens,
					"model":  models.Evaluator,
					"role":   "evaluator",
				})
			}
			evaluationPayload := map[string]any{
				"outcome": verdict.Outcome, "pass": verdict.Pass,
				"reasons": verdict.Reasons, "evidence": verdict.Evidence,
				"model": models.Evaluator, "error": safeEvalErr,
				"trace_id": evaluatorEvidence.TraceID.String(), "attempt": 1,
				"requests_used": evaluatorEvidence.Requests, "repeated_tools": evaluatorEvidence.RepeatedTools,
			}
			_ = l.db.AppendTaskEvent(ctx, task.ID, "evaluation", evaluationPayload)
			if evalErr != nil {
				incomplete := evaluate.IsIncomplete(evalErr)
				evaluationStatus, evaluationReasons = "error", safeEvalErr
				eventType := "evaluation_error"
				if incomplete {
					evaluationStatus = "incomplete"
					eventType = "evaluation_incomplete"
				}
				evaluationPayload["outcome"] = evaluationStatus
				evaluationPayload["stop_reason"] = safeEvalErr
				evaluationPayload["attempt"] = 1
				evaluationPayload["requests_used"] = evaluatorEvidence.Requests
				_ = l.db.AppendTaskEvent(ctx, task.ID, eventType, evaluationPayload)
				if incomplete {
					slog.Warn("evaluator incomplete", "err", evalErr, "task", task.ID)
					_ = l.db.AppendTaskEvent(ctx, task.ID, "incomplete_handoff",
						incompleteHandoff(task.ID, completion.EvaluationIncomplete, safeCodingSummary, safeEvalErr, evaluatorEvidence.TraceID, verificationEvidence))
				} else {
					slog.Warn("evaluator runtime error", "err", evalErr, "task", task.ID)
				}
				if l.cfg.EvaluatorOnError() == "draft-change-request" && tr.AutonomyLevel == "pull-request" {
					draftChangeRequest = true
				} else if l.cfg.EvaluatorOnError() == "fail" {
					_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
					l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
					l.discardWorktree(task.ID.String(), wt, &wtCleanup)
					return fmt.Errorf("evaluator: %w", evalErr)
				} else {
					handoff := incompleteHandoff(task.ID, completion.EvaluationIncomplete, safeCodingSummary, safeEvalErr, evaluatorEvidence.TraceID, verificationEvidence)
					if !incomplete {
						handoff["reason"] = completion.EvaluationError
					}
					_ = l.db.AppendTaskEvent(ctx, task.ID, "suggestion", handoff)
					_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusSuggested)
					notifyTask := *task
					notifyTask.Summary = handoffNotification(handoff)
					l.dispatcher.Fire(ctx, notify.EventSuggested, sig, &notifyTask)
					l.discardWorktree(task.ID.String(), wt, &wtCleanup)
					return nil
				}
			} else if !verdict.Pass {
				evaluationStatus, evaluationReasons = "reject", verdict.Reasons
				_ = l.db.AppendTaskEvent(ctx, task.ID, "evaluation_rejected", evaluationPayload)
				slog.Info("evaluator rejected change; recording as suggestion", "task", task.ID, "reasons", verdict.Reasons)
				_ = l.db.AppendTaskEvent(ctx, task.ID, "suggestion", map[string]any{
					"summary":          safeCodingSummary,
					"rejected_reasons": verdict.Reasons,
				})
				_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusSuggested)
				l.dispatcher.Fire(ctx, notify.EventSuggested, sig, task)
				l.discardWorktree(task.ID.String(), wt, &wtCleanup)
				return nil
			} else {
				evaluationStatus, evaluationReasons = "pass", verdict.Reasons
				_ = l.db.AppendTaskEvent(ctx, task.ID, "evaluation_passed", evaluationPayload)
			}
		}
	}

	// commit creates the output branch. In a worktree it commits in place and
	// returns the worktree's branch; in-repo it falls back to CommitBranch.
	commit := func() (string, error) {
		out := output.New(workDir)
		if wt != nil {
			// Detect changes relative to the worktree base ref so an agent
			// self-commit still routes to PR/completed-with-branch rather than
			// being misread as "no changes".
			changed, err := out.CommitInPlaceFrom(wt.Base, "sidecar: "+task.Summary)
			if err != nil {
				return "", err
			}
			if !changed {
				return output.BranchNoChanges, nil
			}
			return wt.Branch, nil
		}
		return out.CommitBranch(task.ID.String(), "sidecar: "+task.Summary)
	}

	// ── Output routing ───────────────────────────────────────────────────────
	switch tr.AutonomyLevel {
	case "suggest-only":
		summary := safeCodingSummary
		_ = l.db.AppendTaskEvent(ctx, task.ID, "suggestion", map[string]any{"summary": summary})
		slog.Info("sidecar suggestion recorded", "task", task.ID, "change_type", tr.ChangeType)
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusSuggested)
		l.dispatcher.Fire(ctx, notify.EventSuggested, sig, task)
		return nil

	case "pull-request":
		branch, err := commit()
		if err != nil {
			_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
			l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
			return err
		}
		if branch == output.BranchNoChanges {
			slog.Info("sidecar: no changes to commit", "task", task.ID)
			_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusCompleted)
			l.dispatcher.Fire(ctx, notify.EventCompleted, sig, task)
			l.discardWorktree(task.ID.String(), wt, &wtCleanup)
			return nil
		}
		fallbackRepo, fallbackToken := l.resolveRepoAndToken(sig)
		target, resolveErr := output.ResolveDelivery(l.cfg.Delivery, l.repoPath, sig.Source, fallbackRepo, fallbackToken)
		if resolveErr != nil {
			return l.failDelivery(ctx, sig, task, branch, output.DeliveryTarget{Provider: l.cfg.Delivery.Provider}, output.PublishResult{}, resolveErr)
		}
		labels := []string(nil)
		if draftChangeRequest {
			labels = []string{"sidecar:evaluation-incomplete"}
		}
		result, publishErr := output.NewPublisher(target).Publish(ctx, output.PublishRequest{
			RepoPath: l.repoPath,
			Branch:   branch,
			Title:    "sidecar: " + task.Summary,
			Body:     l.prBody(sig, tr, task.ID.String(), codingEvidence, evaluatorEvidence, verificationEvidence, evaluationStatus, evaluationReasons),
			Draft:    draftChangeRequest,
			Labels:   labels,
		})
		if result.Pushed {
			_ = l.db.AppendTaskEvent(ctx, task.ID, "branch_pushed", map[string]any{
				"provider": target.Provider, "remote": target.Remote, "repo": target.Repo, "branch": branch,
			})
		}
		if publishErr != nil {
			return l.failDelivery(ctx, sig, task, branch, target, result, publishErr)
		}
		eventType := "change_request_created"
		if result.Reused {
			eventType = "change_request_reused"
		}
		_ = l.db.AppendTaskEvent(ctx, task.ID, eventType, map[string]any{
			"provider": target.Provider, "url": result.URL, "branch": branch, "base_branch": target.BaseBranch,
			"draft": draftChangeRequest, "evaluation_status": evaluationStatus,
		})
		slog.Info("sidecar change request ready", "provider", target.Provider, "url", result.URL, "task", task.ID)
		status := StatusCompleted
		if draftChangeRequest {
			status = StatusNeedsReview
		}
		_ = l.db.UpdateTaskStatus(ctx, task.ID, status)
		l.dispatcher.Fire(ctx, notify.EventCompleted, sig, task)
		return nil

	case "auto-commit":
		branch, err := commit()
		if err != nil {
			_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
			l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
			return err
		}
		if branch != output.BranchNoChanges {
			slog.Info("sidecar committed changes", "branch", branch, "task", task.ID)
		} else {
			l.discardWorktree(task.ID.String(), wt, &wtCleanup)
		}
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusCompleted)
		l.dispatcher.Fire(ctx, notify.EventCompleted, sig, task)
		return nil

	default:
		_ = l.db.AppendTaskEvent(ctx, task.ID, "invalid_autonomy", map[string]any{"level": tr.AutonomyLevel})
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
		l.discardWorktree(task.ID.String(), wt, &wtCleanup)
		return fmt.Errorf("invalid autonomy level %q", tr.AutonomyLevel)
	}
}

func (l *Loop) failDelivery(ctx context.Context, sig adapter.Signal, task *store.Task, branch string, target output.DeliveryTarget, result output.PublishResult, deliveryErr error) error {
	phase := "resolve"
	var typed *output.DeliveryError
	if errors.As(deliveryErr, &typed) {
		phase = typed.Phase
	}
	errText := deliveryErr.Error()
	if target.Token != "" {
		errText = strings.ReplaceAll(errText, target.Token, "[REDACTED]")
	}
	_ = l.db.AppendTaskEvent(ctx, task.ID, "delivery_failed", map[string]any{
		"provider": target.Provider, "phase": phase, "branch": branch, "branch_pushed": result.Pushed, "error": errText,
	})
	_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
	l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
	return deliveryErr
}

func workspaceHasChanges(workDir, baseRef string) (bool, error) {
	if baseRef == "" {
		baseRef = "HEAD"
	}
	cmd := exec.Command("git", "-C", workDir, "diff", "--quiet", baseRef, "--")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("checking workspace diff: %w", err)
	}
	status, err := exec.Command("git", "-C", workDir, "status", "--porcelain").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checking workspace status: %w", err)
	}
	return strings.TrimSpace(string(status)) != "", nil
}

func verificationEvent(result verification.Result) map[string]any {
	return map[string]any{
		"command":     result.Name,
		"exit_code":   result.ExitCode,
		"output":      result.Output,
		"duration_ms": result.Duration.Milliseconds(),
		"truncated":   result.Truncated,
		"error":       errString(result.Err),
	}
}

func (l *Loop) failVerification(ctx context.Context, sig adapter.Signal, task *store.Task, wt *worktree.Worktree, cleanup *func() error, command string, result verification.Result) error {
	payload := verificationEvent(result)
	payload["command"] = command
	_ = l.db.AppendTaskEvent(ctx, task.ID, "verification_failed", payload)
	_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
	l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
	l.discardWorktree(task.ID.String(), wt, cleanup)
	if result.Err != nil {
		return fmt.Errorf("verification %q failed: %w", command, result.Err)
	}
	return fmt.Errorf("verification %q failed with exit code %d", command, result.ExitCode)
}

func isTurnExhaustion(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "maximum turns")
}

func (l *Loop) handleCodingIncomplete(ctx context.Context, sig adapter.Signal, task *store.Task, wt *worktree.Worktree, cleanup *func() error, summary, stopReason string, trace agenttrace.Summary) error {
	payload := incompleteHandoff(task.ID, completion.CodingIncomplete, summary, stopReason, trace.TraceID, nil)
	payload["attempt"] = 1
	payload["requests_used"] = trace.Requests
	_ = l.db.AppendTaskEvent(ctx, task.ID, "coding_incomplete", payload)
	l.discardWorktree(task.ID.String(), wt, cleanup)
	if strings.TrimSpace(summary) == "" {
		_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		l.dispatcher.Fire(ctx, notify.EventFailed, sig, task)
		return fmt.Errorf("coding agent incomplete without a usable handoff: %s", stopReason)
	}
	_ = l.db.AppendTaskEvent(ctx, task.ID, "suggestion", payload)
	_ = l.db.UpdateTaskStatus(ctx, task.ID, StatusSuggested)
	notifyTask := *task
	notifyTask.Summary = handoffNotification(payload)
	l.dispatcher.Fire(ctx, notify.EventSuggested, sig, &notifyTask)
	return nil
}

func incompleteHandoff(taskID uuid.UUID, reason completion.Outcome, summary, stopReason string, traceID uuid.UUID, checks []verification.Result) map[string]any {
	completed := []string{}
	for _, check := range checks {
		if check.Err == nil && check.ExitCode == 0 {
			completed = append(completed, fmt.Sprintf("deterministic verification passed: %s", check.Name))
		}
	}
	if strings.TrimSpace(summary) != "" {
		completed = append(completed, "bounded agent summary captured")
	}
	return (completion.Handoff{Reason: reason, Summary: summary, StopReason: stopReason,
		Completed: completed, Remaining: []string{"human review of incomplete stage"},
		RecommendedAction: "Review the bounded evidence and continue the task manually.",
		TaskID:            taskID, TraceID: traceID}).Payload()
}

func handoffNotification(handoff map[string]any) string {
	return boundEvidence(fmt.Sprintf("%s: %s Recommended action: %s Task: %s Trace: %s",
		handoff["reason"], handoff["summary"], handoff["recommended_action"], handoff["task_id"], handoff["trace_id"]), 4096)
}

func sanitizeVerificationEvidence(results []verification.Result, secrets ...string) []verification.Result {
	clean := make([]verification.Result, len(results))
	copy(clean, results)
	for i := range clean {
		clean[i].Output = boundEvidence(agenttrace.Redact(clean[i].Output, secrets...), 4096)
		if clean[i].Err != nil {
			clean[i].Err = errors.New(agenttrace.Redact(clean[i].Err.Error(), secrets...))
		}
	}
	return clean
}

func (l *Loop) discardWorktree(taskID string, wt *worktree.Worktree, cleanup *func() error) {
	if wt == nil {
		return
	}
	if cleanup != nil && *cleanup != nil {
		if err := (*cleanup)(); err != nil {
			slog.Warn("failed to discard task worktree", "err", err, "task", taskID)
			return
		}
		*cleanup = nil
	}
	if err := worktree.DeleteBranch(l.repoPath, wt.Branch); err != nil {
		slog.Warn("failed to delete rejected task branch", "err", err, "task", taskID, "branch", wt.Branch)
	}
}

// runReview snapshots the parent runtime and extracts memory via a
// haiku reviewer with the memory tool only. Detaches from the parent
// ctx (which is canceled by the time OnStop fires) and applies its own
// 90s timeout.
func (l *Loop) runReview(parent *runtime.Runtime, task store.Task) {
	// 70s outer = 60s ReviewSpec.Timeout (capped by Review itself) +
	// ~10s headroom for GetTaskEvents and reviewer-runtime construction.
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()

	events, err := l.db.GetTaskEvents(ctx, task.ID)
	if err != nil {
		slog.Warn("review: failed to load task events", "err", err, "task", task.ID)
		return
	}

	reviewerReg := tool.NewRegistry()
	reviewerReg.Register(l.memTool)
	reviewerTrace := agenttrace.New(l.db, task.ID, "reviewer", 1, parent.Workspace, l.cfg.EffectiveAgentTraces(), l.traceSecrets()...)
	var traceErr error

	res := runtime.Review(ctx, parent, runtime.ReviewSpec{
		Prompt:   buildReviewerPrompt(task, events),
		Tools:    reviewerReg,
		Model:    ResolveModels(l.cfg).Triage,
		MaxTurns: 4,
		Timeout:  60 * time.Second,
		OnEvent: func(event runtime.AgentEvent) {
			if traceErr == nil {
				traceErr = reviewerTrace.Handle(ctx, event)
			}
		},
	})
	if flushErr := reviewerTrace.Flush(ctx); traceErr == nil {
		traceErr = flushErr
	}
	if traceErr != nil {
		_ = l.db.AppendTaskEvent(ctx, task.ID, "trace_persistence_failed", map[string]any{"role": "reviewer", "error": agenttrace.Redact(traceErr.Error(), l.traceSecrets()...)})
		slog.Warn("review trace persistence failed", "err", traceErr, "task", task.ID)
	}
	if res.Err != nil {
		slog.Warn("review failed", "err", res.Err, "task", task.ID)
		return
	}
	slog.Info("review completed", "task", task.ID, "actions", len(res.Actions))
}

// runTraceRetention performs at most one bounded cleanup batch per Loop per
// day. It is triggered by task activity so embedding applications do not need
// a separate scheduler.
func (l *Loop) runTraceRetention(ctx context.Context, taskID uuid.UUID, cfg config.AgentTraceConfig) {
	if cfg.RetentionDays <= 0 {
		return
	}
	l.retentionMu.Lock()
	defer l.retentionMu.Unlock()
	if !l.retainedAt.IsZero() && time.Since(l.retainedAt) < 24*time.Hour {
		return
	}
	deleted, err := l.db.DeleteAgentTraceEventsBefore(ctx, l.workspace.ID, time.Now().UTC().AddDate(0, 0, -cfg.RetentionDays), 1000)
	if err != nil {
		slog.Warn("agent trace retention cleanup failed", "err", err)
		_ = l.db.AppendTaskEvent(ctx, taskID, "trace_retention_failed", map[string]any{"error": agenttrace.Redact(err.Error(), l.traceSecrets()...)})
		return
	}
	l.retainedAt = time.Now()
	if deleted > 0 {
		slog.Info("expired agent traces deleted", "workspace", l.workspace.ID, "rows", deleted)
	}
}

// resolveRepoAndToken looks up the repo slug and resolved token for a signal.
func (l *Loop) resolveRepoAndToken(sig adapter.Signal) (repo, token string) {
	if r, ok := sig.Payload["repo"].(string); ok && r != "" {
		repo = r
	}
	for _, sc := range l.cfg.Signals {
		if sc.Repo == repo {
			token = sc.ResolveToken()
			return
		}
	}
	token = os.Getenv("GITHUB_TOKEN")
	return
}

// prBody generates a bounded decision record. It references durable traces
// rather than copying complete prompts, CI logs, or hidden model reasoning.
func (l *Loop) prBody(sig adapter.Signal, tr triage.TriageResult, taskID string, coding, evaluator agenttrace.Summary, checks []verification.Result, evaluationStatus, evaluationReasons string) string {
	var body strings.Builder
	fmt.Fprintf(&body, "## Sidecar automated fix\n\n**Signal:** %s — %s\n**Change type:** %s\n**Task ID:** `%s`\n\n", string(sig.Type), summarize(sig), tr.ChangeType, taskID)
	fmt.Fprintf(&body, "## Decision log\n\n- Triage: `%s` (%s)\n- Autonomy: `%s`\n- Evaluator: `%s`", tr.ChangeType, tr.Reason, tr.AutonomyLevel, evaluationStatus)
	if evaluationReasons != "" {
		fmt.Fprintf(&body, " — %s", evaluationReasons)
	}
	body.WriteString("\n\n## Repair agent record\n\n")
	if coding.Enabled {
		fmt.Fprintf(&body, "- Trace: `%s`\n", coding.TraceID)
	} else {
		body.WriteString("- Trace: disabled by configuration\n")
	}
	fmt.Fprintf(&body, "- Model requests: %d\n- Tokens: %d input / %d output\n- Tool calls: %d\n", coding.Requests, coding.InputTokens, coding.OutputTokens, coding.ToolCalls)
	if repeated := repeatedToolEvidence(coding.RepeatedTools); len(repeated) > 0 {
		fmt.Fprintf(&body, "- Repeated tool calls: %s\n", strings.Join(repeated, ", "))
	}
	if coding.Text != "" {
		text := coding.Text
		if len(text) > 4096 {
			text = text[:4096] + "\n[TRUNCATED]"
		}
		fmt.Fprintf(&body, "\n<details><summary>Visible final agent message</summary>\n\n%s\n\n</details>\n", text)
	}
	body.WriteString("\n## Verification record\n\n")
	if len(checks) == 0 {
		body.WriteString("- No configured deterministic command was run.\n")
	}
	for _, check := range checks {
		fmt.Fprintf(&body, "- `%s`: exit %d in %s (output truncated: %t)\n", check.Name, check.ExitCode, check.Duration.Round(time.Millisecond), check.Truncated)
	}
	body.WriteString("\n## Evaluator record\n\n")
	if evaluator.Enabled {
		fmt.Fprintf(&body, "- Trace: `%s`\n- Outcome: `%s`\n- Model requests: %d / max turns %d\n- Tokens: %d input / %d output\n- Tool calls: %d\n", evaluator.TraceID, evaluationStatus, evaluator.Requests, l.cfg.EvaluatorMaxTurns(), evaluator.InputTokens, evaluator.OutputTokens, evaluator.ToolCalls)
		if evaluator.PersistenceFail != nil {
			body.WriteString("- Audit evidence: incomplete because durable trace persistence failed\n")
		}
	} else {
		fmt.Fprintf(&body, "- Outcome: `%s`\n- Trace: not available\n", evaluationStatus)
	}
	if evaluationStatus == "error" || evaluationStatus == "incomplete" {
		body.WriteString("\n> **Human review required:** deterministic verification passed, but evaluator execution did not complete. This change request is not evaluator-approved.\n")
	}
	if evaluator.Text != "" {
		text := evaluator.Text
		if len(text) > 4096 {
			text = text[:4096] + "\n[TRUNCATED]"
		}
		fmt.Fprintf(&body, "\n<details><summary>Visible evaluator message</summary>\n\n%s\n\n</details>\n", text)
	}
	body.WriteString("\nThis change request contains sanitized, bounded evidence. When tracing is enabled, full permitted trace records remain in Sidecar's task database.\n")
	return boundEvidence(agenttrace.Redact(body.String(), l.traceSecrets()...), 32*1024)
}

func repeatedToolEvidence(repeats map[string]int) []string {
	var result []string
	for toolHash, count := range repeats {
		if count > 1 {
			name := strings.SplitN(toolHash, ":", 2)[0]
			result = append(result, fmt.Sprintf("`%s` × %d", name, count))
		}
	}
	sort.Strings(result)
	return result
}

func boundEvidence(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	const marker = "\n\n[SIDECAR EVIDENCE TRUNCATED]\n"
	value = value[:limit-len(marker)]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + marker
}

func (l *Loop) traceSecrets() []string {
	secrets := []string{l.cfg.Delivery.ResolveToken(), os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("PAI_TOKEN"), os.Getenv("DATABASE_URL"), os.Getenv("SIDECAR_TEST_DB_URL")}
	for _, signal := range l.cfg.Signals {
		secrets = append(secrets, signal.ResolveToken())
	}
	return secrets
}

// BuildSystemPrompt constructs the signal-specific prompt without runtime
// workspace context. Kept as a compatibility wrapper for callers and tests.
func BuildSystemPrompt(sig adapter.Signal) string {
	return BuildSystemPromptWithContext(sig, "", nil)
}

// BuildSystemPromptWithContext binds the agent to the resolved task workspace
// and tells it which deterministic commands must pass before output can ship.
func BuildSystemPromptWithContext(sig adapter.Signal, workDir string, commands []config.VerificationCommand) string {
	base := `You are an autonomous engineering agent (Sidecar) attached to a software project.
Your job is to improve, fix, and maintain the codebase. You have access to the filesystem and bash.
Make targeted, minimal changes. Run tests after any code change to verify correctness.
Only modify files relevant to the current task.`
	if workDir != "" {
		base += fmt.Sprintf(`

Workspace root: %s
All filesystem and Bash tools already execute from this directory.
Use relative paths. Do not change to or guess another repository path.
Run pwd before investigating.`, workDir)
	}
	if block := verification.PromptBlock(commands); block != "" {
		base += "\n\n" + block
	}

	switch sig.Type {
	case adapter.SignalGitCommit:
		hash, _ := sig.Payload["hash"].(string)
		return fmt.Sprintf(`%s

A new commit (%s) was just pushed. Review it and fix any immediate issues:
broken tests, compilation errors, obvious bugs introduced by the change.
If everything looks good, do nothing.`, base, hash)

	case adapter.SignalCIFailure:
		workflow, _ := sig.Payload["workflow_name"].(string)
		sha, _ := sig.Payload["head_sha"].(string)
		url, _ := sig.Payload["html_url"].(string)
		failedJob, _ := sig.Payload["failed_job"].(string)
		jobLog, _ := sig.Payload["job_log"].(string)
		changedFiles, _ := sig.Payload["changed_files"].(string)
		commitDiff, _ := sig.Payload["commit_diff"].(string)
		msg := fmt.Sprintf(`%s

A CI run failed in %s:
Workflow: %s
Commit: %s
Run URL: %s`, base, sig.Source, workflow, sha, url)
		if failedJob != "" {
			msg += fmt.Sprintf("\nFailed job: %s", failedJob)
		}
		if jobLog != "" {
			msg += fmt.Sprintf("\n\nCI error output:\n%s", jobLog)
		}
		if commitDiff != "" {
			msg += fmt.Sprintf("\n\nCommit diff:\n%s", commitDiff)
		} else if changedFiles != "" {
			msg += fmt.Sprintf("\n\nChanged files: %s", changedFiles)
		}
		msg += "\n\nUse the error output and diff above to identify the root cause. Fix it and run tests locally to verify before committing."
		return msg

	case adapter.SignalScheduleTick:
		return fmt.Sprintf(`%s

This is a proactive maintenance sweep. Look for improvement opportunities:
- Fix or improve any area flagged as fragile or prone to regression in the workspace memory above
- Add tests to code paths noted as missing coverage
- If no specific area is flagged, check for: stale dependencies, dead code, or outdated docs

Pick ONE meaningful improvement and apply it. Run tests to verify your change.`, base)

	case adapter.SignalLogAnomaly:
		pattern, _ := sig.Payload["pattern"].(string)
		source, _ := sig.Payload["source"].(string)
		line, _ := sig.Payload["line"].(string)
		return fmt.Sprintf(`%s

A log anomaly was detected in the running application.

Pattern: %s
Source:  %s
Sample:  %s

Investigate the root cause. Check relevant code paths, reproduce the issue if possible,
and apply a fix. Run tests to verify your change.`, base, pattern, source, line)

	case adapter.SignalMetricAlert:
		name, _ := sig.Payload["alert_name"].(string)
		message, _ := sig.Payload["message"].(string)
		provider, _ := sig.Payload["provider"].(string)
		return fmt.Sprintf(`%s

A metrics alert has fired in the %s monitoring system.

Alert:   %s
Details: %s

Investigate the root cause in the codebase. Check recent changes, identify the code
path responsible, and apply a fix. Run tests to verify your change.`, base, provider, name, message)

	case adapter.SignalUptimeFailure:
		url, _ := sig.Payload["url"].(string)
		ft, _ := sig.Payload["failure_type"].(string)
		elapsedMs, _ := sig.Payload["elapsed_ms"].(int64)
		diagSummary, _ := sig.Payload["diagnostic_summary"].(string)

		diagBlock := ""
		if diagSummary != "" {
			diagBlock = fmt.Sprintf(`

Diagnostic results: %s

IMPORTANT — check these before touching any code:
- If dns:FAIL or tcp:FAIL → the server process or network is the problem, not code. Check if the process is running, check firewall rules, check load balancer config.
- If cross:FAIL (all endpoints down) → infrastructure outage, not a code bug. Do not commit any changes; notify the team.
- If dns:ok, tcp:ok, cross shows isolated failure → this is likely a code issue. Proceed to investigate handlers and middleware.
- If tls:FAIL → check certificate expiry; renew or update the cert configuration.
- Review any shell/http diagnostic output above for additional clues (database, cache, dependencies).`, diagSummary)
		}

		switch ft {
		case "unreachable":
			errMsg, _ := sig.Payload["error"].(string)
			return fmt.Sprintf(`%s

An uptime check detected that %s is unreachable.
Error: %s%s

Start with: bash -c 'curl -sv %s 2>&1 | head -30'
Then check if the server process is running and inspect recent git log for changes that could affect startup or routing.`, base, url, errMsg, diagBlock, url)

		case "wrong_status":
			got, _ := sig.Payload["got_status"].(int)
			want, _ := sig.Payload["expected_status"].(int)
			return fmt.Sprintf(`%s

An uptime check detected an unexpected HTTP status from %s.
Got: %d  Expected: %d%s

Start with: bash -c 'curl -sv %s 2>&1 | head -40'
Then check recent handler, middleware, and routing changes. Run the test suite to identify what is failing.`, base, url, got, want, diagBlock, url)

		case "slow_response":
			thresholdMs, _ := sig.Payload["threshold_ms"].(int)
			return fmt.Sprintf(`%s

A performance check detected that %s is responding slowly.
Response time: %dms  Threshold: %dms%s

Start with: bash -c 'curl -w "%%{time_total}" -o /dev/null -s %s'
Investigate slow database queries, missing indexes, N+1 patterns, or blocking synchronous operations on the request path. Apply a targeted fix and verify the improvement.`, base, url, elapsedMs, thresholdMs, diagBlock, url)
		}
		return fmt.Sprintf(`%s

An uptime check failed for %s.%s

Investigate the root cause starting with the diagnostic results above.`, base, url, diagBlock)

	default:
		desc, _ := sig.Payload["description"].(string)
		return fmt.Sprintf(`%s

On-demand task: %s

Complete this task. Run tests to verify your changes.`, base, desc)
	}
}

// userMessage returns the initial user turn sent to the agent.
func userMessage(sig adapter.Signal) string {
	switch sig.Type {
	case adapter.SignalGitCommit:
		hash, _ := sig.Payload["hash"].(string)
		return fmt.Sprintf("New commit detected: %s. Review and fix any issues.", hash)
	case adapter.SignalCIFailure:
		workflow, _ := sig.Payload["workflow_name"].(string)
		sha, _ := sig.Payload["head_sha"].(string)
		failedJob, _ := sig.Payload["failed_job"].(string)
		msg := fmt.Sprintf("CI failure in workflow %q on commit %s.", workflow, sha)
		if failedJob != "" {
			msg += fmt.Sprintf(" Failed job: %s.", failedJob)
		}
		msg += " Investigate and fix."
		return msg
	case adapter.SignalScheduleTick:
		return "Proactive sweep: identify and apply one meaningful improvement."
	case adapter.SignalLogAnomaly:
		pattern, _ := sig.Payload["pattern"].(string)
		source, _ := sig.Payload["source"].(string)
		return fmt.Sprintf("Log anomaly detected: pattern %q in %s. Investigate and fix.", pattern, source)
	case adapter.SignalMetricAlert:
		name, _ := sig.Payload["alert_name"].(string)
		provider, _ := sig.Payload["provider"].(string)
		return fmt.Sprintf("Metrics alert %q fired in %s. Investigate and fix.", name, provider)
	case adapter.SignalUptimeFailure:
		url, _ := sig.Payload["url"].(string)
		ft, _ := sig.Payload["failure_type"].(string)
		return fmt.Sprintf("Uptime check failed for %s (%s). Investigate and fix.", url, ft)
	default:
		desc, _ := sig.Payload["description"].(string)
		if desc == "" {
			return "Perform a general codebase health check and fix any obvious issues."
		}
		return desc
	}
}

// summarize produces a short human-readable summary of the signal for storage.
func summarize(sig adapter.Signal) string {
	switch sig.Type {
	case adapter.SignalGitCommit:
		hash, _ := sig.Payload["hash"].(string)
		if len(hash) > 8 {
			hash = hash[:8]
		}
		return "review commit " + hash
	case adapter.SignalCIFailure:
		workflow, _ := sig.Payload["workflow_name"].(string)
		sha, _ := sig.Payload["head_sha"].(string)
		if len(sha) > 8 {
			sha = sha[:8]
		}
		return fmt.Sprintf("fix CI failure in %s @ %s", workflow, sha)
	case adapter.SignalScheduleTick:
		return "proactive sweep"
	case adapter.SignalLogAnomaly:
		pattern, _ := sig.Payload["pattern"].(string)
		source, _ := sig.Payload["source"].(string)
		return fmt.Sprintf("fix log anomaly: %s in %s", pattern, source)
	case adapter.SignalMetricAlert:
		name, _ := sig.Payload["alert_name"].(string)
		return fmt.Sprintf("fix metric alert: %s", name)
	case adapter.SignalUptimeFailure:
		url, _ := sig.Payload["url"].(string)
		ft, _ := sig.Payload["failure_type"].(string)
		return fmt.Sprintf("fix uptime failure: %s (%s)", url, ft)
	default:
		desc, _ := sig.Payload["description"].(string)
		runes := []rune(desc)
		if len(runes) > 60 {
			return string(runes[:60]) + "..."
		}
		return desc
	}
}
