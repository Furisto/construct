package conv

import (
	"testing"
	"time"

	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
)

func TestFinalResponseStatusForRPCAndEvents(t *testing.T) {
	for _, tc := range []struct {
		name      string
		source    types.MessageSource
		processed bool
		tool      bool
		want      bool
	}{
		{"final", types.MessageSourceAssistant, true, false, true},
		{"streaming", types.MessageSourceAssistant, false, false, false},
		{"tool call", types.MessageSourceAssistant, true, true, false},
		{"user", types.MessageSourceUser, true, false, false},
		{"system", types.MessageSourceSystem, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &memory.Message{Source: tc.source, Content: &types.MessageContent{}}
			if tc.processed {
				m.ProcessedTime = time.Now()
			}
			if tc.tool {
				m.Content.Blocks = []types.MessageBlock{{Kind: types.MessageBlockKindToolCall, Payload: `{"provider":{"kind":"anthropic","id":"call"},"tool":"code_interpreter","input":{"interpreter":{"script":"print(1)"}}}`}}
			}
			rpc, err := ConvertMemoryMessageToProto(m)
			if err != nil {
				t.Fatal(err)
			}
			if rpc.Status.IsFinalResponse != tc.want {
				t.Fatalf("RPC final=%v, want %v", rpc.Status.IsFinalResponse, tc.want)
			}
			stream, err := ConvertStreamEventToProto(event.NewMessageCreatedEvent(m))
			if err != nil {
				t.Fatal(err)
			}
			if stream.GetMessage().GetMessage().GetStatus().GetIsFinalResponse() != tc.want {
				t.Fatalf("event final flag differs from RPC")
			}
		})
	}
}
