package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/secret"
	"github.com/google/uuid"
)

type ModelProviderFactory struct {
	encryption *secret.Encryption
	memory     *memory.Client
}

func NewModelProviderFactory(encryption *secret.Encryption, memory *memory.Client) *ModelProviderFactory {
	return &ModelProviderFactory{
		encryption: encryption,
		memory:     memory,
	}
}

func (f *ModelProviderFactory) CreateClient(
	ctx context.Context,
	modelProviderID uuid.UUID,
) (providerClient model.ModelProvider, err error) {
	logger := slog.With(
		KeyComponent, "model_provider_factory",
		KeyModelProvider, modelProviderID,
	)

	provider, err := f.memory.ModelProvider.Get(ctx, modelProviderID)
	if err != nil {
		LogError(logger, "fetch model provider", err)
		return nil, fmt.Errorf("failed to fetch model provider: %w", err)
	}
	logger = logger.With(KeyProvider, string(provider.ProviderType))

	providerAuth, err := f.encryption.Decrypt(provider.Secret, []byte(secret.ModelProviderAssociated(provider.ID)))
	if err != nil {
		LogError(logger, "decrypt model provider secret", err)
		return nil, fmt.Errorf("failed to decrypt model provider secret: %w", err)
	}
	logger.Debug("model provider secret decrypted")

	var auth struct {
		APIKey string `json:"apiKey"`
	}

	err = json.Unmarshal(providerAuth, &auth)
	if err != nil {
		LogError(logger, "unmarshal model provider auth", err)
		return nil, fmt.Errorf("failed to unmarshal model provider auth: %w", err)
	}

	logger.Debug("creating model provider client")
	switch provider.ProviderType {
	case types.ModelProviderTypeAnthropic:
		providerClient, err = model.NewAnthropicProvider(auth.APIKey)

	case types.ModelProviderTypeOpenAI:
		providerClient, err = model.NewOpenAICompletionProvider(auth.APIKey)

	case types.ModelProviderTypeGemini:
		providerClient, err = model.NewGeminiProvider(auth.APIKey)

	case types.ModelProviderTypeXAI:
		providerClient, err = model.NewOpenAICompletionProvider(auth.APIKey, model.WithURL("https://api.xai.com/v1"))

	default:
		logger.Error("unknown model provider type",
			KeyProvider, string(provider.ProviderType),
		)
		return nil, fmt.Errorf("unknown model provider type: %s", provider.ProviderType)
	}

	if err != nil {
		LogError(logger, "create provider", err)
		return nil, fmt.Errorf("failed to create Anthropic provider: %w", err)
	}

	return providerClient, nil
}
