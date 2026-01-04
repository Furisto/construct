package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

type TaskSummary struct {
	ent.Schema
}

func (TaskSummary) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Unique().Immutable(),
		field.UUID("message_anchor", uuid.UUID{}),
		field.JSON("summary", &types.TaskSummary{}),
		field.Int64("token_budget").Default(0),
		field.UUID("task_id", uuid.UUID{}),
	}
}

func (TaskSummary) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).Field("task_id").Unique().Required().Annotations(
			entsql.Annotation{
				OnDelete: entsql.Cascade,
			},
		),
		edge.To("message", Message.Type).Field("message_anchor").Unique().Required(),
	}
}

func (TaskSummary) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id").Unique(),
		index.Fields("create_time"),
	}
}

func (TaskSummary) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
