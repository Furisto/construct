package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

type AgentMixin struct {
	mixin.Schema
}

func (AgentMixin) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("agent_id", uuid.UUID{}).Default(uuid.New).Unique(),
	}
}

func (AgentMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_id"),
	}
}
