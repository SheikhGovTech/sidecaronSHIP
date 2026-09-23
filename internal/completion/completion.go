// Package completion defines Sidecar-owned terminal records. These outcomes
// are independent of Harness stop reasons: exhausting a runtime is not
// equivalent to completing the stage's contract.
package completion

import "github.com/google/uuid"

type Outcome string

const (
	CodingChanged        Outcome = "changed"
	CodingNoChange       Outcome = "no_change"
	CodingIncomplete     Outcome = "coding_incomplete"
	EvaluationPass       Outcome = "pass"
	EvaluationReject     Outcome = "reject"
	EvaluationIncomplete Outcome = "evaluation_incomplete"
	EvaluationError      Outcome = "evaluation_error"
)

type StageResult struct {
	Outcome      Outcome
	StopReason   string
	Attempt      int
	RequestsUsed int
	TraceID      uuid.UUID
	Summary      string
}

func (r StageResult) Payload() map[string]any {
	return map[string]any{
		"outcome": r.Outcome, "stop_reason": r.StopReason, "attempt": r.Attempt,
		"requests_used": r.RequestsUsed, "trace_id": r.TraceID.String(), "summary": r.Summary,
	}
}

type Handoff struct {
	Reason            Outcome
	Summary           string
	StopReason        string
	Completed         []string
	Remaining         []string
	RecommendedAction string
	TaskID            uuid.UUID
	TraceID           uuid.UUID
}

func (h Handoff) Payload() map[string]any {
	return map[string]any{
		"reason": h.Reason, "summary": h.Summary, "stop_reason": h.StopReason,
		"completed": h.Completed, "remaining": h.Remaining,
		"recommended_action": h.RecommendedAction,
		"task_id":            h.TaskID.String(), "trace_id": h.TraceID.String(),
	}
}
