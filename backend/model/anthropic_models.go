package model

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

func SupportedAnthropicModels() []Model {
	return []Model{
		{
			ID:       uuid.MustParse("0197e0d5-7567-70c6-8f64-e217dee9eb05"),
			Name:     "claude-sonnet-4-20250514",
			Provider: ProviderKindAnthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
				CapabilityExtendedThinking,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     15.0,
				CacheWrite: 3.75,
				CacheRead:  0.3,
			},
		},
		{
			ID:       uuid.MustParse("0197e0d5-8f08-7609-9fe0-d407b2563375"),
			Name:     "claude-opus-4-20250514",
			Provider: ProviderKindAnthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
				CapabilityExtendedThinking,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      15.0,
				Output:     75.0,
				CacheWrite: 18.75,
				CacheRead:  1.5,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-45b6-76df-b208-f48b7b0d5f51"),
			Name:     "claude-3-7-sonnet-20250219",
			Provider: ProviderKindAnthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
				CapabilityExtendedThinking,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     15.0,
				CacheWrite: 3.75,
				CacheRead:  0.3,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-7d71-79e0-97da-3045fb1ffc3e"),
			Name:     "claude-3-5-sonnet-20241022",
			Provider: ProviderKindAnthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     15.0,
				CacheWrite: 3.75,
				CacheRead:  0.3,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-a5df-736d-82ea-00f46db3dadc"),
			Name:     "claude-3-5-sonnet-20240620",
			Provider: ProviderKindAnthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
			},
			ContextWindow: 100000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     15.0,
				CacheWrite: 3.75,
				CacheRead:  0.3,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-c741-724d-bb2a-3b0f7fdbc5f4"),
			Name:     "claude-3-5-haiku-20241022",
			Provider: ProviderKindAnthropic,
			Capabilities: []Capability{
				CapabilityPromptCache,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      0.8,
				Output:     4.0,
				CacheWrite: 1.0,
				CacheRead:  0.08,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-efd4-7c5c-a9a2-219318e0e181"),
			Name:     "claude-3-opus-20240229",
			Provider: ProviderKindAnthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityPromptCache,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      15.0,
				Output:     75.0,
				CacheWrite: 18.75,
				CacheRead:  1.5,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e3-1da7-71af-ba34-6689aed6c4a2"),
			Name:     "claude-3-haiku-20240307",
			Provider: ProviderKindAnthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityPromptCache,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      0.25,
				Output:     1.25,
				CacheWrite: 0.3,
				CacheRead:  0.03,
			},
		},
	}
}

func DefaultAnthropicModel() *Model {
	models := SupportedAnthropicModels()
	return &models[0]
}

type AnthropicModelProfile struct {
	AnthropicVersion string        `json:"anthropic_version,omitempty"`
	AnthropicBeta    []string      `json:"anthropic_beta,omitempty"`
	Timeout          time.Duration `json:"timeout,omitempty"`
	MaxRetries       int           `json:"max_retries,omitempty"`

	Temperature   float64  `json:"temperature,omitempty"`
	MaxTokens     int64    `json:"max_tokens,omitempty"`
	DefaultTopP   float32  `json:"default_top_p,omitempty"`
	TopK          int      `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`

	EnablePromptCaching bool `json:"enable_prompt_caching,omitempty"`
	EnableThinkingMode  bool `json:"enable_thinking_mode,omitempty"`
	EnableAnalysisMode  bool `json:"enable_analysis_mode,omitempty"`
	EnableComputerUse   bool `json:"enable_computer_use,omitempty"`
}

var _ ModelProfile = (*AnthropicModelProfile)(nil)

func (c *AnthropicModelProfile) Kind() ProviderKind {
	return ProviderKindAnthropic
}

func (c *AnthropicModelProfile) Validate() error {
	if c.Temperature < 0 || c.Temperature > 1.0 {
		//lint:ignore ST1005 -- Anthropic should be capitalized
		return fmt.Errorf("Anthropic temperature must be between 0 and 1.0")
	}

	if c.TopK < 0 {
		return fmt.Errorf("top_k must be non-negative")
	}

	if c.Timeout == 0 {
		c.Timeout = 60 * time.Second
	}

	if c.AnthropicVersion == "" {
		c.AnthropicVersion = "2024-01-01"
	}

	if c.EnablePromptCaching && !slices.Contains(c.AnthropicBeta, "prompt-caching-2024-07-31") {
		c.AnthropicBeta = append(c.AnthropicBeta, "prompt-caching-2024-07-31")
	}

	if c.EnableComputerUse && !slices.Contains(c.AnthropicBeta, "computer-use-2024-10-22") {
		c.AnthropicBeta = append(c.AnthropicBeta, "computer-use-2024-10-22")
	}

	return nil
}
