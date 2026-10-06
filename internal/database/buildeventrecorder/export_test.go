package buildeventrecorder

import (
	"context"

	bes "github.com/bazelbuild/bazel/src/main/java/com/google/devtools/build/lib/buildeventstream/proto"
	"github.com/buildbarn/bb-portal/internal/database"
	"go.opentelemetry.io/otel/trace/noop"
)

// SaveActionExecutedForTest runs saveActionExecuted for an invocation that
// already exists, using the default sampling configuration.
func SaveActionExecutedForTest(ctx context.Context, db database.Client, invocationDbID int64, actionExecuted *bes.ActionExecuted, id *bes.BuildEventId_ActionCompletedId) error {
	r := &buildEventRecorder{
		db:             db,
		InvocationDbID: invocationDbID,
		actionSampler:  actionSamplingConfigFromEnv().newSampler(nil),
	}
	return r.saveActionExecuted(ctx, db, actionExecuted, id)
}

// RecorderForTest exposes the batch-saving path of a buildEventRecorder.
type RecorderForTest struct {
	r *buildEventRecorder
}

// NewRecorderForTest builds a recorder for an existing invocation. The
// reservoir sampler always draws 0, so every success after the cap displaces
// slot 0.
func NewRecorderForTest(db database.Client, invocationDbID int64, sampleCap int) *RecorderForTest {
	cfg := actionSamplingConfig{strategy: actionSampleStrategyReservoir, cap: sampleCap}
	return &RecorderForTest{r: &buildEventRecorder{
		db:             db,
		tracer:         noop.NewTracerProvider().Tracer("test"),
		InvocationDbID: invocationDbID,
		actionSampler:  cfg.newSampler(func(int) int { return 0 }),
	}}
}

// LoadHandledEvents loads the invocation's handled-event metadata.
func (t *RecorderForTest) LoadHandledEvents(ctx context.Context) error {
	return t.r.loadHandledEvents(ctx)
}

// BreakHandledEvents makes the final metadata update of saveRemainingBatch
// fail, so the transaction rolls back after the events were saved.
func (t *RecorderForTest) BreakHandledEvents() { t.r.handledEvents.id = -1 }

// SaveRemaining runs saveRemainingBatch.
func (t *RecorderForTest) SaveRemaining(ctx context.Context, batch []BuildEventWithInfo) error {
	return t.r.saveRemainingBatch(ctx, batch)
}
