package model

import "github.com/google/uuid"

type Model struct {
	ID            uuid.UUID
	Provider      Provider
	Name          string
	Capabilities  []Capability
	ContextWindow int64
	Pricing       ModelPricing
}

type Provider string

const (
	Anthropic Provider = "anthropic"
	OpenAI    Provider = "openai"
	DeepSeek  Provider = "deepseek"
	Gemini    Provider = "gemini"
	XAI       Provider = "xai"
	Bedrock   Provider = "bedrock"
)

type Capability string

const (
	CapabilityImage            Capability = "image"
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

func SupportedModels(provider Provider) []Model {
	switch provider {
	case Anthropic:
		return SupportedAnthropicModels()
	}
	return nil
}
