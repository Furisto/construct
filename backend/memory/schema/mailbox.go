package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

type Mailbox struct {
	ent.Schema
}

func (Mailbox) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.JSON("content", &types.MessageContent{}),
	}
}
