package buildeventrecorder_test

import (
	"context"
	"testing"

	"entgo.io/contrib/entgql"
	"github.com/buildbarn/bb-portal/ent/gen/ent"
	"github.com/buildbarn/bb-portal/internal/database"
	"github.com/buildbarn/bb-portal/internal/database/dbauthservice"
	"github.com/buildbarn/bb-portal/test/testutils"
	"github.com/stretchr/testify/require"
)

func TestUpdateTargetsMaxTestRunDuration(t *testing.T) {
	ctx := dbauthservice.NewContextWithDbAuthServiceBypass(context.Background())

	setup := func(t *testing.T) (database.Client, *ent.InstanceName) {
		db := testutils.SetupTestDB(t, dbProvider)
		return db, testutils.CreateInstanceName(ctx, t, db.Ent(), "testInstance")
	}
	createTarget := func(t *testing.T, db database.Client, instanceName *ent.InstanceName, label string) *ent.Target {
		target, err := db.Ent().Target.Create().
			SetLabel(label).
			SetAspect("").
			SetTargetKind("sh_test rule").
			SetInstanceName(instanceName).
			Save(ctx)
		require.NoError(t, err)
		return target
	}
	// Records a test run of the target in the invocation. A nil duration
	// records a test summary that has no duration set.
	addTestSummary := func(t *testing.T, db database.Client, invocation *ent.BazelInvocation, target *ent.Target, durationInMs *int64) {
		invocationTarget, err := db.Ent().InvocationTarget.Create().
			SetBazelInvocation(invocation).
			SetTarget(target).
			SetAbortReason("NONE").
			Save(ctx)
		require.NoError(t, err)
		create := db.Ent().TestSummary.Create().SetInvocationTarget(invocationTarget)
		if durationInMs != nil {
			create.SetTotalRunDurationInMs(*durationInMs)
		}
		require.NoError(t, create.Exec(ctx))
	}
	newInvocation := func(t *testing.T, db database.Client, instanceName *ent.InstanceName) *ent.BazelInvocation {
		invocation, err := testutils.StartCreateInvocation(db.Ent(), instanceName).Save(ctx)
		require.NoError(t, err)
		return invocation
	}
	update := func(t *testing.T, db database.Client, invocation *ent.BazelInvocation) {
		require.NoError(t, db.Sqlc().UpdateTargetsMaxTestRunDuration(ctx, invocation.ID))
	}
	maxDuration := func(t *testing.T, db database.Client, target *ent.Target) int64 {
		got, err := db.Ent().Target.Get(ctx, target.ID)
		require.NoError(t, err)
		return got.MaxTestRunDurationInMs
	}
	ptr := func(v int64) *int64 { return &v }

	t.Run("DefaultsToZero", func(t *testing.T) {
		db, instanceName := setup(t)
		target := createTarget(t, db, instanceName, "//pkg:a_test")
		require.Equal(t, int64(0), maxDuration(t, db, target))
	})

	t.Run("TakesLongestSummaryOfInvocation", func(t *testing.T) {
		db, instanceName := setup(t)
		target := createTarget(t, db, instanceName, "//pkg:a_test")
		invocation := newInvocation(t, db, instanceName)
		// The same target can be built in more than one configuration.
		addTestSummary(t, db, invocation, target, ptr(100))
		addTestSummary(t, db, invocation, target, ptr(300))
		addTestSummary(t, db, invocation, target, ptr(200))

		update(t, db, invocation)

		require.Equal(t, int64(300), maxDuration(t, db, target))
	})

	t.Run("NeverLowersTheValue", func(t *testing.T) {
		db, instanceName := setup(t)
		target := createTarget(t, db, instanceName, "//pkg:a_test")

		slow := newInvocation(t, db, instanceName)
		addTestSummary(t, db, slow, target, ptr(300))
		update(t, db, slow)
		require.Equal(t, int64(300), maxDuration(t, db, target))

		fast := newInvocation(t, db, instanceName)
		addTestSummary(t, db, fast, target, ptr(50))
		update(t, db, fast)
		require.Equal(t, int64(300), maxDuration(t, db, target), "a faster run must not lower the value")

		slower := newInvocation(t, db, instanceName)
		addTestSummary(t, db, slower, target, ptr(500))
		update(t, db, slower)
		require.Equal(t, int64(500), maxDuration(t, db, target), "a slower run must raise the value")
	})

	t.Run("OnlyUpdatesTargetsOfTheInvocation", func(t *testing.T) {
		db, instanceName := setup(t)
		inInvocation := createTarget(t, db, instanceName, "//pkg:a_test")
		elsewhere := createTarget(t, db, instanceName, "//pkg:b_test")

		first := newInvocation(t, db, instanceName)
		addTestSummary(t, db, first, inInvocation, ptr(100))
		other := newInvocation(t, db, instanceName)
		addTestSummary(t, db, other, elsewhere, ptr(900))

		update(t, db, first)

		require.Equal(t, int64(100), maxDuration(t, db, inInvocation))
		require.Equal(t, int64(0), maxDuration(t, db, elsewhere))
	})

	t.Run("IgnoresSummaryWithoutDuration", func(t *testing.T) {
		db, instanceName := setup(t)
		target := createTarget(t, db, instanceName, "//pkg:a_test")
		invocation := newInvocation(t, db, instanceName)
		addTestSummary(t, db, invocation, target, nil)

		update(t, db, invocation)

		require.Equal(t, int64(0), maxDuration(t, db, target))
	})

	t.Run("SortsTargetsByDuration", func(t *testing.T) {
		db, instanceName := setup(t)
		invocation := newInvocation(t, db, instanceName)
		for label, duration := range map[string]int64{
			"//pkg:slow_test":   9000,
			"//pkg:medium_test": 2000,
			"//pkg:fast_test":   10,
		} {
			addTestSummary(t, db, invocation, createTarget(t, db, instanceName, label), ptr(duration))
		}
		// A target that has not run any test yet, such as one that existed
		// before this column was added.
		createTarget(t, db, instanceName, "//pkg:never_ran_test")
		update(t, db, invocation)

		labels := func(direction entgql.OrderDirection) []string {
			first := 10
			connection, err := db.Ent().Target.Query().Paginate(ctx, nil, &first, nil, nil,
				ent.WithTargetOrder(&ent.TargetOrder{
					Direction: direction,
					Field:     ent.TargetOrderFieldMaxTestRunDurationInMs,
				}))
			require.NoError(t, err)
			var got []string
			for _, edge := range connection.Edges {
				got = append(got, edge.Node.Label)
			}
			return got
		}

		require.Equal(t,
			[]string{"//pkg:slow_test", "//pkg:medium_test", "//pkg:fast_test", "//pkg:never_ran_test"},
			labels(entgql.OrderDirectionDesc),
			"slowest first, with targets that never ran last")
		require.Equal(t,
			[]string{"//pkg:never_ran_test", "//pkg:fast_test", "//pkg:medium_test", "//pkg:slow_test"},
			labels(entgql.OrderDirectionAsc))
	})
}
