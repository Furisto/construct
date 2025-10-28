package agent

import (
	"context"
	"encoding/json"
	"fmt"

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
) (model.ModelProvider, error) {
	provider, err := f.memory.ModelProvider.Get(ctx, modelProviderID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch model provider: %w", err)
	}

	providerAuth, err := f.encryption.Decrypt(provider.Secret, []byte(secret.ModelProviderAssociated(provider.ID)))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt model provider secret: %w", err)
	}

	var auth struct {
		APIKey string `json:"apiKey"`
	}
	err = json.Unmarshal(providerAuth, &auth)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal model provider auth: %w", err)
	}

	switch provider.ProviderType {
	case types.ModelProviderTypeAnthropic:
		apiProvider, err := model.NewAnthropicProvider(auth.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create Anthropic provider: %w", err)
		}
		return apiProvider, nil

	case types.ModelProviderTypeOpenAI:
		apiProvider, err := model.NewOpenAICompletionProvider(auth.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI provider: %w", err)
		}
		return apiProvider, nil

	case types.ModelProviderTypeGemini:
		apiProvider, err := model.NewGeminiProvider(auth.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini provider: %w", err)
		}
		return apiProvider, nil

	case types.ModelProviderTypeXAI:
		apiProvider, err := model.NewOpenAICompletionProvider(auth.APIKey, model.WithURL("https://api.xai.com/v1"))
		if err != nil {
			return nil, fmt.Errorf("failed to create XAI provider: %w", err)
		}
		return apiProvider, nil

	default:
		return nil, fmt.Errorf("unknown model provider type: %s", provider.ProviderType)
	}
}
