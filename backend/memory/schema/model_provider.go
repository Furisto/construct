package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

type ModelProvider struct {
	ent.Schema
}

func (ModelProvider) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.String("name").NotEmpty(),
	}
}

func (ModelProvider) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
