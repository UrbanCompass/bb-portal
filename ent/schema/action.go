package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Action holds the schema definition for the Action entity.
type Action struct {
	ent.Schema
}

// Annotations of the Action.
func (Action) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField("findActions"),
	}
}

// Fields of the Action.
func (Action) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("bazel_invocation_id").
			Comment("The id of the bazel invocation").
			Immutable().
			Unique().
			Annotations(
				entgql.Skip(),
			),

		field.Int64("configuration_id").
			Comment("The id of the configuration").
			Immutable().
			Unique().
			Annotations(
				entgql.Skip(),
			),

		field.String("label"),
		field.String("type").Optional(),

		field.Bool("success").Optional(),
		field.Int32("exit_code").Optional(),

		field.Strings("command_line").Optional(),

		field.Time("start_time").Optional(),
		field.Time("end_time").Optional(),

		field.String("failure_code").Optional(),
		field.String("failure_message").Optional(),

		field.String("primary_output").
			Comment("The primary output of the action, used to match the same action across invocations").
			Optional(),
		field.Bool("sampled").
			Comment("True if this is a successful action kept by sampling. Failed actions are always stored").
			Optional(),
		field.Int64("sample_key").
			Comment("Sampling key. The successful actions with the lowest keys are kept").
			Optional().
			Annotations(entgql.Skip()),

		field.Int64("stdout_file_id").
			Optional().
			Immutable().
			Annotations(entgql.Skip()),

		field.Int64("stderr_file_id").
			Optional().
			Immutable().
			Annotations(entgql.Skip()),
	}
}

// Edges of the Action.
func (Action) Edges() []ent.Edge {
	return []ent.Edge{
		// Edge back to the bazel invocation.
		edge.From("bazel_invocation", BazelInvocation.Type).
			Field("bazel_invocation_id").
			Ref("actions").
			Unique().
			Required().
			Immutable(),

		// Edge to the configuration.
		edge.To("configuration", Configuration.Type).
			Field("configuration_id").
			Unique().
			Required().
			Immutable(),

		edge.To("stdout", File.Type).
			Field("stdout_file_id").
			Unique().
			Immutable(),

		edge.To("stderr", File.Type).
			Field("stderr_file_id").
			Unique().
			Immutable(),
	}
}

// Indexes for Action.
func (Action) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("label"),
		index.Fields("primary_output"),
		// Also serves as the index for the bazel_invocation edge.
		index.Fields("bazel_invocation_id", "sampled", "sample_key"),
		index.Edges("configuration"),
		index.Edges("stdout"),
		index.Edges("stderr"),
	}
}

// Mixin of the Action.
func (Action) Mixin() []ent.Mixin {
	return []ent.Mixin{
		Int64IdMixin{},
	}
}
