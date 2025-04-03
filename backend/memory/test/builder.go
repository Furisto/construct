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
		name:            "anthropic",
		secret:          []byte("mock-secret"),
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

type ModelBuilder struct {
	*entityBuilder
	modelID uuid.UUID

	modelProviderID uuid.UUID
	name            string

	contextWindow  int64
	inputCost      float64
	outputCost     float64
	cacheReadCost  float64
	cacheWriteCost float64
	enabled        bool
}

func NewModelBuilder(t *testing.T, db *memory.Client, modelProvider *memory.ModelProvider) *ModelBuilder {
	if modelProvider == nil {
		t.Fatal("model provider is required")
	}

	return &ModelBuilder{
		entityBuilder:   newEntityBuilder(t, db),
		modelProviderID: modelProvider.ID,
		modelID:         ModelID,
		name:            "claude-3-7-sonnet-20250219",
		contextWindow:   200_000,
		inputCost:       3,
		outputCost:      15,
		cacheWriteCost:  3.75,
		cacheReadCost:   0.3,
		enabled:         true,
	}
}

func (b *ModelBuilder) WithEnabled(enabled bool) *ModelBuilder {
	b.enabled = enabled
	return b
}

func (b *ModelBuilder) WithID(id uuid.UUID) *ModelBuilder {
	b.modelID = id
	return b
}

func (b *ModelBuilder) Build(ctx context.Context) *memory.Model {
	model, err := b.db.Model.Create().
		SetID(b.modelID).
		SetModelProviderID(b.modelProviderID).
		SetName(b.name).
		SetContextWindow(b.contextWindow).
		SetInputCost(b.inputCost).
		SetOutputCost(b.outputCost).
		SetCacheReadCost(b.cacheReadCost).
		SetCacheWriteCost(b.cacheWriteCost).
		SetEnabled(b.enabled).
		Save(ctx)

	if err != nil {
		b.t.Fatalf("failed to create model: %v", err)
	}

	return model
}

type AgentBuilder struct {
	*entityBuilder
	agentID uuid.UUID

	name         string
	description  string
	defaultModel uuid.UUID
	instructions string
}

func NewAgentBuilder(t *testing.T, db *memory.Client, defaultModel *memory.Model) *AgentBuilder {
	if defaultModel == nil {
		t.Fatal("model is required")
	}

	return &AgentBuilder{
		entityBuilder: newEntityBuilder(t, db),
		agentID:       AgentID,
		name:          "coder",
		description:   "Writes code",
		defaultModel:  defaultModel.ID,
		instructions:  "Implement the plan exactly as described",
	}
}

func (b *AgentBuilder) WithID(id uuid.UUID) *AgentBuilder {
	b.agentID = id
	return b
}

func (b *AgentBuilder) WithName(name string) *AgentBuilder {
	b.name = name
	return b
}

func (b *AgentBuilder) WithDescription(description string) *AgentBuilder {
	b.description = description
	return b
}

func (b *AgentBuilder) WithInstructions(instructions string) *AgentBuilder {
	b.instructions = instructions
	return b
}

func (b *AgentBuilder) Build(ctx context.Context) *memory.Agent {
	agent, err := b.db.Agent.Create().
		SetID(b.agentID).
		SetName(b.name).
		SetDescription(b.description).
		SetDefaultModel(b.defaultModel).
		SetInstructions(b.instructions).
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

func (b *TaskBuilder) WithID(id uuid.UUID) *TaskBuilder {
	b.taskID = id
	return b
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

func (b *MessageBuilder) WithID(id uuid.UUID) *MessageBuilder {
	b.messageID = id
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
