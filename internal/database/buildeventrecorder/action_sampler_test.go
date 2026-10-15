package buildeventrecorder_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	bes "github.com/bazelbuild/bazel/src/main/java/com/google/devtools/build/lib/buildeventstream/proto"
	"github.com/buildbarn/bb-portal/ent/gen/ent/action"
	"github.com/buildbarn/bb-portal/internal/database"
	"github.com/buildbarn/bb-portal/internal/database/buildeventrecorder"
	"github.com/buildbarn/bb-portal/internal/database/dbauthservice"
	"github.com/buildbarn/bb-portal/test/testutils"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestActionSampleStrategyRegistry(t *testing.T) {
	require.Equal(t, []string{"consistent", "first", "random"}, buildeventrecorder.ActionSampleStrategyNames())
}

func TestActionSamplingConfigFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name         string
		strategy     string
		cap          string
		wantStrategy string
		wantCap      int
	}{
		{name: "Unset", wantStrategy: "random", wantCap: 100},
		{name: "Valid", strategy: "consistent", cap: "25", wantStrategy: "consistent", wantCap: 25},
		{name: "Zero", strategy: "first", cap: "0", wantStrategy: "first", wantCap: 0},
		{name: "Invalid", strategy: "bogus", cap: "-1", wantStrategy: "random", wantCap: 100},
		{name: "NotANumber", cap: "lots", wantStrategy: "random", wantCap: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(buildeventrecorder.ActionSampleStrategyEnv, tc.strategy)
			t.Setenv(buildeventrecorder.ActionSampleCapEnv, tc.cap)
			strategy, sampleCap := buildeventrecorder.ActionSamplingConfigFromEnvForTest()
			require.Equal(t, buildeventrecorder.ActionSampleStrategyForTest(tc.wantStrategy), strategy)
			require.Equal(t, tc.wantCap, sampleCap)
		})
	}
}

func TestConsistentActionSampleStrategy(t *testing.T) {
	strategy := buildeventrecorder.ActionSampleStrategyForTest("consistent")
	id := func(output string) *bes.BuildEventId_ActionCompletedId {
		return &bes.BuildEventId_ActionCompletedId{Label: "//a:b", PrimaryOutput: output}
	}
	executed := &bes.ActionExecuted{Type: "GoCompile"}

	key := strategy.SampleKey(id("bazel-out/a.a"), executed)
	require.Equal(t, key, strategy.SampleKey(id("bazel-out/a.a"), executed))
	require.NotEqual(t, key, strategy.SampleKey(id("bazel-out/b.a"), executed))
	require.GreaterOrEqual(t, key, int64(0))
}

func TestSaveActionExecutedSampling(t *testing.T) {
	ctx := dbauthservice.NewContextWithDbAuthServiceBypass(context.Background())

	setup := func(t *testing.T) (database.Client, int64) {
		db := testutils.SetupTestDB(t, dbProvider)
		instanceName := testutils.CreateInstanceName(ctx, t, db.Ent(), "testInstance")
		invocation, err := testutils.StartCreateInvocation(db.Ent(), instanceName).Save(ctx)
		require.NoError(t, err)
		return db, invocation.ID
	}
	completedID := func(i int) *bes.BuildEventId_ActionCompletedId {
		return &bes.BuildEventId_ActionCompletedId{
			Label:         fmt.Sprintf("//pkg:target%d", i),
			PrimaryOutput: fmt.Sprintf("bazel-out/bin/pkg/out%d", i),
			Configuration: &bes.BuildEventId_ConfigurationId{Id: "system"},
		}
	}
	countActions := func(t *testing.T, db database.Client, invocationDbID int64, success bool) int {
		n, err := db.Ent().Action.Query().
			Where(action.BazelInvocationID(invocationDbID), action.Success(success)).
			Count(ctx)
		require.NoError(t, err)
		return n
	}

	for _, strategyName := range buildeventrecorder.ActionSampleStrategyNames() {
		t.Run("BoundedStorage/"+strategyName, func(t *testing.T) {
			strategy := buildeventrecorder.ActionSampleStrategyForTest(strategyName)
			for _, numActions := range []int{3, 10, 50} {
				db, invocationDbID := setup(t)
				for i := 0; i < numActions; i++ {
					require.NoError(t, buildeventrecorder.SaveActionExecutedForTest(
						ctx, db, invocationDbID, strategy, 10,
						&bes.ActionExecuted{Success: true, Type: "Genrule"}, completedID(i)))
					// Interleave failures, which are never evicted.
					if i%10 == 0 {
						require.NoError(t, buildeventrecorder.SaveActionExecutedForTest(
							ctx, db, invocationDbID, strategy, 10,
							&bes.ActionExecuted{Success: false, ExitCode: 1, Type: "Genrule"}, completedID(1000+i)))
					}
				}
				require.Equal(t, min(numActions, 10), countActions(t, db, invocationDbID, true))
				require.Equal(t, (numActions+9)/10, countActions(t, db, invocationDbID, false))
			}
		})
	}

	t.Run("FirstKeepsEarliest", func(t *testing.T) {
		db, invocationDbID := setup(t)
		strategy := buildeventrecorder.ActionSampleStrategyForTest("first")
		for i := 0; i < 5; i++ {
			require.NoError(t, buildeventrecorder.SaveActionExecutedForTest(
				ctx, db, invocationDbID, strategy, 2,
				&bes.ActionExecuted{Success: true}, completedID(i)))
		}
		labels, err := db.Ent().Action.Query().
			Where(action.BazelInvocationID(invocationDbID)).
			Order(action.ByID()).
			Select(action.FieldLabel).
			Strings(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"//pkg:target0", "//pkg:target1"}, labels)
	})

	t.Run("ConsistentKeepsSameActionsAcrossInvocations", func(t *testing.T) {
		strategy := buildeventrecorder.ActionSampleStrategyForTest("consistent")
		sampledOutputs := func(order []int) []string {
			db, invocationDbID := setup(t)
			for _, i := range order {
				require.NoError(t, buildeventrecorder.SaveActionExecutedForTest(
					ctx, db, invocationDbID, strategy, 5,
					&bes.ActionExecuted{Success: true, Type: "Genrule"}, completedID(i)))
			}
			outputs, err := db.Ent().Action.Query().
				Where(action.BazelInvocationID(invocationDbID)).
				Order(action.ByPrimaryOutput()).
				Select(action.FieldPrimaryOutput).
				Strings(ctx)
			require.NoError(t, err)
			return outputs
		}
		forward := make([]int, 40)
		backward := make([]int, 40)
		for i := range forward {
			forward[i] = i
			backward[i] = 39 - i
		}
		// The same actions completing in a different order are sampled
		// identically.
		require.Equal(t, sampledOutputs(forward), sampledOutputs(backward))
	})

	t.Run("CapZeroStoresOnlyFailures", func(t *testing.T) {
		db, invocationDbID := setup(t)
		strategy := buildeventrecorder.ActionSampleStrategyForTest("random")
		require.NoError(t, buildeventrecorder.SaveActionExecutedForTest(
			ctx, db, invocationDbID, strategy, 0, &bes.ActionExecuted{Success: true}, completedID(0)))
		require.NoError(t, buildeventrecorder.SaveActionExecutedForTest(
			ctx, db, invocationDbID, strategy, 0, &bes.ActionExecuted{Success: false}, completedID(1)))
		require.Equal(t, 0, countActions(t, db, invocationDbID, true))
		require.Equal(t, 1, countActions(t, db, invocationDbID, false))
	})

	t.Run("StoresFields", func(t *testing.T) {
		db, invocationDbID := setup(t)
		strategy := buildeventrecorder.ActionSampleStrategyForTest("random")
		start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		end := start.Add(time.Minute)
		require.NoError(t, buildeventrecorder.SaveActionExecutedForTest(
			ctx, db, invocationDbID, strategy, 10,
			&bes.ActionExecuted{
				Success:   true,
				Type:      "GoCompile",
				StartTime: timestamppb.New(start),
				EndTime:   timestamppb.New(end),
			},
			completedID(0)))
		got, err := db.Ent().Action.Query().
			Where(action.BazelInvocationID(invocationDbID)).
			WithConfiguration().
			Only(ctx)
		require.NoError(t, err)
		require.True(t, got.StartTime.Equal(start), "start_time = %v", got.StartTime)
		require.True(t, got.EndTime.Equal(end), "end_time = %v", got.EndTime)
		require.Equal(t, "bazel-out/bin/pkg/out0", got.PrimaryOutput)
		require.True(t, got.Sampled)
		// "system" is never announced in a Configuration event.
		require.Equal(t, "system", got.Edges.Configuration.ConfigurationID)
	})
}
