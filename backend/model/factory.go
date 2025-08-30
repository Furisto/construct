package model

// import "time"

type ModelProfile interface {
	Validate() error
	Kind() ModelProfileKind
}

// type ProviderFactory struct {
// 	configs map[ModelProfileKind]ModelProfile
// }

// func DefaultProviderConfig(kind ModelProfileKind) ModelProfile {
// 	switch kind {
// 	case ProviderKindAnthropic:
// 		return &AnthropicModelProfile{
// 			APIKey: "anthropic-api-key",
// 		}
// 	case ProviderKindOpenAI:
// 		return &OpenAIModelProfile{
// 			APIKey: "openai-api-key",
// 		}
// 	}

// 	return nil
// }

// func NewModelProviderFactory() *ProviderFactory {
// 	return &ProviderFactory{
// 		configs: map[ModelProfileKind]ModelProfile{
// 			ProviderKindAnthropic: &AnthropicModelProfile{
// 				BaseURL:            "https://api.anthropic.com",
// 				DefaultTemperature: 0.5,
// 				DefaultTopK:        100,
// 				Timeout:            60 * time.Second,
// 				AnthropicVersion:   "2024-01-01",
// 				AnthropicBeta: []string{
// 					"prompt-caching-2024-07-31",
// 					"computer-use-2024-10-22",
// 				},
// 			},
// 			ProviderKindOpenAI: &OpenAIModelProfile{
// 				APIURL:                  "https://api.openai.com/v1",
// 				DefaultTemperature:      0.5,
// 				DefaultMaxTokens:        1000,
// 				DefaultTopP:             1.0,
// 				DefaultFrequencyPenalty: 0.0,
// 				DefaultPresencePenalty:  0.0,
// 				EnableJSONMode:          true,
// 				EnableFunctionCalling:   true,
// 				EnableVision:            true,
// 				DefaultStopSequences: []string{
// 					"\n\n",
// 				},
// 				RequestsPerMinute: 1000,
// 				TokensPerMinute:   1000000,
// 				Timeout:           60 * time.Second,
// 				MaxRetries:        3,
// 				Organization:      "org-1234567890",
// 				APIVersion:        "2024-01-01",
// 			},
// 		},
// 	}
// }
