package graphql

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/buildbarn/bb-portal/ent/gen/ent"
	"github.com/buildbarn/bb-portal/internal/database/dbauthservice"
	"github.com/buildbarn/bb-portal/internal/database/embedded"
	"github.com/buildbarn/bb-portal/test/testutils"
	"github.com/stretchr/testify/require"

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

func TestFindActions(t *testing.T) {
	ctx := dbauthservice.NewContextWithDbAuthServiceBypass(context.Background())
	db := testutils.SetupTestDB(t, dbProvider)
	client := db.Ent()
	instanceName := testutils.CreateInstanceName(ctx, t, client, "test")

	// Two invocations containing the same action (same label, type and primary
	// output), as a comparison UI would look it up across runs.
	var invocationIDs []int64
	for i := 0; i < 2; i++ {
		inv, err := testutils.StartCreateInvocation(client, instanceName).Save(ctx)
		require.NoError(t, err)
		cfg, err := client.Configuration.Create().
			SetConfigurationID("cfg").
			SetBazelInvocationID(inv.ID).
			Save(ctx)
		require.NoError(t, err)
		invocationIDs = append(invocationIDs, inv.ID)

		create := func(label, typ, out string, sampled bool) {
			_, err := client.Action.Create().
				SetBazelInvocationID(inv.ID).
				SetConfigurationID(cfg.ID).
				SetLabel(label).
				SetType(typ).
				SetPrimaryOutput(out).
				SetSampled(sampled).
				Save(ctx)
			require.NoError(t, err)
		}
		create("//a:a", "GoCompilePkg", "bazel-out/a.a", true)
		create("//b:b", "GoCompilePkg", "bazel-out/b.a", true)
		create("//a:a", "Genrule", "bazel-out/a.txt", false)
	}

	r := &queryResolver{&Resolver{db: db}}
	find := func(where *ent.ActionWhereInput) *ent.ActionConnection {
		conn, err := r.FindActions(ctx, nil, nil, nil, nil, where)
		require.NoError(t, err)
		return conn
	}
	str := func(s string) *string { return &s }

	t.Run("NoFilterReturnsAll", func(t *testing.T) {
		require.Equal(t, 6, find(&ent.ActionWhereInput{}).TotalCount)
	})

	t.Run("ByLabelAndTypeAcrossInvocations", func(t *testing.T) {
		conn := find(&ent.ActionWhereInput{Label: str("//a:a"), Type: str("GoCompilePkg")})
		require.Equal(t, 2, conn.TotalCount)
		got := map[int64]bool{}
		for _, e := range conn.Edges {
			require.Equal(t, "bazel-out/a.a", e.Node.PrimaryOutput)
			got[e.Node.BazelInvocationID] = true
		}
		require.Len(t, got, 2, "expected one match per invocation")
		for _, id := range invocationIDs {
			require.True(t, got[id])
		}
	})

	t.Run("ByPrimaryOutput", func(t *testing.T) {
		conn := find(&ent.ActionWhereInput{PrimaryOutput: str("bazel-out/b.a")})
		require.Equal(t, 2, conn.TotalCount)
		for _, e := range conn.Edges {
			require.Equal(t, "//b:b", e.Node.Label)
		}
	})

	t.Run("BySampled", func(t *testing.T) {
		sampled := false
		require.Equal(t, 2, find(&ent.ActionWhereInput{Sampled: &sampled}).TotalCount)
	})

	t.Run("NoMatch", func(t *testing.T) {
		conn := find(&ent.ActionWhereInput{Label: str("//missing:x")})
		require.Equal(t, 0, conn.TotalCount)
		require.Empty(t, conn.Edges)
	})
}
