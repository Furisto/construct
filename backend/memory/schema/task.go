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
		field.String("project_directory").Optional(),
		field.Int64("input_tokens").Optional(),
		field.Int64("output_tokens").Optional(),
		field.Int64("cache_write_tokens").Optional(),
		field.Int64("cache_read_tokens").Optional(),
		field.Float("cost").Optional(),
		field.Int64("turns").Default(0),

		field.UUID("agent_id", uuid.UUID{}).Optional(),
	}
}

func (Task) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("messages", Message.Type).Ref("task"),
		edge.To("agent", Agent.Type).Field("agent_id").Unique(),
	}
}

func (Task) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
