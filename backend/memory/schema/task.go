package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

type Task struct {
	ent.Schema
}

func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.Int64("input_tokens").Optional(),
		field.Int64("output_tokens").Optional(),
		field.Int64("cache_write_tokens").Optional(),
		field.Int64("cache_read_tokens").Optional(),
		field.Float("cost").Optional(),
	}
}

func (Task) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("messages", Message.Type),
		edge.From("agent", Agent.Type).
			Ref("tasks").
			Unique(),
	}
}

func (Task) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
