package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Model struct {
	ent.Schema
}

func (Model) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.UUID("model_provider", uuid.UUID{}),
	}
}

func (Model) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agents", Agent.Type).Ref("model"),
		edge.To("model_providers", ModelProvider.Type).Field("model_provider").Unique().Required(),
		edge.From("messages", Message.Type).Ref("model"),
	}
}

func (Model) Mixin() []ent.Mixin {
	return []ent.Mixin{
		// mixin.Time{},
	}
}
