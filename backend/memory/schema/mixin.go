package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

// UserAgentID is the ID of the user
const UserAgentID = "00000000-0000-0000-0000-000000000002"

// ConstructAgentID is the ID of the construct agent
const ConstructAgentID = "00000000-0000-0000-0000-000000000001"

type AgentMixin struct {
	mixin.Schema
}

func (AgentMixin) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("agent_id", uuid.UUID{}),
	}
}

func (AgentMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_id"),
	}
}
