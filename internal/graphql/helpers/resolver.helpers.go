package helpers

import (
	"time"

	"entgo.io/contrib/entgql"
)

// StringSliceArrayToPointerArray takes an array of strings and returns an array of string pointers
func StringSliceArrayToPointerArray(strings []string) []*string {
	result := make([]*string, len(strings))
	for i, str := range strings {
		result[i] = &str
	}
	return result
}

func paginationCursorToUTC(cursor *entgql.Cursor[int64]) {
	if cursor == nil || cursor.Value == nil {
		return
	}
	switch v := cursor.Value.(type) {
	case time.Time:
		cursor.Value = v.UTC()
	case *time.Time:
		if v != nil {
			ut := v.UTC()
			cursor.Value = &ut
		}
	}
}

// PaginationCursorsToUTC converts pagination cursors that consist of
// timestamps to UTC instead of local time. When the backend sends the cursors
// to the frontend, they are in UTC. However, when the frontend sends them
// back, they are interpreted as local time. This causes issues since Sqlite
// cannot handle comparisons between timestamps in different timezones.
func PaginationCursorsToUTC(after, before *entgql.Cursor[int64]) {
	paginationCursorToUTC(after)
	paginationCursorToUTC(before)
}

// RestoreZeroCursorValue puts back a sort value of 0 that entgql dropped from
// a cursor.
//
// entgql tags Cursor.Value with `msgpack:"omitempty"`, so a row whose sort
// value is 0 gets a cursor that has only the ID. When that cursor comes back,
// entgql sees no value and pages by ID alone, which repeats rows that have a
// higher sort value but a lower ID. This is only safe to call when sorting by
// a NOT NULL integer column, where a missing value can only have been 0. Do
// not call it when sorting by ID, because those cursors never have a value.
func RestoreZeroCursorValue(after, before *entgql.Cursor[int64]) {
	for _, cursor := range []*entgql.Cursor[int64]{after, before} {
		if cursor != nil && cursor.Value == nil {
			cursor.Value = int64(0)
		}
	}
}

// StringMapToAnyMap converts a map[string]string to a map[string]any. This is
// useful for converting from the database type to the Graphql type.
func StringMapToAnyMap(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
