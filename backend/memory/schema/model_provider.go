package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

type ModelProvider struct {
	ent.Schema
}

func (ModelProvider) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.String("name").NotEmpty(),
		field.Enum("provider_type").GoType(types.ModelProviderType("")),
		field.String("url").NotEmpty(),
		field.String("secret_ref").NotEmpty().Sensitive(),
		field.String("secret_store").NotEmpty(),
		field.Bool("enabled").Default(true),
	}
}

func (ModelProvider) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("models", Model.Type),
	}
}

func (ModelProvider) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
