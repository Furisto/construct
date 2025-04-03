package test

import (
	"context"
	"testing"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

var (
	ModelProviderID = uuid.MustParse("0195fc02-59ef-7194-93d5-387400b068cb")
	ModelID         = uuid.MustParse("0195fbbe-adda-76cf-be67-9f1b64b50a4a")
	AgentID         = uuid.MustParse("0195fbbe-42e1-75fe-8e08-28758035ff95")
	TaskID          = uuid.MustParse("0195fbbe-0be8-74b1-af7a-6e76e80e2462")
	MessageID       = uuid.MustParse("0195fbbd-757d-7db6-83c2-f556128b4586")
)

type entityBuilder struct {
	db *memory.Client
	t  *testing.T
}

func newEntityBuilder(t *testing.T, db *memory.Client) *entityBuilder {
	if t == nil {
		panic("testing.T is required")
	}

	if db == nil {
		t.Fatal("memory client is required")
	}

	return &entityBuilder{
		t:  t,
		db: db,
	}
}

type ModelProviderBuilder struct {
	*entityBuilder
	modelProviderID uuid.UUID

	providerType types.ModelProviderType
	name         string
	secret       []byte
	enabled      bool
}

func NewModelProviderBuilder(t *testing.T, db *memory.Client) *ModelProviderBuilder {
	return &ModelProviderBuilder{
		entityBuilder:   newEntityBuilder(t, db),
		modelProviderID: ModelProviderID,
		providerType:    types.ModelProviderTypeAnthropic,
		name:            "test",
		secret:          []byte("test"),
		enabled:         true,
	}
}

func (b *ModelProviderBuilder) Build(ctx context.Context) *memory.ModelProvider {
	modelProvider, err := b.db.ModelProvider.Create().
		SetID(b.modelProviderID).
		SetName(b.name).
		SetProviderType(b.providerType).
		SetSecret(b.secret).
		SetEnabled(b.enabled).
		Save(ctx)

	if err != nil {
		b.t.Fatalf("failed to create model provider: %v", err)
	}

	return modelProvider
}

func (b *Builder) Model(ctx context.Context, modelProvider *memory.ModelProvider, opts ...BuilderOption) *memory.Model {
	options := DefaultBuilderOptions()
	for _, opt := range opts {
		opt(options)
	}

	model, err := b.db.Model.Create().
		SetID(options.ModelID).
		SetName("test").
		SetModelProviderID(modelProvider.ID).
		Save(ctx)

	if err != nil {
		b.t.Fatalf("failed to create model: %v", err)
	}

	return model
}

func (b *Builder) Agent(ctx context.Context, model *memory.Model, opts ...BuilderOption) *memory.Agent {
	options := DefaultBuilderOptions()
	for _, opt := range opts {
		opt(options)
	}

	agent, err := b.db.Agent.Create().
		SetID(options.AgentID).
		SetName("test").
		SetModelID(model.ID).
		Save(ctx)

	if err != nil {
		b.t.Fatalf("failed to create agent: %v", err)
	}

	return agent
}

type TaskBuilder struct {
	*entityBuilder
	taskID uuid.UUID

	agentID uuid.UUID
}

func NewTaskBuilder(t *testing.T, db *memory.Client, agent *memory.Agent) *TaskBuilder {
	return &TaskBuilder{
		entityBuilder: newEntityBuilder(t, db),
		taskID:        TaskID,
		agentID:       agent.ID,
	}
}

func (b *TaskBuilder) Build(ctx context.Context) *memory.Task {
	task, err := b.db.Task.Create().
		SetID(b.taskID).
		SetAgentID(b.agentID).
		Save(ctx)

	if err != nil {
		b.t.Fatalf("failed to create task: %v", err)
	}

	return task
}

type MessageBuilder struct {
	*entityBuilder
	messageID uuid.UUID

	taskID uuid.UUID

	agentID uuid.UUID
	modelID uuid.UUID
	role    types.MessageRole
	content *types.MessageContent
}

func NewMessageBuilder(t *testing.T, db *memory.Client, task *memory.Task) *MessageBuilder {
	if task == nil {
		t.Fatal("task is required")
	}

	return &MessageBuilder{
		entityBuilder: newEntityBuilder(t, db),
		messageID:     MessageID,
		taskID:        task.ID,
		role:          types.MessageRoleUser,
		content: &types.MessageContent{Blocks: []types.MessageContentBlock{
			{
				Type: types.MessageContentBlockTypeText,
				Text: "test message",
			},
		}},
	}
}

func (b *MessageBuilder) WithAgent(agent *memory.Agent) *MessageBuilder {
	b.agentID = agent.ID
	b.modelID = agent.DefaultModel
	b.role = types.MessageRoleAssistant
	return b
}

func (b *MessageBuilder) WithContent(content *types.MessageContent) *MessageBuilder {
	b.content = content
	return b
}

func (b *MessageBuilder) Build(ctx context.Context) *memory.Message {
	message, err := b.db.Message.Create().
		SetID(b.messageID).
		SetTaskID(b.taskID).
		SetAgentID(b.agentID).
		SetModelID(b.modelID).
		SetContent(b.content).
		SetRole(b.role).
		Save(ctx)

	if err != nil {
		b.t.Fatalf("failed to create message: %v", err)
	}

	return message
}
