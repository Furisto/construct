package model

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

func SupportedOpenAIModels() []Model {
	return []Model{
		{
			ID:            uuid.MustParse("01960000-0001-7000-8000-000000000001"),
			Name:          shared.ChatModelChatgpt4oLatest,
			Provider:      OpenAI,
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
			Provider:      OpenAI,
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
			Provider:      OpenAI,
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
			Provider:      OpenAI,
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
			Provider:      OpenAI,
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
			Provider:      OpenAI,
			Capabilities:  []Capability{},
			ContextWindow: 128000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     12.0,
				CacheWrite: 1.5,
				CacheRead:  0.3,
			},
		},
	}
}

type OpenAIProvider struct {
	client openai.Client
}

func NewOpenAIProvider(apiKey string) (*OpenAIProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai API key is required")
	}

	provider := &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
	}

	return provider, nil
}

func (p *OpenAIProvider) InvokeModel(ctx context.Context, model, systemPrompt string, messages []*Message, opts ...InvokeModelOption) (*Message, error) {
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	if systemPrompt == "" {
		return nil, fmt.Errorf("system prompt is required")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}

	options := DefaultInvokeModelOptions()
	for _, opt := range opts {
		opt(options)
	}

	// // Convert internal messages to OpenAI Responses API format
	// inputItems := make([]responses.ResponseInputItemUnionParam, 0, len(messages)+1)

	// inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
	// 	OfMessage: &responses.EasyInputMessageParam{
	// 		Role: responses.EasyInputMessageRoleSystem,
	// 	},
	// })

	// // Add user messages and tool results
	// for _, message := range messages {
	// 	for _, block := range message.Content {
	// 		switch block := block.(type) {
	// 		case *TextBlock:
	// 			if message.Source == MessageSourceUser {
	// 				// Create text input item
	// 				inputItems = append(inputItems, responses.ResponseInputItemParam{
	// 					OfMessage: &responses.ResponseInputMessageParam{
	// 						Role: "user",
	// 						Content: []responses.ResponseInputContentUnionParam{
	// 							{
	// 								OfInputText: &responses.ResponseInputTextParam{
	// 									Type: "input_text",
	// 									Text: block.Text,
	// 								},
	// 							},
	// 						},
	// 					},
	// 				})
	// 			}
	// 			// Skip model messages as they're handled by previous response ID
	// 		case *ToolResultBlock:
	// 			// Add tool result
	// 			inputItems = append(inputItems, responses.ResponseInputItemParam{
	// 				OfFunctionCallOutput: &responses.ResponseInputFunctionCallOutputParam{
	// 					Type:   "function_call_output",
	// 					CallID: block.ID,
	// 					Output: block.Result,
	// 				},
	// 			})
	// 		}
	// 	}
	// }

	// // Convert tools to OpenAI format
	// var tools []responses.ToolUnionParam
	// for _, tool := range options.Tools {
	// 	toolParam := responses.ToolUnionParam{
	// 		OfFunction: &responses.FunctionToolParam{
	// 			Type:        "function",
	// 			Name:        tool.Name(),
	// 			Description: openai.String(tool.Description()),
	// 			Parameters:  tool.Schema(),
	// 		},
	// 	}
	// 	tools = append(tools, toolParam)
	// }

	// // Create request parameters
	// params := responses.ResponseNewParams{
	// 	Model:           shared.ChatModelChatgpt4oLatest,
	// 	Instructions:    openai.String(systemPrompt),
	// 	MaxOutputTokens: openai.Int(int64(options.MaxTokens)),
	// 	Temperature:     openai.Float(options.Temperature),
	// 	Input: responses.ResponseNewParamsInputUnion{
	// 		OfInputItemList: inputItems,
	// 	},
	// }

	// // Map model string to correct enum
	// switch model {
	// case "gpt-4o":
	// 	params.Model = shared.ResponsesModelGPT4o
	// case "gpt-4o-mini":
	// 	params.Model = shared.ResponsesModelGPT4oMini
	// case "gpt-4-turbo":
	// 	params.Model = shared.ResponsesModelGPT4Turbo
	// case "gpt-3.5-turbo":
	// 	params.Model = shared.ResponsesModelGPT3_5Turbo
	// case "o1":
	// 	params.Model = shared.ResponsesModelO1
	// case "o1-mini":
	// 	params.Model = shared.ResponsesModelO1Mini
	// default:
	// 	return nil, fmt.Errorf("unsupported model: %s", model)
	// }

	// if len(tools) > 0 {
	// 	params.Tools = tools
	// }

	// // For streaming, we'd need to handle the streaming response differently
	// // For now, let's implement non-streaming version
	// resp, err := p.client.Responses.New(ctx, params)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create response: %w", err)
	// }

	// // Process the response and extract content blocks
	// var contentBlocks []ContentBlock

	// // Extract text from output
	// if outputText := resp.OutputText(); outputText != "" {
	// 	contentBlocks = append(contentBlocks, &TextBlock{Text: outputText})
	// }

	// // Extract tool calls from output
	// for _, output := range resp.Output {
	// 	if output.Type == "function_call" {
	// 		funcCall := output.AsFunctionCall()
	// 		contentBlocks = append(contentBlocks, &ToolCallBlock{
	// 			ID:   funcCall.CallID,
	// 			Tool: funcCall.Name,
	// 			Args: []byte(funcCall.Arguments),
	// 		})
	// 	}
	// }

	// // Calculate usage from response
	// var usage Usage
	// if resp.Usage.JSON.Valid() {
	// 	usage = Usage{
	// 		InputTokens:  int(resp.Usage.InputTokens),
	// 		OutputTokens: int(resp.Usage.OutputTokens),
	// 		// OpenAI Responses API doesn't provide cache token info the same way
	// 		CacheWriteTokens: 0,
	// 		CacheReadTokens:  0,
	// 	}
	// }

	// // Handle streaming if requested
	// if options.StreamHandler != nil {
	// 	// For streaming, send the final result
	// 	streamMessage := &Message{
	// 		Source:  MessageSourceModel,
	// 		Content: contentBlocks,
	// 	}
	// 	options.StreamHandler(ctx, streamMessage)
	// }

	// return NewModelMessage(contentBlocks, usage), nil
	return nil, nil
}

func (p *OpenAIProvider) GetModel(ctx context.Context, modelID uuid.UUID) (Model, error) {
	for _, model := range SupportedOpenAIModels() {
		if model.ID == modelID {
			return model, nil
		}
	}

	return Model{}, fmt.Errorf("model not supported")
}
