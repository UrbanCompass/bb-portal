package schema

import (
	"time"

	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Target holds the schema definition for the Target entity.
type Target struct {
	ent.Schema
}

// Fields of the Target.
func (Target) Fields() []ent.Field {
	return []ent.Field{
		// Targets are only ever added with a higher ID than the ones before,
		// so ordering by ID DESC lists the newest targets first.
		field.Int64("id").
			Annotations(entgql.OrderField("ID")),

		// The label of the target ex: //foo:bar.
		field.String("label"),

		// The aspect of the target completion if any.
		field.String("aspect"),

		// The kind of target.
		// (e.g.,  e.g. "cc_library rule", "source file",
		// "generated file") where the completion is reported.
		field.String("target_kind"),

		// The longest total run duration of this target's tests across all
		// invocations. Stays 0 for targets that are not tests or have no
		// recorded run. Kept up to date when test summaries are saved, so that
		// targets can be sorted by it without scanning test summaries. It is
		// not nullable on purpose: NULLs sort first on DESC and break cursor
		// pagination.
		field.Int64("max_test_run_duration_in_ms").Default(0).
			Annotations(entgql.OrderField("MAX_TEST_RUN_DURATION_IN_MS")),

		// When the most recent invocation that ran this target's tests was
		// created. Stays at the Unix epoch for targets that have no recorded
		// run. Kept up to date together with max_test_run_duration_in_ms. It is
		// not nullable for the same reason as that column.
		field.Time("last_test_build_at").
			Default(func() time.Time { return time.Unix(0, 0).UTC() }).
			Annotations(
				entgql.OrderField("LAST_TEST_BUILD_AT"),
				entsql.Default("'1970-01-01 00:00:00+00'"),
			),
	}
}

// Edges of the Target.
func (Target) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("instance_name", InstanceName.Type).
			Ref("targets").
			Unique().
			Required(),

		// Target Data for the completed Invocation
		edge.To("invocation_targets", InvocationTarget.Type).
			Annotations(
				entsql.OnDelete(entsql.Cascade),
				entgql.RelayConnection(),
			),

		edge.To("target_kind_mappings", TargetKindMapping.Type).
			Annotations(
				entsql.OnDelete(entsql.Cascade),
			),

		edge.To("test_target", TestTarget.Type).
			Unique(),
	}
}

// Indexes of the Target.
func (Target) Indexes() []ent.Index {
	return []ent.Index{
		index.Edges("instance_name"),
		index.Fields("label", "aspect").
			Edges("instance_name"),
		index.Fields("label", "aspect", "target_kind").
			Edges("instance_name").
			Unique(),
	}
}

// Annotations of the Target
func (Target) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField("findTargets"),
	}
}

