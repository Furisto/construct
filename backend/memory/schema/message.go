package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Message struct {
	ent.Schema
}

func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.String("content").NotEmpty(),
		field.String("role").NotEmpty(),
	}
}

func (Message) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("task", Task.Type).Ref("messages"),
	}
}

func (Message) Mixin() []ent.Mixin {
	return []ent.Mixin{
		AgentMixin{},
	}
}

