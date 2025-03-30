package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
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
	}
}

func (Agent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("model", Model.Type).Ref("agents").Unique(),
		edge.To("delegators", Agent.Type).
			From("delegates"),
	}
}

func (Agent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
