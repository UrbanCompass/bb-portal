package buildeventrecorder

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
)

func makeAny(typeURL string) *anypb.Any {
	return &anypb.Any{TypeUrl: typeURL}
}

func TestCacheStatusFromStrategyDetails(t *testing.T) {
	tests := []struct {
		name    string
		details []*anypb.Any
		want    string
	}{
		{
			name:    "nil list",
			details: nil,
			want:    "",
		},
		{
			name:    "empty list",
			details: []*anypb.Any{},
			want:    "",
		},
		{
			name:    "nil element",
			details: []*anypb.Any{nil},
			want:    "",
		},
		{
			name:    "RemoteSpawnMetrics suffix → remote cache hit",
			details: []*anypb.Any{makeAny("type.googleapis.com/build.bazel.lib.actions.RemoteSpawnMetrics")},
			want:    "remote cache hit",
		},
		{
			name:    "SpawnMetrics suffix → remote",
			details: []*anypb.Any{makeAny("type.googleapis.com/build.bazel.lib.actions.SpawnMetrics")},
			want:    "remote",
		},
		{
			name:    "unrecognized type URL with slash → tail after last separator",
			details: []*anypb.Any{makeAny("type.googleapis.com/foo.bar.LocalSpawnMetrics")},
			want:    "LocalSpawnMetrics",
		},
		{
			name:    "type URL with dot separator → tail after last dot",
			details: []*anypb.Any{makeAny("foo.LocalExecution")},
			want:    "LocalExecution",
		},
		{
			name:    "bare type URL with no separators → returned as-is",
			details: []*anypb.Any{makeAny("SomeType")},
			want:    "SomeType",
		},
		{
			name: "first non-nil entry wins",
			details: []*anypb.Any{
				nil,
				makeAny("type.googleapis.com/build.bazel.lib.actions.RemoteSpawnMetrics"),
				makeAny("type.googleapis.com/build.bazel.lib.actions.SpawnMetrics"),
			},
			want: "remote cache hit",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cacheStatusFromStrategyDetails(tc.details)
			if got != tc.want {
				t.Errorf("cacheStatusFromStrategyDetails(%v) = %q, want %q", tc.details, got, tc.want)
			}
		})
	}
}
