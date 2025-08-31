package model

import (
	"fmt"

	"github.com/google/uuid"
)

type Model struct {
	ID            uuid.UUID
	Provider      ModelProfileKind
	Name          string
	Capabilities  []Capability
	ContextWindow int64
	Pricing       ModelPricing
}

type ModelProfileKind string

func ensureModelProfile[T ModelProfile](modelProfile ModelProfile) (T, error) {
	if modelProfile == nil {
		return *new(T), fmt.Errorf("no model profile provided")
	}

	p, ok := modelProfile.(T)
	if !ok {
		return *new(T), fmt.Errorf("model profile is not an %T profile", new(T))
	}

	err := p.Validate()
	if err != nil {
		return *new(T), fmt.Errorf("model profile is invalid: %w", err)
	}

	return p, nil
}

const (
	ProviderKindAnthropic ModelProfileKind = "anthropic"
	ProviderKindOpenAI    ModelProfileKind = "openai"
	ProviderKindDeepSeek  ModelProfileKind = "deepseek"
	ProviderKindGemini    ModelProfileKind = "gemini"
	ProviderKindXAI       ModelProfileKind = "xai"
	ProviderKindBedrock   ModelProfileKind = "bedrock"
)

type Capability string

const (
	CapabilityImage            Capability = "image"
	CapabilityAudio            Capability = "audio"
	CapabilityComputerUse      Capability = "computer_use"
	CapabilityPromptCache      Capability = "prompt_cache"
	CapabilityExtendedThinking Capability = "extended_thinking"
)

type ModelPricing struct {
	Input      float64
	Output     float64
	CacheWrite float64
	CacheRead  float64
}

func SupportedModels(provider ModelProfileKind) []Model {
	switch provider {
	case ProviderKindAnthropic:
		return SupportedAnthropicModels()
	case ProviderKindOpenAI:
		return SupportedOpenAIModels()
	case ProviderKindGemini:
		return SupportedGeminiModels()
	}

	return nil
}
