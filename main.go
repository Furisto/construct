package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/furisto/construct/backend/modelprovider"
)

func main() {
	provider, err := modelprovider.NewAnthropicProvider(os.Getenv("ANTHROPIC_API_KEY"))
	if err != nil {
		log.Fatalf("failed to create anthropic provider: %v", err)
	}

	_, err = provider.ListModels(context.Background())
	if err != nil {
		log.Fatalf("failed to list anthropic models: %v", err)
	}

	

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
