package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"

	"github.com/furisto/construct/backend/agent"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/tool"
)

func main() {
	provider, err := model.NewAnthropicProvider("")
	if err != nil {
		log.Fatalf("failed to create anthropic provider: %v", err)
	}

	ctx := context.Background()

	stopCh := make(chan struct{})
	agent := agent.NewAgent(
		agent.WithModelProviders(provider),
		agent.WithSystemPrompt(agent.ConstructSystemPrompt),
		agent.WithSystemMemory(agent.NewEphemeralMemory()),
		agent.WithUserMemory(agent.NewEphemeralMemory()),
		agent.WithTools(
			tool.FilesystemTools()...,
		),
	)

	go func() {
		err := agent.Run(ctx)
		fmt.Println("agent stopped")
		if err != nil {
			slog.Error("failed to run agent", "error", err)
		}
		stopCh <- struct{}{}
	}()

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
