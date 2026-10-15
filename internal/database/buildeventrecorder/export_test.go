package buildeventrecorder

import (
	"context"

	bes "github.com/bazelbuild/bazel/src/main/java/com/google/devtools/build/lib/buildeventstream/proto"
	"github.com/buildbarn/bb-portal/internal/database"
)

// SaveActionExecutedForTest runs saveActionExecuted in its own transaction for
// an invocation that already exists, sampling with the given strategy and cap.
func SaveActionExecutedForTest(ctx context.Context, db database.Client, invocationDbID int64, strategy ActionSampleStrategy, sampleCap int, actionExecuted *bes.ActionExecuted, id *bes.BuildEventId_ActionCompletedId) error {
	r := &buildEventRecorder{
		db:             db,
		InvocationDbID: invocationDbID,
		actionSampling: actionSamplingConfig{strategy: strategy, cap: sampleCap},
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := r.saveActionExecuted(ctx, tx, actionExecuted, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ActionSampleStrategyForTest returns the registered strategy with the given
// name, or nil.
func ActionSampleStrategyForTest(name string) ActionSampleStrategy {
	return actionSampleStrategies[name]
}

// ActionSamplingConfigFromEnvForTest exposes the strategy and cap read from
// the environment.
func ActionSamplingConfigFromEnvForTest() (ActionSampleStrategy, int) {
	cfg := actionSamplingConfigFromEnv()
	return cfg.strategy, cfg.cap
}
