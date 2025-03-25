package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Task struct {
	ent.Schema
}

func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.NewV7).Unique().Immutable(),
		field.Int64("input_tokens"),
		field.Int64("output_tokens"),
		field.Int64("cache_write_tokens"),
		field.Int64("cache_read_tokens"),
		// field.UUID("last_processed_message", uuid.UUID{}).Optional(),
	}
}

func (Task) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("messages", Message.Type),
	}
}

func (Task) Mixin() []ent.Mixin {
	return []ent.Mixin{
		AgentMixin{},
	}
}
