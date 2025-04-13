package terminal

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
)

type model struct {
	viewport  viewport.Model
	textInput textinput.Model

	width  int
	height int

	// waiting             bool
	// typingMessage       *message
	// typingAnimationDone bool

	apiClient   *api_client.Client
	messages    []message
	task        *v1.Task
	activeAgent string
	agents      []string

	eventChannel chan *v1.SubscribeResponse
	ctx          context.Context
}

func NewModel(ctx context.Context, apiClient *api_client.Client, task *v1.Task) model {
	ti := textinput.New()
	ti.Placeholder = "What do you want to build?"
	ti.Focus()
	ti.CharLimit = 512
	ti.Width = 80

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return model{
		apiClient: apiClient,
		textInput: ti,
		viewport:  vp,
		messages:  []message{},
		// waiting:       false,
		activeAgent: *task.Spec.AgentId,
		task:        task,
		eventChannel: make(chan *v1.SubscribeResponse, 100),
		ctx:          ctx,
		// typingMessage: nil,
	}
}

type messageType int

const (
	MessageTypeUser messageType = iota
	MessageTypeAssistantText
	MessageTypeAssistantTool
	MessageTypeAssistantTyping
)

type message interface {
	Type() messageType
}

type userMessage struct {
	content string
}

func (m *userMessage) Type() messageType {
	return MessageTypeUser
}

type assistantTextMessage struct {
	content string
}

func (m *assistantTextMessage) Type() messageType {
	return MessageTypeAssistantText
}
