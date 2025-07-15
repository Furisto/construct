package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

type Model struct {
	ent.Schema
}

func (Model) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.String("name"),
		field.Int64("context_window"),
		field.JSON("capabilities", []types.ModelCapability{}).Optional(),
		field.Float("input_cost").Min(0).Default(0),
		field.Float("output_cost").Min(0).Default(0),
		field.Float("cache_write_cost").Min(0).Default(0),
		field.Float("cache_read_cost").Min(0).Default(0),
		field.Bool("enabled").Default(true),
		field.String("alias").Optional(),

		field.UUID("model_provider_id", uuid.UUID{}),
	}
}

func (Model) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agents", Agent.Type).Ref("model"),
		edge.From("model_provider", ModelProvider.Type).
			Ref("models").
			Field("model_provider_id").
			Unique().
			Required(),
		edge.From("messages", Message.Type).Ref("model"),
	}
}

func (Model) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
