package model

import (
	// 	"context"
	// 	"fmt"
	"fmt"
	"time"

	"github.com/furisto/construct/backend/tool/native"
	"github.com/google/uuid"
	"github.com/openai/openai-go/shared"
	// "github.com/furisto/construct/backend/tool/native"
	// "github.com/google/uuid"
	// "github.com/openai/openai-go"
	// "github.com/openai/openai-go/option"
	// "github.com/openai/openai-go/responses"
	// "github.com/openai/openai-go/shared"
)

type OpenAIModelProfile struct {
	// API Configuration
	APIURL       string        `json:"api_url,omitempty"`
	Organization string        `json:"organization,omitempty"`
	APIVersion   string        `json:"api_version,omitempty"`
	Timeout      time.Duration `json:"timeout,omitempty"`
	MaxRetries   int           `json:"max_retries,omitempty"`

	// Default Model Parameters
	Temperature      float64 `json:"temperature,omitempty"`
	MaxTokens        int64   `json:"max_tokens,omitempty"`
	TopP             float32 `json:"top_p,omitempty"`
	FrequencyPenalty float32 `json:"frequency_penalty,omitempty"`
	PresencePenalty  float32 `json:"presence_penalty,omitempty"`

	// Feature Flags
	EnableJSONMode        bool     `json:"enable_json_mode,omitempty"`
	EnableFunctionCalling bool     `json:"enable_function_calling,omitempty"`
	ParallelToolCalls     bool     `json:"parallel_tool_calls,omitempty"`
	EnableVision          bool     `json:"enable_vision,omitempty"`
	StopSequences         []string `json:"stop_sequences,omitempty"`

	// Rate Limiting
	RequestsPerMinute int `json:"requests_per_minute,omitempty"`
	TokensPerMinute   int `json:"tokens_per_minute,omitempty"`
}

var _ ModelProfile = (*OpenAIModelProfile)(nil)

func (c *OpenAIModelProfile) Kind() ProviderKind {
	return ProviderKindOpenAI
}

func (c *OpenAIModelProfile) Validate() error {
	if c.APIURL == "" {
		c.APIURL = "https://api.openai.com/v1"
	}

	// Validate temperature range
	if c.Temperature < 0 || c.Temperature > 2.0 {
		return fmt.Errorf("OpenAI temperature must be between 0 and 2.0")
	}

	// Validate penalties
	if c.FrequencyPenalty < -2.0 || c.FrequencyPenalty > 2.0 {
		return fmt.Errorf("frequency_penalty must be between -2.0 and 2.0")
	}

	if c.PresencePenalty < -2.0 || c.PresencePenalty > 2.0 {
		return fmt.Errorf("presence_penalty must be between -2.0 and 2.0")
	}

	// Set defaults
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}

	if c.MaxRetries == 0 {
		c.MaxRetries = 3
	}

	return nil
}

func SupportedOpenAIModels() []Model {
	return []Model{
		{
			ID:            uuid.MustParse("01960000-0001-7000-8000-000000000001"),
			Name:          shared.ChatModelChatgpt4oLatest,
			Provider:      ProviderKindOpenAI,
			Capabilities:  []Capability{CapabilityImage},
			ContextWindow: 128000,
			Pricing: ModelPricing{
				Input:      2.5,
				Output:     10.0,
				CacheWrite: 1.25,
				CacheRead:  0.25,
			},
		},
		{
			ID:            uuid.MustParse("01960000-0002-7000-8000-000000000002"),
			Name:          shared.ChatModelO4Mini,
			Provider:      ProviderKindOpenAI,
			Capabilities:  []Capability{CapabilityImage},
			ContextWindow: 128000,
			Pricing: ModelPricing{
				Input:      0.15,
				Output:     0.6,
				CacheWrite: 0.075,
				CacheRead:  0.015,
			},
		},
		{
			ID:            uuid.MustParse("01960000-0003-7000-8000-000000000003"),
			Name:          "gpt-4-turbo",
			Provider:      ProviderKindOpenAI,
			Capabilities:  []Capability{CapabilityImage},
			ContextWindow: 128000,
			Pricing: ModelPricing{
				Input:      10.0,
				Output:     30.0,
				CacheWrite: 5.0,
				CacheRead:  1.0,
			},
		},
		{
			ID:            uuid.MustParse("01960000-0004-7000-8000-000000000004"),
			Name:          "gpt-3.5-turbo",
			Provider:      ProviderKindOpenAI,
			Capabilities:  []Capability{},
			ContextWindow: 16385,
			Pricing: ModelPricing{
				Input:      0.5,
				Output:     1.5,
				CacheWrite: 0.25,
				CacheRead:  0.05,
			},
		},
		{
			ID:            uuid.MustParse("01960000-0005-7000-8000-000000000005"),
			Name:          "o1",
			Provider:      ProviderKindOpenAI,
			Capabilities:  []Capability{},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      15.0,
				Output:     60.0,
				CacheWrite: 7.5,
				CacheRead:  1.5,
			},
		},
		{
			ID:            uuid.MustParse("01960000-0006-7000-8000-000000000006"),
			Name:          "o1-mini",
			Provider:      ProviderKindOpenAI,
			Capabilities:  []Capability{},
			ContextWindow: 128000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     12.0,
				CacheWrite: 1.5,
				CacheRead:  0.3,
			},
		},
		{
			ID:       uuid.MustParse("01960000-0007-7000-8000-000000000007"),
			Name:     "gpt-5-2025-08-07",
			Provider: ProviderKindOpenAI,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
				CapabilityExtendedThinking,
			},
			ContextWindow: 128000,
			Pricing: ModelPricing{
				Input:      1.25,
				Output:     10.0,
				CacheWrite: 1.25,
				CacheRead:  0.25,
			},
		},
	}
}

// type OpenAIProvider struct {
// 	client openai.Client
// }

// func NewOpenAIProvider(apiKey string) (*OpenAIProvider, error) {
// 	if apiKey == "" {
// 		return nil, fmt.Errorf("openai API key is required")
// 	}

// 	provider := &OpenAIProvider{
// 		client: openai.NewClient(option.WithAPIKey(apiKey)),
// 	}

// 	return provider, nil
// }

// func (p *OpenAIProvider) InvokeModel(ctx context.Context, model, systemPrompt string, messages []*Message, opts ...InvokeModelOption) (*Message, error) {
// 	if model == "" {
// 		return nil, fmt.Errorf("model is required")
// 	}

// 	if systemPrompt == "" {
// 		return nil, fmt.Errorf("system prompt is required")
// 	}

// 	if len(messages) == 0 {
// 		return nil, fmt.Errorf("at least one message is required")
// 	}

// 	options := DefaultOpenAIModelOptions()
// 	for _, opt := range opts {
// 		opt(options)
// 	}

// 	modelProfile, err := ensureModelProfile(options.ModelProfile)
// 	if err != nil {
// 		return nil, err
// 	}

// 	openaiMessages, err := transformMessages(messages)
// 	if err != nil {
// 		return nil, err
// 	}

// 	tools, err := transformTools(options.Tools)
// 	if err != nil {
// 		return nil, err
// 	}

// 	params := responses.ResponseNewParams{
// 		Model:             model,
// 		Instructions:      openai.String(systemPrompt),
// 		MaxOutputTokens:   openai.Int(int64(modelProfile.MaxTokens)),
// 		Temperature:       openai.Float(modelProfile.Temperature),
// 		Tools:             tools,
// 		ParallelToolCalls: openai.Bool(true),
// 		Input: responses.ResponseNewParamsInputUnion{
// 			OfInputItemList: openaiMessages,
// 		},
// 	}

// 	stream := p.client.Responses.NewStreaming(ctx, params)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create response: %w", err)
// 	}

// 	for stream.Next() {
// 		event := stream.Current()

// 		switch event.Type {

// 		}

// 	}

// 	// var contentBlocks []ContentBlock
// 	// for _, output := range stream. {
// 	// 	switch output.Type {
// 	// 	case "message":

// 	// 		contentBlocks = append(contentBlocks, &TextBlock{Text: output.AsMessage().Content})
// 	// 	case "function_call":
// 	// 		funcCall := output.AsFunctionCall()
// 	// 		contentBlocks = append(contentBlocks, &ToolCallBlock{
// 	// 			ID:   funcCall.CallID,
// 	// 			Tool: funcCall.Name,
// 	// 			Args: []byte(funcCall.Arguments),
// 	// 		})
// 	// 	}
// 	// }

// 	// // Calculate usage from response
// 	// var usage Usage
// 	// if resp.Usage.JSON.Valid() {
// 	// 	usage = Usage{
// 	// 		InputTokens:  int(resp.Usage.InputTokens),
// 	// 		OutputTokens: int(resp.Usage.OutputTokens),
// 	// 		// OpenAI Responses API doesn't provide cache token info the same way
// 	// 		CacheWriteTokens: 0,
// 	// 		CacheReadTokens:  0,
// 	// 	}
// 	// }

// 	// // Handle streaming if requested
// 	// if options.StreamHandler != nil {
// 	// 	// For streaming, send the final result
// 	// 	streamMessage := &Message{
// 	// 		Source:  MessageSourceModel,
// 	// 		Content: contentBlocks,
// 	// 	}
// 	// 	options.StreamHandler(ctx, streamMessage)
// 	// }

// 	// return NewModelMessage(contentBlocks, usage), nil
// 	return nil, nil
// }

// func transformMessages(messages []*Message) ([]responses.ResponseInputItemUnionParam, error) {

// 	// Convert internal messages to OpenAI Responses API format
// 	inputItems := make([]responses.ResponseInputItemUnionParam, 0, len(messages))

// 	inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
// 		OfMessage: &responses.EasyInputMessageParam{
// 			Role: responses.EasyInputMessageRoleSystem,
// 		},
// 	})
// 	// Add user messages and tool results
// 	for _, message := range messages {
// 		for _, block := range message.Content {
// 			switch block := block.(type) {
// 			case *TextBlock:
// 				if message.Source == MessageSourceUser {
// 					// Create text input item
// 					inputItems = append(inputItems, responses.ResponseInputItemParam{
// 						OfMessage: &responses.ResponseInputMessageParam{
// 							Role: "user",
// 							Content: []responses.ResponseInputContentUnionParam{
// 								{
// 									OfInputText: &responses.ResponseInputTextParam{
// 										Type: "input_text",
// 										Text: block.Text,
// 									},
// 								},
// 							},
// 						},
// 					})
// 				}
// 				// Skip model messages as they're handled by previous response ID
// 			case *ToolResultBlock:
// 				// Add tool result
// 				inputItems = append(inputItems, responses.ResponseInputItemParam{
// 					OfFunctionCallOutput: &responses.ResponseInputFunctionCallOutputParam{
// 						Type:   "function_call_output",
// 						CallID: block.ID,
// 						Output: block.Result,
// 					},
// 				})
// 			}
// 		}
// 	}

// 	// Convert tools to OpenAI format
// 	var tools []responses.ToolUnionParam
// 	for _, tool := range options.Tools {
// 		toolParam := responses.ToolUnionParam{
// 			OfFunction: &responses.FunctionToolParam{
// 				Type:        "function",
// 				Name:        tool.Name(),
// 				Description: openai.String(tool.Description()),
// 				Parameters:  tool.Schema(),
// 			},
// 		}
// 		tools = append(tools, toolParam)
// 	}
// }

// func transformTools(tools []native.Tool) ([]responses.ToolUnionParam, error) {
// 	var openaiTools []responses.ToolUnionParam
// 	for _, tool := range tools {
// 		openaiTools = append(openaiTools, responses.ToolUnionParam{
// 			OfFunction: &responses.FunctionToolParam{
// 				Name:        tool.Name(),
// 				Description: openai.String(tool.Description()),
// 				Parameters:  tool.Schema(),
// 			},
// 		})
// 	}

// 	return openaiTools, nil
// }

func DefaultOpenAIModelOptions() *InvokeModelOptions {
	return &InvokeModelOptions{
		Tools:       []native.Tool{},
		ModelProfile: &OpenAIModelProfile{
			APIURL:                "",
			Organization:          "",
			APIVersion:            "",
			MaxTokens:             8192,
			EnableFunctionCalling: true,
			ParallelToolCalls:     true,
		},
		StreamCallback: nil,
	}
}

// func (p *OpenAIProvider) GetModel(ctx context.Context, modelID uuid.UUID) (Model, error) {
// 	for _, model := range SupportedOpenAIModels() {
// 		if model.ID == modelID {
// 			return model, nil
// 		}
// 	}

// 	return Model{}, fmt.Errorf("model not supported")
// }
