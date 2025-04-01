package schema

import (
	"entgo.io/ent"
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
		field.JSON("content", &types.MessageContent{}),
		field.Enum("role").GoType(types.MessageRole("")),
		field.JSON("usage", &types.MessageUsage{}).Optional(),
		field.Time("processed_time").Optional(),

		field.UUID("task_id", uuid.UUID{}),
		field.UUID("agent_id", uuid.UUID{}).Optional(),
		field.UUID("model_id", uuid.UUID{}).Optional(),
	}
}

func (Message) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).Field("task_id").Unique().Required(),
		edge.To("agent", Agent.Type).Field("agent_id").Unique(),
		edge.To("model", Model.Type).Field("model_id").Unique(),
		// edge.From("task", Task.Type).Ref("messages").Unique().Field("task_id"),
	}
}

func (Message) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
