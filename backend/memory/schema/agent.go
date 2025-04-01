package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Agent struct {
	ent.Schema
}

func (Agent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.String("name").NotEmpty(),
		field.String("description").Optional(),
		field.String("instructions"),

		field.UUID("default_model", uuid.UUID{}),
	}
}

func (Agent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("model", Model.Type).Field("default_model").Unique().Required(),
		edge.From("tasks", Task.Type).Ref("agent"),
		edge.From("messages", Message.Type).Ref("agent"),
	}
}

func (Agent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		// mixin.Time{},
	}
}
