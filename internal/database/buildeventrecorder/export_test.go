package buildeventrecorder

import (
	"context"

	bes "github.com/bazelbuild/bazel/src/main/java/com/google/devtools/build/lib/buildeventstream/proto"
	"github.com/buildbarn/bb-portal/internal/database"
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
