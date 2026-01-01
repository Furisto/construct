package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

type Token struct {
	ent.Schema
}

func (Token) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.String("name").NotEmpty(),
		field.Enum("type").GoType(types.TokenType("")).Default(string(types.TokenTypeAPIToken)),
		field.Bytes("token_hash"),
		field.String("description").Optional(),
		field.Time("expires_at"),
	}
}

func (Token) Edges() []ent.Edge {
	return []ent.Edge{}
}

func (Token) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("token_hash").Unique(),
	}
}

func (Token) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
