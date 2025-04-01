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
		field.String("url").Optional(),
		field.Bytes("secret").NotEmpty().Sensitive(),
		field.Bool("enabled").Default(true),
	}
}

func (ModelProvider) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("models", Model.Type).Ref("model_providers"),
	}
}

func (ModelProvider) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
