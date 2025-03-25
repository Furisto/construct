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
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique(),
		field.String("name"),
		field.Int64("context_window"),
		field.Bool("enabled").Default(true),
	}
}

func (Model) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("model_provider", ModelProvider.Type).Ref("models").Unique(),
	}
}
