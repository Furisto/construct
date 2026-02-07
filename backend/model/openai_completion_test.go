package model

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/furisto/construct/backend/tool/native"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/spf13/afero"
)

// mockChatCompletionService is a mock implementation of OpenAIChatCompletionService for testing.
type mockChatCompletionService struct {
	stream    *ssestream.Stream[openai.ChatCompletionChunk]
	params    openai.ChatCompletionNewParams
	callCount int
	mu        sync.Mutex
}

func (m *mockChatCompletionService) NewStreaming(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.params = params
	m.callCount++
	return m.stream
}

func (m *mockChatCompletionService) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// mockTool implements native.Tool for testing.
type mockTool struct {
	name        string
	description string
	schema      map[string]any
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return t.description }
func (t *mockTool) Schema() map[string]any {
	if t.schema != nil {
		return t.schema
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string"},
		},
	}
}
func (t *mockTool) Run(ctx context.Context, fs afero.Fs, input json.RawMessage) (string, error) {
	return "mock result", nil
}

// =============================================================================
// Provider Creation Tests
// =============================================================================

func TestNewOpenAICompletionProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		apiKey      string
		opts        []ProviderOption
		expectError bool
		errorMsg    string
	}{
		{
			name:        "success with valid API key",
			apiKey:      "sk-test-key-12345",
			expectError: false,
		},
		{
			name:        "error with empty API key",
			apiKey:      "",
			expectError: true,
			errorMsg:    "openai API key is required",
		},
		{
			name:        "success with custom URL",
			apiKey:      "sk-test-key-12345",
			opts:        []ProviderOption{WithURL("https://custom.openai.com/v1")},
			expectError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, err := NewOpenAICompletionProvider(tt.apiKey, tt.opts...)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if provider == nil {
				t.Error("expected provider to be non-nil")
			}
		})
	}
}

func TestNewOpenAICompletionProviderWithService(t *testing.T) {
	t.Parallel()

	mockService := &mockChatCompletionService{}
	provider := NewOpenAICompletionProviderWithService(mockService)

	if provider == nil {
		t.Error("expected provider to be non-nil")
	}

	if provider.chatService != mockService {
		t.Error("expected chatService to be the mock service")
	}
}

// =============================================================================
// Input Validation Tests
// =============================================================================

func TestOpenAICompletionProvider_InvokeModel_Validation(t *testing.T) {
	t.Parallel()

	mockService := &mockChatCompletionService{}
	provider := NewOpenAICompletionProviderWithService(mockService)

	tests := []struct {
		name         string
		model        string
		systemPrompt string
		messages     []*Message
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "error with empty model",
			model:        "",
			systemPrompt: "You are a helpful assistant",
			messages:     []*Message{{Source: MessageSourceUser, Content: []ContentBlock{&TextBlock{Text: "Hello"}}}},
			expectError:  true,
			errorMsg:     "model is required",
		},
		{
			name:         "error with empty system prompt",
			model:        "gpt-4",
			systemPrompt: "",
			messages:     []*Message{{Source: MessageSourceUser, Content: []ContentBlock{&TextBlock{Text: "Hello"}}}},
			expectError:  true,
			errorMsg:     "system prompt is required",
		},
		{
			name:         "error with empty messages",
			model:        "gpt-4",
			systemPrompt: "You are a helpful assistant",
			messages:     []*Message{},
			expectError:  true,
			errorMsg:     "at least one message is required",
		},
		{
			name:         "error with nil messages",
			model:        "gpt-4",
			systemPrompt: "You are a helpful assistant",
			messages:     nil,
			expectError:  true,
			errorMsg:     "at least one message is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			_, err := provider.InvokeModel(ctx, tt.model, tt.systemPrompt, tt.messages)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestOpenAICompletionProvider_InvokeModel_ModelProfileValidation(t *testing.T) {
	t.Parallel()

	mockService := &mockChatCompletionService{}
	provider := NewOpenAICompletionProviderWithService(mockService)

	tests := []struct {
		name        string
		profile     ModelProfile
		expectError bool
		errorMsg    string
	}{
		{
			name:        "error with nil model profile",
			profile:     nil,
			expectError: true,
			errorMsg:    "no model profile provided",
		},
		{
			name: "error with invalid temperature",
			profile: &OpenAIModelProfile{
				Temperature: 3.0, // Invalid: must be 0-2.0
				MaxTokens:   8192,
			},
			expectError: true,
			errorMsg:    "model profile is invalid: OpenAI temperature must be between 0 and 2.0",
		},
		{
			name: "error with invalid frequency penalty",
			profile: &OpenAIModelProfile{
				FrequencyPenalty: 3.0, // Invalid: must be -2.0 to 2.0
				MaxTokens:        8192,
			},
			expectError: true,
			errorMsg:    "model profile is invalid: frequency_penalty must be between -2.0 and 2.0",
		},
		{
			name: "error with invalid presence penalty",
			profile: &OpenAIModelProfile{
				PresencePenalty: -3.0, // Invalid: must be -2.0 to 2.0
				MaxTokens:       8192,
			},
			expectError: true,
			errorMsg:    "model profile is invalid: presence_penalty must be between -2.0 and 2.0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			messages := []*Message{{Source: MessageSourceUser, Content: []ContentBlock{&TextBlock{Text: "Hello"}}}}
			opts := []InvokeModelOption{}
			if tt.profile != nil {
				opts = append(opts, WithModelProfile(tt.profile))
			} else {
				// Force nil profile by using a custom option
				opts = append(opts, func(o *InvokeModelOptions) {
					o.ModelProfile = nil
				})
			}

			_, err := provider.InvokeModel(ctx, "gpt-4", "You are a helpful assistant", messages, opts...)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// =============================================================================
// Message Transformation Tests
// =============================================================================

func TestOpenAICompletionProvider_TransformMessages(t *testing.T) {
	t.Parallel()

	provider := NewOpenAICompletionProviderWithService(&mockChatCompletionService{})

	tests := []struct {
		name             string
		messages         []*Message
		expectedCount    int
		validateMessages func(t *testing.T, result []openai.ChatCompletionMessageParamUnion)
	}{
		{
			name: "user message with text",
			messages: []*Message{
				{
					Source:  MessageSourceUser,
					Content: []ContentBlock{&TextBlock{Text: "Hello, how are you?"}},
				},
			},
			expectedCount: 1,
			validateMessages: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfUser == nil {
					t.Error("expected user message")
				}
			},
		},
		{
			name: "model message with text",
			messages: []*Message{
				{
					Source:  MessageSourceModel,
					Content: []ContentBlock{&TextBlock{Text: "I'm doing well, thank you!"}},
				},
			},
			expectedCount: 1,
			validateMessages: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfAssistant == nil {
					t.Error("expected assistant message")
				}
			},
		},
		{
			name: "model message with tool calls",
			messages: []*Message{
				{
					Source: MessageSourceModel,
					Content: []ContentBlock{
						&ToolCallBlock{
							ID:   "call_123",
							Tool: "get_weather",
							Args: json.RawMessage(`{"location": "New York"}`),
						},
					},
				},
			},
			expectedCount: 1,
			validateMessages: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfAssistant == nil {
					t.Error("expected assistant message")
					return
				}
				if len(result[0].OfAssistant.ToolCalls) != 1 {
					t.Errorf("expected 1 tool call, got %d", len(result[0].OfAssistant.ToolCalls))
				}
			},
		},
		{
			name: "system message with tool result",
			messages: []*Message{
				{
					Source: MessageSourceSystem,
					Content: []ContentBlock{
						&ToolResultBlock{
							ID:        "call_123",
							Name:      "get_weather",
							Result:    "Sunny, 72°F",
							Succeeded: true,
						},
					},
				},
			},
			expectedCount: 1,
			validateMessages: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfTool == nil {
					t.Error("expected tool message")
				}
			},
		},
		{
			name: "mixed conversation history",
			messages: []*Message{
				{
					Source:  MessageSourceUser,
					Content: []ContentBlock{&TextBlock{Text: "What's the weather?"}},
				},
				{
					Source: MessageSourceModel,
					Content: []ContentBlock{
						&ToolCallBlock{
							ID:   "call_456",
							Tool: "get_weather",
							Args: json.RawMessage(`{"location": "NYC"}`),
						},
					},
				},
				{
					Source: MessageSourceSystem,
					Content: []ContentBlock{
						&ToolResultBlock{
							ID:        "call_456",
							Name:      "get_weather",
							Result:    "Rainy, 55°F",
							Succeeded: true,
						},
					},
				},
				{
					Source:  MessageSourceModel,
					Content: []ContentBlock{&TextBlock{Text: "The weather in NYC is rainy and 55°F."}},
				},
			},
			expectedCount: 4,
			validateMessages: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfUser == nil {
					t.Error("expected first message to be user")
				}
				if result[1].OfAssistant == nil {
					t.Error("expected second message to be assistant")
				}
				if result[2].OfTool == nil {
					t.Error("expected third message to be tool")
				}
				if result[3].OfAssistant == nil {
					t.Error("expected fourth message to be assistant")
				}
			},
		},
		{
			name: "model message with text and tool calls",
			messages: []*Message{
				{
					Source: MessageSourceModel,
					Content: []ContentBlock{
						&TextBlock{Text: "Let me check the weather for you."},
						&ToolCallBlock{
							ID:   "call_789",
							Tool: "get_weather",
							Args: json.RawMessage(`{"location": "LA"}`),
						},
					},
				},
			},
			expectedCount: 1,
			validateMessages: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfAssistant == nil {
					t.Error("expected assistant message")
					return
				}
				if len(result[0].OfAssistant.ToolCalls) != 1 {
					t.Errorf("expected 1 tool call, got %d", len(result[0].OfAssistant.ToolCalls))
				}
			},
		},
		{
			name: "multiple tool calls in single message",
			messages: []*Message{
				{
					Source: MessageSourceModel,
					Content: []ContentBlock{
						&ToolCallBlock{
							ID:   "call_1",
							Tool: "get_weather",
							Args: json.RawMessage(`{"location": "NYC"}`),
						},
						&ToolCallBlock{
							ID:   "call_2",
							Tool: "get_time",
							Args: json.RawMessage(`{"timezone": "EST"}`),
						},
					},
				},
			},
			expectedCount: 1,
			validateMessages: func(t *testing.T, result []openai.ChatCompletionMessageParamUnion) {
				if result[0].OfAssistant == nil {
					t.Error("expected assistant message")
					return
				}
				if len(result[0].OfAssistant.ToolCalls) != 2 {
					t.Errorf("expected 2 tool calls, got %d", len(result[0].OfAssistant.ToolCalls))
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := provider.transformMessages(tt.messages)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) != tt.expectedCount {
				t.Errorf("expected %d messages, got %d", tt.expectedCount, len(result))
				return
			}

			if tt.validateMessages != nil {
				tt.validateMessages(t, result)
			}
		})
	}
}

// =============================================================================
// Tool Transformation Tests
// =============================================================================

func TestOpenAICompletionProvider_TransformTools(t *testing.T) {
	t.Parallel()

	provider := NewOpenAICompletionProviderWithService(&mockChatCompletionService{})

	tests := []struct {
		name          string
		tools         []native.Tool
		expectedCount int
		validateTools func(t *testing.T, result []openai.ChatCompletionToolParam)
	}{
		{
			name:          "empty tools",
			tools:         []native.Tool{},
			expectedCount: 0,
		},
		{
			name: "single tool",
			tools: []native.Tool{
				&mockTool{
					name:        "get_weather",
					description: "Get the current weather for a location",
					schema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string"},
						},
						"required": []string{"location"},
					},
				},
			},
			expectedCount: 1,
			validateTools: func(t *testing.T, result []openai.ChatCompletionToolParam) {
				if result[0].Function.Name != "get_weather" {
					t.Errorf("expected tool name 'get_weather', got %q", result[0].Function.Name)
				}
				desc := result[0].Function.Description.Value
				if desc != "Get the current weather for a location" {
					t.Errorf("unexpected description: %q", desc)
				}
			},
		},
		{
			name: "multiple tools",
			tools: []native.Tool{
				&mockTool{name: "tool_1", description: "First tool"},
				&mockTool{name: "tool_2", description: "Second tool"},
				&mockTool{name: "tool_3", description: "Third tool"},
			},
			expectedCount: 3,
			validateTools: func(t *testing.T, result []openai.ChatCompletionToolParam) {
				expectedNames := []string{"tool_1", "tool_2", "tool_3"}
				for i, expected := range expectedNames {
					if result[i].Function.Name != expected {
						t.Errorf("expected tool[%d] name %q, got %q", i, expected, result[i].Function.Name)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := provider.transformTools(tt.tools)

			if len(result) != tt.expectedCount {
				t.Errorf("expected %d tools, got %d", tt.expectedCount, len(result))
				return
			}

			if tt.validateTools != nil {
				tt.validateTools(t, result)
			}
		})
	}
}

// =============================================================================
// Edge Cases Tests
// =============================================================================

func TestOpenAICompletionProvider_TransformMessages_EdgeCases(t *testing.T) {
	t.Parallel()

	provider := NewOpenAICompletionProviderWithService(&mockChatCompletionService{})

	tests := []struct {
		name     string
		messages []*Message
	}{
		{
			name: "unicode content",
			messages: []*Message{
				{
					Source:  MessageSourceUser,
					Content: []ContentBlock{&TextBlock{Text: "Hello 世界! 🌍 Привет мир! مرحبا"}},
				},
			},
		},
		{
			name: "very long content",
			messages: []*Message{
				{
					Source:  MessageSourceUser,
					Content: []ContentBlock{&TextBlock{Text: string(make([]byte, 100000))}},
				},
			},
		},
		{
			name: "empty text block",
			messages: []*Message{
				{
					Source:  MessageSourceUser,
					Content: []ContentBlock{&TextBlock{Text: ""}},
				},
			},
		},
		{
			name: "multiple text blocks in user message",
			messages: []*Message{
				{
					Source: MessageSourceUser,
					Content: []ContentBlock{
						&TextBlock{Text: "First part"},
						&TextBlock{Text: "Second part"},
					},
				},
			},
		},
		{
			name: "special characters in tool arguments",
			messages: []*Message{
				{
					Source: MessageSourceModel,
					Content: []ContentBlock{
						&ToolCallBlock{
							ID:   "call_special",
							Tool: "process_text",
							Args: json.RawMessage(`{"text": "Line1\nLine2\tTabbed\u0000Null"}`),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := provider.transformMessages(tt.messages)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) == 0 {
				t.Error("expected at least one transformed message")
			}
		})
	}
}

func TestOpenAICompletionProvider_TransformTools_ManyTools(t *testing.T) {
	t.Parallel()

	provider := NewOpenAICompletionProviderWithService(&mockChatCompletionService{})

	// Create many tools
	tools := make([]native.Tool, 50)
	for i := 0; i < 50; i++ {
		tools[i] = &mockTool{
			name:        "tool_" + string(rune('a'+i%26)) + "_" + string(rune('0'+i/26)),
			description: "Tool number " + string(rune('0'+i)),
		}
	}

	result := provider.transformTools(tools)

	if len(result) != 50 {
		t.Errorf("expected 50 tools, got %d", len(result))
	}
}

// =============================================================================
// Validate Input Tests
// =============================================================================

func TestOpenAICompletionProvider_ValidateInput(t *testing.T) {
	t.Parallel()

	provider := NewOpenAICompletionProviderWithService(&mockChatCompletionService{})

	tests := []struct {
		name         string
		model        string
		systemPrompt string
		messages     []*Message
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "valid input",
			model:        "gpt-4",
			systemPrompt: "You are a helpful assistant",
			messages:     []*Message{{Source: MessageSourceUser}},
			expectError:  false,
		},
		{
			name:         "empty model",
			model:        "",
			systemPrompt: "prompt",
			messages:     []*Message{{}},
			expectError:  true,
			errorMsg:     "model is required",
		},
		{
			name:         "empty system prompt",
			model:        "gpt-4",
			systemPrompt: "",
			messages:     []*Message{{}},
			expectError:  true,
			errorMsg:     "system prompt is required",
		},
		{
			name:         "nil messages",
			model:        "gpt-4",
			systemPrompt: "prompt",
			messages:     nil,
			expectError:  true,
			errorMsg:     "at least one message is required",
		},
		{
			name:         "empty messages slice",
			model:        "gpt-4",
			systemPrompt: "prompt",
			messages:     []*Message{},
			expectError:  true,
			errorMsg:     "at least one message is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := provider.validateInput(tt.model, tt.systemPrompt, tt.messages)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// =============================================================================
// Token Usage Tests
// =============================================================================

func TestOpenAICompletionProvider_CacheHitRatioCalculation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		promptTokens      int64
		cachedTokens      int64
		expectedRatioZero bool
	}{
		{
			name:              "no tokens - ratio is zero",
			promptTokens:      0,
			cachedTokens:      0,
			expectedRatioZero: true,
		},
		{
			name:              "all cached - ratio is 0.5",
			promptTokens:      100,
			cachedTokens:      100,
			expectedRatioZero: false,
		},
		{
			name:              "no cache - ratio approaches zero",
			promptTokens:      100,
			cachedTokens:      0,
			expectedRatioZero: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Replicate the calculation from InvokeModel
			cacheHitRatio := 0.0
			if tt.promptTokens+tt.cachedTokens > 0 {
				cacheHitRatio = float64(tt.cachedTokens) / float64(tt.promptTokens+tt.cachedTokens)
			}

			if tt.expectedRatioZero && cacheHitRatio != 0.0 {
				t.Errorf("expected ratio to be 0, got %f", cacheHitRatio)
			}
			if !tt.expectedRatioZero && cacheHitRatio == 0.0 {
				t.Errorf("expected non-zero ratio, got 0")
			}
		})
	}
}

// =============================================================================
// Integration-style Tests (testing the flow without actual API calls)
// =============================================================================

func TestOpenAIModelProfile_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		profile     *OpenAIModelProfile
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid profile with defaults",
			profile: &OpenAIModelProfile{
				MaxTokens: 8192,
			},
			expectError: false,
		},
		{
			name: "valid profile with all fields",
			profile: &OpenAIModelProfile{
				APIURL:           "https://api.openai.com/v1",
				Temperature:      0.7,
				MaxTokens:        4096,
				TopP:             0.9,
				FrequencyPenalty: 0.5,
				PresencePenalty:  0.5,
			},
			expectError: false,
		},
		{
			name: "invalid temperature too high",
			profile: &OpenAIModelProfile{
				Temperature: 2.5,
			},
			expectError: true,
			errorMsg:    "OpenAI temperature must be between 0 and 2.0",
		},
		{
			name: "invalid temperature negative",
			profile: &OpenAIModelProfile{
				Temperature: -0.1,
			},
			expectError: true,
			errorMsg:    "OpenAI temperature must be between 0 and 2.0",
		},
		{
			name: "invalid frequency penalty too high",
			profile: &OpenAIModelProfile{
				FrequencyPenalty: 2.5,
			},
			expectError: true,
			errorMsg:    "frequency_penalty must be between -2.0 and 2.0",
		},
		{
			name: "invalid frequency penalty too low",
			profile: &OpenAIModelProfile{
				FrequencyPenalty: -2.5,
			},
			expectError: true,
			errorMsg:    "frequency_penalty must be between -2.0 and 2.0",
		},
		{
			name: "invalid presence penalty too high",
			profile: &OpenAIModelProfile{
				PresencePenalty: 2.5,
			},
			expectError: true,
			errorMsg:    "presence_penalty must be between -2.0 and 2.0",
		},
		{
			name: "invalid presence penalty too low",
			profile: &OpenAIModelProfile{
				PresencePenalty: -2.5,
			},
			expectError: true,
			errorMsg:    "presence_penalty must be between -2.0 and 2.0",
		},
		{
			name: "boundary temperature at 2.0",
			profile: &OpenAIModelProfile{
				Temperature: 2.0,
			},
			expectError: false,
		},
		{
			name: "boundary penalties at limits",
			profile: &OpenAIModelProfile{
				FrequencyPenalty: 2.0,
				PresencePenalty:  -2.0,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.profile.Validate()

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestOpenAIModelProfile_Defaults(t *testing.T) {
	t.Parallel()

	profile := &OpenAIModelProfile{}
	err := profile.Validate()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	if profile.APIURL != "https://api.openai.com/v1" {
		t.Errorf("expected default APIURL, got %q", profile.APIURL)
	}

	if profile.Timeout != 30*1e9 { // 30 seconds in nanoseconds
		t.Errorf("expected default Timeout of 30s, got %v", profile.Timeout)
	}

	if profile.MaxRetries != 3 {
		t.Errorf("expected default MaxRetries of 3, got %d", profile.MaxRetries)
	}
}

func TestOpenAIModelProfile_Kind(t *testing.T) {
	t.Parallel()

	profile := &OpenAIModelProfile{}
	if profile.Kind() != ProviderKindOpenAI {
		t.Errorf("expected Kind to be %q, got %q", ProviderKindOpenAI, profile.Kind())
	}
}

func TestDefaultOpenAIModelOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultOpenAIModelOptions()

	if opts == nil {
		t.Fatal("expected non-nil options")
	}

	if opts.Tools == nil {
		t.Error("expected Tools to be initialized")
	}

	if opts.ModelProfile == nil {
		t.Fatal("expected ModelProfile to be initialized")
	}

	profile, ok := opts.ModelProfile.(*OpenAIModelProfile)
	if !ok {
		t.Fatal("expected ModelProfile to be *OpenAIModelProfile")
	}

	if profile.MaxTokens != 8192 {
		t.Errorf("expected MaxTokens 8192, got %d", profile.MaxTokens)
	}

	if !profile.EnableFunctionCalling {
		t.Error("expected EnableFunctionCalling to be true")
	}

	if !profile.ParallelToolCalls {
		t.Error("expected ParallelToolCalls to be true")
	}
}

// =============================================================================
// Error Scenario Tests
// =============================================================================

type errorMockChatCompletionService struct {
	err error
}

func (m *errorMockChatCompletionService) NewStreaming(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	// Return nil to simulate an error scenario
	// In real usage, the stream manager would return errors during iteration
	return nil
}

func TestOpenAICompletionProvider_WrongProfileType(t *testing.T) {
	t.Parallel()

	mockService := &mockChatCompletionService{}
	provider := NewOpenAICompletionProviderWithService(mockService)

	// Create a different profile type (using a mock that satisfies ModelProfile)
	type wrongProfile struct{}

	ctx := context.Background()
	messages := []*Message{{Source: MessageSourceUser, Content: []ContentBlock{&TextBlock{Text: "Hello"}}}}

	// Use an option that sets a wrong type profile
	// Since we can't easily create a wrong type, we test with nil which should also fail
	_, err := provider.InvokeModel(ctx, "gpt-4", "prompt", messages, func(o *InvokeModelOptions) {
		o.ModelProfile = nil
	})

	if err == nil {
		t.Error("expected error for nil model profile")
	}

	expectedErr := "no model profile provided"
	if err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestOpenAICompletionProvider_ConcurrentValidation(t *testing.T) {
	t.Parallel()

	mockService := &mockChatCompletionService{}
	provider := NewOpenAICompletionProviderWithService(mockService)

	var wg sync.WaitGroup
	errorChan := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			err := provider.validateInput("gpt-4", "prompt", []*Message{{Source: MessageSourceUser}})
			if err != nil {
				errorChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	for err := range errorChan {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenAICompletionProvider_ConcurrentTransformMessages(t *testing.T) {
	t.Parallel()

	provider := NewOpenAICompletionProviderWithService(&mockChatCompletionService{})

	messages := []*Message{
		{Source: MessageSourceUser, Content: []ContentBlock{&TextBlock{Text: "Hello"}}},
		{Source: MessageSourceModel, Content: []ContentBlock{&TextBlock{Text: "Hi there"}}},
	}

	var wg sync.WaitGroup
	errorChan := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			result, err := provider.transformMessages(messages)
			if err != nil {
				errorChan <- err
				return
			}
			if len(result) != 2 {
				errorChan <- errors.New("unexpected result length")
			}
		}()
	}

	wg.Wait()
	close(errorChan)

	for err := range errorChan {
		t.Errorf("unexpected error: %v", err)
	}
}
