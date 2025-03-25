package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/furisto/construct/backend/agent"
	"github.com/furisto/construct/backend/api"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/toolbox"
)

func main() {
	provider, err := model.NewAnthropicProvider(os.Getenv("ANTHROPIC_API_KEY"))
	if err != nil {
		log.Fatalf("failed to create anthropic provider: %v", err)
	}

	ctx := context.Background()

	stopCh := make(chan struct{})
	agent := agent.NewAgent(
		agent.WithModelProviders([]model.ModelProvider{provider}),
		agent.WithSystemPrompt(agent.ConstructSystemPrompt),
		agent.WithSystemMemory(agent.NewEphemeralMemory()),
		agent.WithUserMemory(agent.NewEphemeralMemory()),
		agent.WithToolbox(toolbox.NewToolbox()),
	)
	go func() {
		agent.Run(ctx)
		stopCh <- struct{}{}
	}()

	handler := api.NewApiHandler(agent)
	http.ListenAndServe(":8080", handler)

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
