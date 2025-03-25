package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/tool"
	"github.com/google/uuid"
	"k8s.io/client-go/util/workqueue"
)

type AgentOptions struct {
	SystemPrompt   string
	ModelProviders []model.ModelProvider
	Tools          []tool.Tool
	Mailbox        Memory
	SystemMemory   Memory
	UserMemory     Memory
	Concurrency    int
}

func DefaultAgentOptions() *AgentOptions {
	return &AgentOptions{
		ModelProviders: []model.ModelProvider{},
		SystemPrompt:   "You are a helpful assistant that can help with tasks and answer questions.",
		Tools:          []tool.Tool{},
		Mailbox:        NewEphemeralMemory(),
		SystemMemory:   NewEphemeralMemory(),
		UserMemory:     NewEphemeralMemory(),
		Concurrency:    5,
	}
}

type AgentOption func(*AgentOptions)

func WithSystemPrompt(systemPrompt string) AgentOption {
	return func(o *AgentOptions) {
		o.SystemPrompt = systemPrompt
	}
}

func WithModelProviders(modelProviders ...model.ModelProvider) AgentOption {
	return func(o *AgentOptions) {
		o.ModelProviders = modelProviders
	}
}

func WithTools(tools ...tool.Tool) AgentOption {
	return func(o *AgentOptions) {
		o.Tools = tools
	}
}

func WithMailbox(mailbox Memory) AgentOption {
	return func(o *AgentOptions) {
		o.Mailbox = mailbox
	}
}

func WithSystemMemory(memory Memory) AgentOption {
	return func(o *AgentOptions) {
		o.SystemMemory = memory
	}
}

func WithUserMemory(memory Memory) AgentOption {
	return func(o *AgentOptions) {
		o.UserMemory = memory
	}
}

func WithConcurrency(concurrency int) AgentOption {
	return func(o *AgentOptions) {
		o.Concurrency = concurrency
	}
}

type Agent struct {
	ModelProviders []model.ModelProvider
	SystemPrompt   string
	Toolbox        *tool.Toolbox
	Mailbox        *Mailbox
	SystemMemory   Memory
	Concurrency    int
	Queue          workqueue.TypedDelayingInterface[uuid.UUID]
	running        atomic.Bool
}

func NewAgent(opts ...AgentOption) *Agent {
	options := DefaultAgentOptions()
	for _, opt := range opts {
		opt(options)
	}
	toolbox := tool.NewToolbox()
	for _, tool := range options.Tools {
		toolbox.AddTool(tool)
	}

	queue := workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[uuid.UUID]{
		Name: "construct",
	})

	return &Agent{
		ModelProviders: options.ModelProviders,
		SystemPrompt:   options.SystemPrompt,
		Toolbox:        toolbox,
		Mailbox:        NewMailbox(),
		SystemMemory:   options.SystemMemory,
		Concurrency:    options.Concurrency,
		Queue:          queue,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	if !a.running.CompareAndSwap(false, true) {
		return nil
	}

	var wg sync.WaitGroup
	for range a.Concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				taskID, shutdown := a.Queue.Get()
				if shutdown {
					return
				}
				a.processTask(ctx, taskID)
			}
		}()
	}
	wg.Wait()
	return nil
}

func (a *Agent) processTask(ctx context.Context, taskID uuid.UUID) error {
	defer a.Queue.Done(taskID)

	messages := a.Mailbox.Dequeue(taskID)






	for _, mp := range a.ModelProviders {
		resp, err := mp.InvokeModel(ctx, uuid.MustParse("0195b4e2-45b6-76df-b208-f48b7b0d5f51"), ConstructSystemPrompt, []model.Message{
			{
				Source: model.MessageSourceUser,
				Content: []model.ContentBlock{
					&model.TextContentBlock{
						Text: "Hello, how are you? Please write at least 200 words and then read the file /etc/passwd",
					},
				},
			},
		}, model.WithStreamHandler(func(ctx context.Context, message *model.Message) {
			for _, block := range message.Content {
				switch block := block.(type) {
				case *model.TextContentBlock:
					fmt.Print(block.Text)
				}
			}
		}), model.WithTools(tool.FilesystemTools()))

		if err != nil {
			return err
		}

		for _, block := range resp.Message.Content {
			switch block := block.(type) {
			case *model.TextContentBlock:
				fmt.Print(block.Text)
			case *model.ToolCallContentBlock:
				fmt.Println(block.Name)
				fmt.Println(string(block.Input))
			}
		}

		fmt.Println(resp.Usage)
	}
	return nil
}


func(a *Agent) SendMessage(taskID uuid.UUID, message string) {
	a.Mailbox.Enqueue(taskID, message)
}

func (a *Agent) CreateTask() uuid.UUID {
	return uuid.New()
}

