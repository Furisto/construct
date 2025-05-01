package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

type Message struct {
	ent.Schema
}

func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.Enum("source").GoType(types.MessageSource("")),
		field.JSON("content", &types.MessageContent{}),
		field.JSON("usage", &types.MessageUsage{}).Optional(),
		field.Time("processed_time").Optional(),

		field.UUID("task_id", uuid.UUID{}),
		field.UUID("agent_id", uuid.UUID{}).Optional(),
		field.UUID("model_id", uuid.UUID{}).Optional(),
	}
}

func (Message) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).Field("task_id").Unique().Required().Annotations(
			entsql.Annotation{
				OnDelete: entsql.Cascade,
			},
		),
		edge.To("agent", Agent.Type).Field("agent_id").Unique(),
		edge.To("model", Model.Type).Field("model_id").Unique(),
	}
}

func (Message) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

func (Message) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Checks: map[string]string{
				"agent_model": "(agent_id IS NULL OR agent_id IS NOT NULL AND model_id IS NOT NULL)",
			},
		},
	}
}
