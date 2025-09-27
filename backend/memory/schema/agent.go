package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
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
		field.Bool("builtin").Default(false),

		field.UUID("model_id", uuid.UUID{}).Optional(),
	}
}

func (Agent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("model", Model.Type).Field("model_id").Unique(),
		edge.From("tasks", Task.Type).Ref("agent"),
		edge.From("messages", Message.Type).Ref("agent"),
	}
}

func (Agent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").
			Unique(),
	}
}

func (Agent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
