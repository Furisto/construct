package cmd

import (
	"testing"

	api "github.com/furisto/construct/api/go/client"
	"github.com/spf13/afero"
)

func TestContextUse(t *testing.T) {
	setup := &TestSetup{}

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - switch to existing context",
			Command: []string{"context", "use", "staging"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					CurrentContext: "local",
					Contexts: map[string]api.EndpointContext{
						"local": {
							Address: "/home/user/.construct/construct.sock",
							Kind:    "unix",
						},
						"staging": {
							Address: "https://construct.staging.example.com:8443",
							Kind:    "http",
						},
					},
				})
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Switched to context \"staging\"\n"),
			},
		},
		{
			Name:    "error - context not found",
			Command: []string{"context", "use", "nonexistent"},
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
				Error: "context nonexistent not found",
			},
		},
		{
			Name:    "error - missing argument",
			Command: []string{"context", "use"},
			Expected: TestExpectation{
				Error: "accepts 1 arg(s), received 0",
			},
		},
		{
			Name:    "success - switch to previous context",
			Command: []string{"context", "use", "-"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					CurrentContext:  "local",
					PreviousContext: "staging",
					Contexts: map[string]api.EndpointContext{
						"local": {
							Address: "/home/user/.construct/construct.sock",
							Kind:    "unix",
						},
						"staging": {
							Address: "https://construct.staging.example.com:8443",
							Kind:    "http",
						},
					},
				})
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Switched to context \"staging\"\n"),
			},
		},
	})
}
