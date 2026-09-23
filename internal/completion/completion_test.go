package completion_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/sausheong/sidecar/internal/completion"
	"github.com/stretchr/testify/assert"
)

func TestStageResultPayloadKeepsIncompleteDistinct(t *testing.T) {
	traceID := uuid.New()
	payload := (completion.StageResult{Outcome: completion.EvaluationIncomplete, StopReason: "verdict missing",
		Attempt: 1, RequestsUsed: 8, TraceID: traceID}).Payload()
	assert.Equal(t, completion.EvaluationIncomplete, payload["outcome"])
	assert.NotEqual(t, completion.EvaluationReject, payload["outcome"])
	assert.Equal(t, traceID.String(), payload["trace_id"])
}
