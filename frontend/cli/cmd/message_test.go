package cmd

import (
	"testing"

	v1 "github.com/furisto/construct/api/go/v1"
)

func TestConvertMessageToDisplayContent(t *testing.T) {
	text := &v1.MessagePart{Data: &v1.MessagePart_Text_{Text: &v1.MessagePart_Text{Content: "reply"}}}
	for _, tt := range []struct {
		name    string
		content []*v1.MessagePart
		want    string
	}{
		{name: "empty"},
		{name: "text", content: []*v1.MessagePart{text}, want: "reply"},
		{name: "non-text and nil", content: []*v1.MessagePart{nil, {}, {Data: &v1.MessagePart_ToolCall{}}, text}, want: "reply"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			message := &v1.Message{Metadata: &v1.MessageMetadata{}, Spec: &v1.MessageSpec{Content: tt.content}}
			if got := ConvertMessageToDisplay(message).Content; got != tt.want {
				t.Fatalf("content = %q, want %q", got, tt.want)
			}
		})
	}
}
