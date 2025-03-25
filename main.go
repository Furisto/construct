package main

import (
	"context"
	"fmt"
	"log/slog"

	"entgo.io/ent/dialect"
	"github.com/furisto/construct/backend/agent"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/tool"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	provider, err := model.NewAnthropicProvider("")
	if err != nil {
		slog.Error("failed to create anthropic provider", "error", err)
	}

	ctx := context.Background()
	client, err := memory.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		slog.Error("failed opening connection to sqlite", "error", err)
	}
	defer client.Close()

	if err := client.Schema.Create(ctx); err != nil {
		slog.Error("failed creating schema resources", "error", err)
	}

	stopCh := make(chan struct{})
	agent := agent.NewAgent(
		agent.WithModelProviders(provider),
		agent.WithSystemPrompt(agent.ConstructSystemPrompt),
		agent.WithMemory(client),
		// agent.WithSystemMemory(agent.NewEphemeralMemory()),
		// agent.WithUserMemory(agent.NewEphemeralMemory()),
		agent.WithTools(
			tool.FilesystemTools()...,
		),
	)

	taskID, err := agent.CreateTask(ctx)
	if err != nil {
		slog.Error("failed to create task", "error", err)
	}

	go func() {
		err := agent.Run(ctx)
		fmt.Println("agent stopped")
		if err != nil {
			slog.Error("failed to run agent", "error", err)
		}
		stopCh <- struct{}{}
	}()

	agent.SendMessage(taskID, "Hello, how are you?")

	// go func() {
	// 	handler := api.NewApiHandler(agent)
	// 	http.ListenAndServe(":8080", handler)
	// }()

	// task := agent.NewTask()
	// task.OnMessage(func(msg model.Message) {
	// 	fmt.Print(msg.Content)
	// })
	// task.SendMessage(ctx, "Hello, how are you?")

	// stream := agent.SendMessage(ctx, "Hello, how are you?")
	// stream.OnMessage(func(msg model.Message) {
	// 	fmt.Print(msg.Content)
	// })

	<-stopCh

	// openaiProvider, err := modelprovider.NewOpenAIProvider(os.Getenv("OPENAI_API_KEY"))
	// if err != nil {
	// 	log.Fatalf("failed to create openai provider: %v", err)
	// }

	// openaiModels, err := openaiProvider.ListModels(context.Background())
	// if err != nil {
	// 	log.Fatalf("failed to list openai models: %v", err)
	// }

	// for _, model := range openaiModels {
	// 	fmt.Printf("model: %v\n", model)
	// }
}
