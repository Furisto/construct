package cmd

import (
	"testing"

	api "github.com/furisto/construct/api/go/client"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

func setupContextFile(t *testing.T, fs *afero.Afero, contexts *api.EndpointContexts) {
	t.Helper()

	content, err := yaml.Marshal(contexts)
	if err != nil {
		t.Fatalf("failed to marshal contexts: %v", err)
	}

	fs.MkdirAll("/home/user/.construct", 0700)
	err = fs.WriteFile("/home/user/.construct/context.yaml", content, 0600)
	if err != nil {
		t.Fatalf("failed to write context file: %v", err)
	}
}

func stringPtr(s string) *string {
	return &s
}
