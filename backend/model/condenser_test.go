package model

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type CondenserTestExpectation struct {
	Result *CondenserResult
	Error  string
}

type CondenserTestScenario struct {
	Name      string
	Condenser *TruncationCondenser
	Messages  []*Message
	Expected  CondenserTestExpectation
}

type CondenserTestSetup struct {
	CmpOptions []cmp.Option
}

func (setup *CondenserTestSetup) RunCondenserTests(t *testing.T, scenarios []CondenserTestScenario) {
	t.Helper()

	for _, scenario := range scenarios {
		scenario := scenario // capture loop variable
		t.Run(scenario.Name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			result, err := scenario.Condenser.Condense(ctx, scenario.Messages)

			if scenario.Expected.Error != "" {
				if err == nil {
					t.Errorf("Expected error %q, got nil", scenario.Expected.Error)
					return
				}
				if err.Error() != scenario.Expected.Error {
					t.Errorf("Expected error %q, got %q", scenario.Expected.Error, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(scenario.Expected.Result, result, setup.CmpOptions...); diff != "" {
				t.Errorf("Result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTruncationCondenser(t *testing.T) {
	t.Parallel()

	setup := &CondenserTestSetup{
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(CondenserResult{}),
			cmp.AllowUnexported(Message{}, Usage{}, TextBlock{}),
			cmpopts.EquateEmpty(),
		},
	}

	setup.RunCondenserTests(t, []CondenserTestScenario{
		{
			Name:      "empty messages",
			Condenser: NewTruncationCondenser(100000),
			Messages:  []*Message{},
			Expected: CondenserTestExpectation{
				Result: &CondenserResult{},
			},
		},
		{
			Name:      "no model message",
			Condenser: NewTruncationCondenser(100000),
			Messages:  createUserMessages(5),
			Expected: CondenserTestExpectation{
				Result: &CondenserResult{},
			},
		},
		{
			Name:      "below threshold",
			Condenser: NewTruncationCondenser(100000),
			Messages: []*Message{
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),
				// Model message with token usage below threshold (80000 tokens)
				createTestMessage(MessageSourceModel, 30000, 20000, 5000, 5000), // Total: 60000 < 80000
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),
			},
			Expected: CondenserTestExpectation{
				Result: &CondenserResult{},
			},
		},
		{
			Name:      "insufficient messages for truncation",
			Condenser: NewTruncationCondenser(100000),
			Messages: []*Message{
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),
				// Model message with high token usage above threshold
				createTestMessage(MessageSourceModel, 50000, 30000, 10000, 10000), // Total: 100000 > 80000
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),
				// Only 5 messages total, need 6 for truncation
			},
			Expected: CondenserTestExpectation{
				Result: &CondenserResult{},
			},
		},
		{
			Name:      "successful truncation",
			Condenser: NewTruncationCondenser(100000),
			Messages: func() []*Message {
				messages := []*Message{
					createTestMessage(MessageSourceUser, 0, 0, 0, 0),                  // preserve
					createTestMessage(MessageSourceUser, 0, 0, 0, 0),                  // preserve
					createTestMessage(MessageSourceUser, 0, 0, 0, 0),                  // eligible for removal
					createTestMessage(MessageSourceUser, 0, 0, 0, 0),                  // eligible for removal
					createTestMessage(MessageSourceUser, 0, 0, 0, 0),                  // eligible for removal
					createTestMessage(MessageSourceUser, 0, 0, 0, 0),                  // eligible for removal
					createTestMessage(MessageSourceUser, 0, 0, 0, 0),                  // preserve
					createTestMessage(MessageSourceModel, 50000, 30000, 10000, 10000), // preserve - Total: 100000 > 80000
				}
				return messages
			}(),
			Expected: CondenserTestExpectation{
				Result: &CondenserResult{
					AddedMessages: []*Message{},
					RemovedMessages: func() []*Message {
						// Should remove middle 2 of the 4 eligible messages (indices 3, 4)
						return []*Message{
							createTestMessage(MessageSourceUser, 0, 0, 0, 0),
							createTestMessage(MessageSourceUser, 0, 0, 0, 0),
						}
					}(),
				},
			},
		},
		{
			Name: "custom configuration",
			Condenser: &TruncationCondenser{
				ContextWindow:     50000,
				TruncationRatio:   0.9,  // Trigger at 90%
				PreserveCount:     1,    // Preserve only 1 message at each end
				MaxRemovalPercent: 0.25, // Remove at most 25%
			},
			Messages: []*Message{
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),          // preserve
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),          // eligible
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),          // eligible
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),          // eligible
				createTestMessage(MessageSourceUser, 0, 0, 0, 0),          // eligible
				createTestMessage(MessageSourceModel, 30000, 20000, 0, 0), // preserve - Total: 50000 > 45000 (90% of 50000)
			},
			Expected: CondenserTestExpectation{
				Result: &CondenserResult{
					AddedMessages: []*Message{},
					RemovedMessages: []*Message{
						// Should remove 1 message (4 * 0.25 = 1) from middle of eligible range
						createTestMessage(MessageSourceUser, 0, 0, 0, 0),
					},
				},
			},
		},
	})
}

func createTestMessage(source MessageSource, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64) *Message {
	return &Message{
		Source: source,
		Content: []ContentBlock{
			&TextBlock{Text: "test message"},
		},
		Usage: Usage{
			InputTokens:      inputTokens,
			OutputTokens:     outputTokens,
			CacheReadTokens:  cacheReadTokens,
			CacheWriteTokens: cacheWriteTokens,
		},
	}
}

func createUserMessages(count int) []*Message {
	messages := make([]*Message, count)
	for i := 0; i < count; i++ {
		messages[i] = createTestMessage(MessageSourceUser, 0, 0, 0, 0)
	}
	return messages
}
