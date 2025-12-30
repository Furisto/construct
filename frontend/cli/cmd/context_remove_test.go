package cmd

import (
	"testing"

	api "github.com/furisto/construct/api/go/client"
	"github.com/spf13/afero"
)

func TestContextRemove(t *testing.T) {
	setup := &TestSetup{}

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - remove single context with force",
			Command: []string{"context", "remove", "staging", "--force"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					CurrentContext: "local",
					Contexts: map[string]api.EndpointContext{
						"local": {
							Address: "/home/user/.construct/construct.sock",
							Kind:    "unix",
						},
						"staging": {
							Address: "https://staging.example.com:8443",
							Kind:    "http",
						},
					},
				})
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Context \"staging\" removed\n"),
			},
		},
		{
			Name:    "success - remove multiple contexts with force",
			Command: []string{"context", "remove", "staging", "dev", "--force"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					CurrentContext: "local",
					Contexts: map[string]api.EndpointContext{
						"local": {
							Address: "/home/user/.construct/construct.sock",
							Kind:    "unix",
						},
						"staging": {
							Address: "https://staging.example.com:8443",
							Kind:    "http",
						},
						"dev": {
							Address: "https://dev.example.com:8443",
							Kind:    "http",
						},
					},
				})
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Context \"staging\" removed\nContext \"dev\" removed\n"),
			},
		},
		{
			Name:    "success - confirmation prompt respected (no)",
			Command: []string{"context", "remove", "staging"},
			Stdin:   "n\n",
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					CurrentContext: "local",
					Contexts: map[string]api.EndpointContext{
						"local": {
							Address: "/home/user/.construct/construct.sock",
							Kind:    "unix",
						},
						"staging": {
							Address: "https://staging.example.com:8443",
							Kind:    "http",
						},
					},
				})
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Are you sure you want to delete context staging? (y/n): "),
			},
		},
		{
			Name:    "success - confirmation prompt respected (yes)",
			Command: []string{"context", "remove", "staging"},
			Stdin:   "y\n",
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					CurrentContext: "local",
					Contexts: map[string]api.EndpointContext{
						"local": {
							Address: "/home/user/.construct/construct.sock",
							Kind:    "unix",
						},
						"staging": {
							Address: "https://staging.example.com:8443",
							Kind:    "http",
						},
					},
				})
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Are you sure you want to delete context staging? (y/n): Context \"staging\" removed\n"),
			},
		},
		{
			Name:    "error - context not found",
			Command: []string{"context", "remove", "nonexistent", "--force"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					CurrentContext: "local",
					Contexts: map[string]api.EndpointContext{
						"local": {
							Address: "/home/user/.construct/construct.sock",
							Kind:    "unix",
						},
					},
				})
			},
			Expected: TestExpectation{
				Error: "context \"nonexistent\" not found",
			},
		},
	})
}
