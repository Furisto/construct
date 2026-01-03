package test

import (
	"context"
	"testing"
	"time"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
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

func NewModelProviderBuilder(t *testing.T, id uuid.UUID, db *memory.Client) *ModelProviderBuilder {
	return &ModelProviderBuilder{
		entityBuilder:   newEntityBuilder(t, db),
		modelProviderID: id,
		providerType:    types.ModelProviderTypeAnthropic,
		name:            "anthropic",
		secret:          []byte("mock-secret"),
		enabled:         true,
	}
}

func (b *ModelProviderBuilder) WithID(id uuid.UUID) *ModelProviderBuilder {
	b.modelProviderID = id
	return b
}

func (b *ModelProviderBuilder) WithName(name string) *ModelProviderBuilder {
	b.name = name
	return b
}

func (b *ModelProviderBuilder) WithProviderType(providerType types.ModelProviderType) *ModelProviderBuilder {
	b.providerType = providerType
	return b
}

func (b *ModelProviderBuilder) WithEnabled(enabled bool) *ModelProviderBuilder {
	b.enabled = enabled
	return b
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
	alias           string

	contextWindow  int64
	capabilities   []types.ModelCapability
	inputCost      float64
	outputCost     float64
	cacheReadCost  float64
	cacheWriteCost float64
	enabled        bool
}

func NewModelBuilder(t *testing.T, id uuid.UUID, db *memory.Client, modelProvider *memory.ModelProvider) *ModelBuilder {
	if modelProvider == nil {
		t.Fatal("model provider is required")
	}

	return &ModelBuilder{
		entityBuilder:   newEntityBuilder(t, db),
		modelProviderID: modelProvider.ID,
		modelID:         id,
		name:            "claude-3-7-sonnet-20250219",
		alias:           "claude-3-7-sonnet",
		contextWindow:   200_000,
		capabilities:    []types.ModelCapability{types.ModelCapabilityPromptCache},
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

func (b *ModelBuilder) WithName(name string) *ModelBuilder {
	b.name = name
	return b
}

func (b *ModelBuilder) Build(ctx context.Context) *memory.Model {
	model, err := b.db.Model.Create().
		SetID(b.modelID).
		SetModelProviderID(b.modelProviderID).
		SetName(b.name).
		SetContextWindow(b.contextWindow).
		SetCapabilities(b.capabilities).
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

func NewAgentBuilder(t *testing.T, id uuid.UUID, db *memory.Client, defaultModel *memory.Model) *AgentBuilder {
	if defaultModel == nil {
		t.Fatal("model is required")
	}

	return &AgentBuilder{
		entityBuilder: newEntityBuilder(t, db),
		agentID:       id,
		name:          "coder",
		description:   "Writes code",
		defaultModel:  defaultModel.ID,
		instructions:  "Implement the plan",
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
		SetModelID(b.defaultModel).
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

func NewTaskBuilder(t *testing.T, id uuid.UUID, db *memory.Client, agent *memory.Agent) *TaskBuilder {
	return &TaskBuilder{
		entityBuilder: newEntityBuilder(t, db),
		taskID:        id,
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
	source  types.MessageSource
	content *types.MessageContent
}

func NewMessageBuilder(t *testing.T, id uuid.UUID, db *memory.Client, task *memory.Task) *MessageBuilder {
	if task == nil {
		t.Fatal("task is required")
	}

	return &MessageBuilder{
		entityBuilder: newEntityBuilder(t, db),
		messageID:     id,
		taskID:        task.ID,
		source:        types.MessageSourceUser,
		content: &types.MessageContent{Blocks: []types.MessageBlock{
			{
				Kind:    types.MessageBlockKindText,
				Payload: "test message",
			},
		}},
	}
}

func (b *MessageBuilder) WithAgent(agent *memory.Agent) *MessageBuilder {
	b.agentID = agent.ID
	b.modelID = agent.ModelID
	b.source = types.MessageSourceAssistant
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
	create := b.db.Message.Create().
		SetID(b.messageID).
		SetTaskID(b.taskID).
		SetContent(b.content).
		SetSource(b.source)

	if b.agentID != uuid.Nil {
		create.SetAgentID(b.agentID)
	}

	if b.modelID != uuid.Nil {
		create.SetModelID(b.modelID)
	}

	message, err := create.Save(ctx)

	if err != nil {
		b.t.Fatalf("failed to create message: %v", err)
	}

	return message
}

type TokenBuilder struct {
	*entityBuilder
	tokenID uuid.UUID

	name      string
	expiresAt time.Time
	tokenHash []byte
}

func NewTokenBuilder(t *testing.T, id uuid.UUID, db *memory.Client) *TokenBuilder {
	return &TokenBuilder{
		entityBuilder: newEntityBuilder(t, db),
		tokenID:       id,
		name:          "test-token",
		expiresAt:     time.Now().Add(90 * 24 * time.Hour),
		tokenHash:     []byte("test-hash-" + id.String()),
	}
}

func (b *TokenBuilder) WithName(name string) *TokenBuilder {
	b.name = name
	return b
}

func (b *TokenBuilder) WithExpiresAt(expiresAt time.Time) *TokenBuilder {
	b.expiresAt = expiresAt
	return b
}

func (b *TokenBuilder) Build(ctx context.Context) *memory.Token {
	token, err := b.db.Token.Create().
		SetID(b.tokenID).
		SetName(b.name).
		SetType(types.TokenTypeAPIToken).
		SetTokenHash(b.tokenHash).
		SetExpiresAt(b.expiresAt).
		Save(ctx)

	if err != nil {
		b.t.Fatalf("failed to create token: %v", err)
	}

	return token
}
