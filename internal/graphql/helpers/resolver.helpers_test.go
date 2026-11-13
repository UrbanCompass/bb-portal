package helpers_test

import (
	"testing"

	"entgo.io/contrib/entgql"
	"github.com/buildbarn/bb-portal/internal/graphql/helpers"
	"github.com/stretchr/testify/require"
)

func TestRestoreZeroCursorValue(t *testing.T) {
	t.Run("PutsBackAMissingValue", func(t *testing.T) {
		after := &entgql.Cursor[int64]{ID: 159}
		before := &entgql.Cursor[int64]{ID: 7}

		helpers.RestoreZeroCursorValue(after, before)

		require.Equal(t, int64(0), after.Value)
		require.Equal(t, int64(0), before.Value)
		require.Equal(t, int64(159), after.ID)
	})

	t.Run("KeepsAnExistingValue", func(t *testing.T) {
		after := &entgql.Cursor[int64]{ID: 210, Value: int64(700)}

		helpers.RestoreZeroCursorValue(after, nil)

		require.Equal(t, int64(700), after.Value)
	})

	t.Run("IgnoresMissingCursors", func(t *testing.T) {
		require.NotPanics(t, func() { helpers.RestoreZeroCursorValue(nil, nil) })
	})
}
