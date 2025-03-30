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
	}
}

func (Message) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("task", Task.Type).Ref("messages").Unique(),
	}
}

func (Message) Mixin() []ent.Mixin {
	return []ent.Mixin{
		AgentMixin{},
		mixin.Time{},
	}
}
