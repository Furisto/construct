package model

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/furisto/construct/backend/tool/native"
	"github.com/google/uuid"
	"google.golang.org/genai"
)

type GeminiProvider struct {
	client *genai.Client
}

type GeminiModelProfile struct {
	// API configuration
	APIKey     string `json:"api_key,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`

	// Default model parameters
	DefaultTemperature *float64 `json:"default_temperature,omitempty"`
	DefaultMaxTokens   *int32   `json:"default_max_tokens,omitempty"`
	DefaultTopP        *float32 `json:"default_top_p,omitempty"`
	DefaultTopK        *int32   `json:"default_top_k,omitempty"`
}

var _ ModelProfile = (*GeminiModelProfile)(nil)

func (g *GeminiModelProfile) Kind() ModelProfileKind {
	return ProviderKindGemini
}

func (g *GeminiModelProfile) Validate() error {
	if g.APIKey == "" {
		return fmt.Errorf("gemini API key is required")
	}
	if g.DefaultTemperature != nil && (*g.DefaultTemperature < 0 || *g.DefaultTemperature > 1.0) {
		return fmt.Errorf("temperature must be between 0 and 1.0")
	}
	if g.DefaultMaxTokens != nil && *g.DefaultMaxTokens < 0 {
		return fmt.Errorf("max_tokens must be non-negative")
	}
	if g.DefaultTopP != nil && (*g.DefaultTopP < 0 || *g.DefaultTopP > 1.0) {
		return fmt.Errorf("top_p must be between 0 and 1.0")
	}
	if g.DefaultTopK != nil && *g.DefaultTopK < 0 {
		return fmt.Errorf("top_k must be non-negative")
	}
	return nil
}

func NewGeminiProvider(apiKey string) (*GeminiProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini API key is required")
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}
	return &GeminiProvider{client: client}, nil
}

func (p *GeminiProvider) InvokeModel(ctx context.Context, model, systemPrompt string, messages []*Message, opts ...InvokeModelOption) (*Message, error) {
	if err := p.validateInput(model, systemPrompt, messages); err != nil {
		return nil, err
	}

	options := defaultGeminiInvokeOptions()
	for _, opt := range opts {
		opt(options)
	}

	_, err := ensureModelProfile[*GeminiModelProfile](options.ModelProfile)
	if err != nil {
		return nil, err
	}

	history, currentMsg, err := p.transformMessages(messages)
	if err != nil {
		return nil, err
	}

	geminiConfig := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{
			genai.NewPartFromText(systemPrompt),
		}},
	}

	tools := p.transformTools(options.Tools)
	if len(tools) > 0 {
		geminiConfig.Tools = tools
	}

	chat, err := p.client.Chats.Create(ctx, model, geminiConfig, history)
	if err != nil {
		return nil, err
	}

	var finalResp *genai.GenerateContentResponse
	var inputTokens, outputTokens int64

	stream := chat.SendStream(ctx, currentMsg...)

	for m, err := range stream {
		if err != nil {
			return nil, err
		}

		finalResp = m

		if len(m.Candidates) > 0 && m.Candidates[0].Content != nil {
			for _, part := range m.Candidates[0].Content.Parts {
				switch {
				case part.Text != "":
					options.StreamHandler(ctx, part.Text)
				case part.FunctionCall != nil:
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					toolCall := &ToolCallBlock{ID: uuid.NewString(), Tool: part.FunctionCall.Name, Args: argsJSON}

					toolCallJSON, _ := json.Marshal(toolCall)
					options.StreamHandler(ctx, string(toolCallJSON))
				}
			}
		}

		if m.UsageMetadata != nil {
			inputTokens = int64(m.UsageMetadata.PromptTokenCount)
			outputTokens = int64(m.UsageMetadata.CandidatesTokenCount)
		}
	}

	if finalResp == nil || len(finalResp.Candidates) == 0 || finalResp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("no response from gemini")
	}

	var content []ContentBlock
	for _, part := range finalResp.Candidates[0].Content.Parts {
		if part.Text != "" {
			content = append(content, &TextBlock{Text: part.Text})
		} else if part.FunctionCall != nil {
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)
			content = append(content, &ToolCallBlock{ID: uuid.NewString(), Tool: part.FunctionCall.Name, Args: argsJSON})
		}
	}

	return NewModelMessage(content, Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}), nil
}

func (p *GeminiProvider) validateInput(model, systemPrompt string, messages []*Message) error {
	if model == "" {
		return fmt.Errorf("model is required")
	}
	if systemPrompt == "" {
		return fmt.Errorf("system prompt is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("at least one message is required")
	}
	return nil
}

func defaultGeminiInvokeOptions() *InvokeModelOptions {
	return &InvokeModelOptions{
		Tools:        []native.Tool{},
		MaxTokens:    8192,
		Temperature:  0.0,
		ModelProfile: defaultGeminiModelProfile(),
	}
}

func defaultGeminiModelProfile() *GeminiModelProfile {
	return &GeminiModelProfile{
		APIKey:             "",
		BaseURL:            "",
		MaxRetries:         0,
		DefaultTemperature: nil,
		DefaultMaxTokens:   nil,
		DefaultTopP:        nil,
		DefaultTopK:        nil,
	}
}

func (p *GeminiProvider) transformMessages(messages []*Message) ([]*genai.Content, []*genai.Part, error) {
	contents := make([]*genai.Content, 0, len(messages))
	for _, m := range messages {
		c := &genai.Content{}
		switch m.Source {
		case MessageSourceUser:
			c.Role = "user"
		case MessageSourceModel:
			c.Role = "model"
		case MessageSourceSystem:
			c.Role = "user" // encode tool results as user-provided context
		}

		for _, block := range m.Content {
			switch b := block.(type) {
			case *TextBlock:
				c.Parts = append(c.Parts, genai.NewPartFromText(b.Text))
			case *ToolResultBlock:
				payload := map[string]any{}
				if err := json.Unmarshal([]byte(b.Result), &payload); err != nil {
					payload = map[string]any{"result": b.Result, "succeeded": b.Succeeded}
				}
				c.Parts = append(c.Parts, genai.NewPartFromFunctionResponse(b.Name, payload))
			case *ToolCallBlock:
				args := map[string]any{}
				_ = json.Unmarshal(b.Args, &args)
				c.Parts = append(c.Parts, genai.NewPartFromFunctionCall(b.Tool, args))
			}
		}

		contents = append(contents, c)
	}

	// Use all messages except the last one as history, last one as current input
	var history []*genai.Content
	var currentMsg []*genai.Part
	if len(contents) > 0 {
		lastContent := contents[len(contents)-1]
		currentMsg = lastContent.Parts
		if len(contents) > 1 {
			history = contents[:len(contents)-1]
		}
	}

	return history, currentMsg, nil
}

func (p *GeminiProvider) transformTools(tools []native.Tool) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		schema, err := p.convertJSONSchemaToGemini(t.Schema())
		if err != nil {
			schema = &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"input": {Type: genai.TypeString},
				},
			}
		}

		fd := &genai.FunctionDeclaration{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  schema,
		}

		decls = append(decls, fd)
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// convertJSONSchemaToGemini converts a JSON Schema (map[string]any) to genai.Schema
func (p *GeminiProvider) convertJSONSchemaToGemini(jsonSchema map[string]any) (*genai.Schema, error) {
	schema := &genai.Schema{}

	schemaType, exists := jsonSchema["type"]
	if !exists {
		return nil, fmt.Errorf("type is required")
	}

	if schemaType != "object" {
		return nil, fmt.Errorf("type must be object")
	}

	schema.Type = schemaTypeToGemini(schemaType.(string))

	if properties, ok := jsonSchema["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for propName, propDef := range properties {
			if propDefMap, ok := propDef.(map[string]any); ok {
				propSchema := &genai.Schema{}
				if propType, ok := propDefMap["type"].(string); ok {
					switch propType {
					case "object":
						propSchema.Type = genai.TypeObject
					case "string":
						propSchema.Type = genai.TypeString
					case "number":
						propSchema.Type = genai.TypeNumber
					case "integer":
						propSchema.Type = genai.TypeInteger
					case "boolean":
						propSchema.Type = genai.TypeBoolean
					case "array":
						propSchema.Type = genai.TypeArray
					default:
						propSchema.Type = genai.TypeString
					}
				}
				if description, ok := propDefMap["description"].(string); ok {
					propSchema.Description = description
				}
				// Handle enum values
				if enum, ok := propDefMap["enum"].([]any); ok {
					enumStrs := make([]string, 0, len(enum))
					for _, e := range enum {
						if s, ok := e.(string); ok {
							enumStrs = append(enumStrs, s)
						}
					}
					propSchema.Enum = enumStrs
				}
				// Handle array items
				if items, ok := propDefMap["items"].(map[string]any); ok {
					itemsSchema, _ := p.convertJSONSchemaToGemini(items)
					propSchema.Items = itemsSchema
				}
				schema.Properties[propName] = propSchema
			}
		}
	}

	if required, ok := jsonSchema["required"].([]any); ok {
		requiredStrs := make([]string, 0, len(required))
		for _, r := range required {
			if s, ok := r.(string); ok {
				requiredStrs = append(requiredStrs, s)
			}
		}
		schema.Required = requiredStrs
	}

	return schema, nil
}

func schemaTypeToGemini(schemaType string) genai.Type {
	switch schemaType {
	case "object":
		return genai.TypeObject
	case "string":
		return genai.TypeString
	case "number":
		return genai.TypeNumber
	case "integer":
		return genai.TypeInteger
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	default:
		return genai.TypeString
	}
}

func SupportedGeminiModels() []Model {
	return []Model{
		{
			ID:       uuid.MustParse("01970000-0001-7000-8000-000000000001"),
			Name:     "gemini-2.5-pro",
			Provider: ProviderKindGemini,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityAudio,
			},
			ContextWindow: 1048576,
			Pricing: ModelPricing{
				Input:      1.125,
				Output:     10.0,
				CacheWrite: 0.0,
				CacheRead:  0.0,
			},
		},
		{
			ID:       uuid.MustParse("01970000-0002-7000-8000-000000000002"),
			Name:     "gemini-2.5-flash",
			Provider: ProviderKindGemini,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityAudio,
			},
			ContextWindow: 1048576,
			Pricing: ModelPricing{
				Input:      1.25,
				Output:     5.0,
				CacheWrite: 0.0,
				CacheRead:  0.0,
			},
		},
		{
			ID:       uuid.MustParse("01970000-0003-7000-8000-000000000003"),
			Name:     "gemini-2.5-flash-lite",
			Provider: ProviderKindGemini,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityAudio,
			},
			ContextWindow: 1000000,
			Pricing: ModelPricing{
				Input:      0.075,
				Output:     0.3,
				CacheWrite: 0.0,
				CacheRead:  0.0,
			},
		},
	}
}

func (p *GeminiProvider) GetModel(ctx context.Context, modelID uuid.UUID) (Model, error) {
	for _, model := range SupportedGeminiModels() {
		if model.ID == modelID {
			return model, nil
		}
	}

	return Model{}, fmt.Errorf("model not supported")
}
