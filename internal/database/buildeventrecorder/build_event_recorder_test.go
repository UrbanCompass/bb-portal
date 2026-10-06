package buildeventrecorder_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	bes "github.com/bazelbuild/bazel/src/main/java/com/google/devtools/build/lib/buildeventstream/proto"
	"github.com/buildbarn/bb-portal/ent/gen/ent/configuration"
	"github.com/buildbarn/bb-portal/internal/database/buildeventrecorder"
	"github.com/buildbarn/bb-portal/internal/database/dbauthservice"
	"github.com/buildbarn/bb-portal/internal/database/embedded"
	"github.com/buildbarn/bb-portal/test/testutils"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	_ "github.com/buildbarn/bb-portal/ent/gen/ent/runtime"
)

var dbProvider *embedded.DatabaseProvider

func TestMain(m *testing.M) {
	var err error
	dbProvider, err = embedded.NewDatabaseProvider(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not start embedded DB: %v\n", err)
		os.Exit(1)
	}
	defer dbProvider.Cleanup()
	m.Run()
}

func TestFindOrCreateInstanceName(t *testing.T) {
	ctx := context.Background()

	t.Run("CreatesAndReturnsIdempotently", func(t *testing.T) {
		db := testutils.SetupTestDB(t, dbProvider)

		firstID, err := buildeventrecorder.FindOrCreateInstanceName(ctx, db, "testInstance")
		require.NoError(t, err)
		secondID, err := buildeventrecorder.FindOrCreateInstanceName(ctx, db, "testInstance")
		require.NoError(t, err)
		require.Equal(t, firstID, secondID)
	})
}

func TestFindOrCreateInvocation(t *testing.T) {
	ctx := dbauthservice.NewContextWithDbAuthServiceBypass(context.Background())

	invocationsGaugeVec := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "test_bazel_invocations_total",
			Help: "Test gauge for bazel invocations.",
		},
		[]string{"user_type"},
	)

	t.Run("ReconnectExistingUnlocked", func(t *testing.T) {
		db := testutils.SetupTestDB(t, dbProvider)
		instanceNameDbID, err := buildeventrecorder.FindOrCreateInstanceName(ctx, db, "testInstance")
		require.NoError(t, err)
		invocationId := uuid.New()

		firstID, err := buildeventrecorder.FindOrCreateInvocation(
			ctx, db, invocationId, instanceNameDbID, nil, invocationsGaugeVec,
		)
		require.NoError(t, err)
		secondID, err := buildeventrecorder.FindOrCreateInvocation(
			ctx, db, invocationId, instanceNameDbID, nil, invocationsGaugeVec,
		)
		require.NoError(t, err, "Failed to reconnect to unlocked invocation")
		require.Equal(t, firstID, secondID)
	})

	t.Run("FailToReconnectLockedInvocation", func(t *testing.T) {
		db := testutils.SetupTestDB(t, dbProvider)
		client := db.Ent()
		instanceNameDbID, err := buildeventrecorder.FindOrCreateInstanceName(ctx, db, "testInstance")
		require.NoError(t, err)
		invocationID := uuid.New()

		id, err := buildeventrecorder.FindOrCreateInvocation(
			ctx, db, invocationID, instanceNameDbID, nil, invocationsGaugeVec,
		)
		require.NoError(t, err)
		err = client.BazelInvocation.UpdateOneID(id).SetBepCompleted(true).Exec(ctx)
		require.NoError(t, err)
		_, err = buildeventrecorder.FindOrCreateInvocation(
			ctx, db, invocationID, instanceNameDbID, nil, invocationsGaugeVec,
		)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok, "Expected a gRPC status error")
		require.Equal(t, codes.FailedPrecondition, st.Code())
		require.Contains(t, err.Error(), "locked for writing")
	})
}

func TestSaveActionExecutedUnannouncedConfiguration(t *testing.T) {
	ctx := dbauthservice.NewContextWithDbAuthServiceBypass(context.Background())
	db := testutils.SetupTestDB(t, dbProvider)
	client := db.Ent()
	instanceName := testutils.CreateInstanceName(ctx, t, client, "test")
	inv, err := testutils.StartCreateInvocation(client, instanceName).Save(ctx)
	require.NoError(t, err)

	save := func(label, configID string) error {
		return buildeventrecorder.SaveActionExecutedForTest(ctx, db, inv.ID,
			&bes.ActionExecuted{Success: true, Type: "Symlink"},
			&bes.BuildEventId_ActionCompletedId{
				Label:         label,
				PrimaryOutput: "bazel-out/" + label,
				Configuration: &bes.BuildEventId_ConfigurationId{Id: configID},
			})
	}

	// Bazel references the "system" configuration from ActionExecuted events
	// without ever announcing it in a Configuration event. That must not fail
	// the batch.
	require.NoError(t, save("//a:a", "system"))
	require.NoError(t, save("//b:b", "system"))

	configs, err := client.Configuration.Query().
		Where(configuration.ConfigurationID("system")).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, configs, 1, "the unannounced configuration should be created once and reused")

	actions, err := client.Action.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, actions, 2)
	for _, a := range actions {
		require.Equal(t, configs[0].ID, a.ConfigurationID)
		require.True(t, a.Sampled)
	}
}

// TestSaveRemainingBatchRollbackRestoresSampler checks that when the batch
// transaction rolls back after evicting sampled rows, the in-memory sampler
// state is restored, so the retry neither fails nor leaves orphan rows.
func TestSaveRemainingBatchRollbackRestoresSampler(t *testing.T) {
	ctx := dbauthservice.NewContextWithDbAuthServiceBypass(context.Background())
	db := testutils.SetupTestDB(t, dbProvider)
	client := db.Ent()
	instanceName := testutils.CreateInstanceName(ctx, t, client, "test")
	inv, err := testutils.StartCreateInvocation(client, instanceName).Save(ctx)
	require.NoError(t, err)

	const sampleCap = 2
	rec := buildeventrecorder.NewRecorderForTest(db, inv.ID, sampleCap)
	require.NoError(t, rec.LoadHandledEvents(ctx))

	batchOf := func(from, to int) []buildeventrecorder.BuildEventWithInfo {
		var batch []buildeventrecorder.BuildEventWithInfo
		for i := from; i <= to; i++ {
			label := fmt.Sprintf("//pkg:t%d", i)
			batch = append(batch, buildeventrecorder.BuildEventWithInfo{
				SequenceNumber: uint32(i),
				Event: &bes.BuildEvent{
					Id: &bes.BuildEventId{Id: &bes.BuildEventId_ActionCompleted{
						ActionCompleted: &bes.BuildEventId_ActionCompletedId{
							Label:         label,
							PrimaryOutput: "out/" + label,
							Configuration: &bes.BuildEventId_ConfigurationId{Id: "system"},
						},
					}},
					Payload: &bes.BuildEvent_Action{Action: &bes.ActionExecuted{Success: true}},
				},
			})
		}
		return batch
	}

	// Fill the reservoir.
	require.NoError(t, rec.SaveRemaining(ctx, batchOf(1, 2)))

	// This batch evicts slot 0 for each action, then rolls back.
	rec.BreakHandledEvents()
	require.Error(t, rec.SaveRemaining(ctx, batchOf(3, 4)))
	count, err := client.Action.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, sampleCap, count, "rollback must leave the original rows")

	// The retry replays the batch against restored sampler state. Without the
	// restore it deletes a stale ID and fails, or leaves an orphan row.
	require.NoError(t, rec.LoadHandledEvents(ctx))
	require.NoError(t, rec.SaveRemaining(ctx, batchOf(3, 4)))
	count, err = client.Action.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, sampleCap, count, "the cap must hold after the retry")
}
