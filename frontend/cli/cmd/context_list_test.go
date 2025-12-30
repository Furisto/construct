package cmd

import (
	"testing"

	api "github.com/furisto/construct/api/go/client"
	"github.com/spf13/afero"
)

func TestContextList(t *testing.T) {
	setup := &TestSetup{}

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - list multiple contexts with current marker",
			Command: []string{"context", "list"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					CurrentContext: "local",
					Contexts: map[string]api.EndpointContext{
						"local": {
							Address: "/home/user/.construct/construct.sock",
							Kind:    "unix",
						},
						"ec2-dev": {
							Address: "https://construct.dev.internal:8443",
							Kind:    "http",
							Auth: &api.AuthConfig{
								Type:     api.AuthTypeToken,
								TokenRef: api.KeyringRefPrefix + "ec2-dev",
							},
						},
						"staging": {
							Address: "https://construct.staging.example.com:8443",
							Kind:    "http",
							Auth: &api.AuthConfig{
								Type:  api.AuthTypeToken,
								Token: "inline-token-123",
							},
						},
					},
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ContextDisplay{
					{
						Name:     "ec2-dev",
						Endpoint: "https://construct.dev.internal:8443",
						Kind:     "http",
						Auth:     "keyring://construct/ec2-dev",
						Current:  false,
					},
					{
						Name:     "local",
						Endpoint: "/home/user/.construct/construct.sock",
						Kind:     "unix",
						Auth:     "none",
						Current:  true,
					},
					{
						Name:     "staging",
						Endpoint: "https://construct.staging.example.com:8443",
						Kind:     "http",
						Auth:     "token (inline)",
						Current:  false,
					},
				},
			},
		},
		{
			Name:    "success - list with JSON output",
			Command: []string{"context", "list", "--output", "json"},
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
				DisplayFormat: &RenderOptions{
					Format: OutputFormatJSON,
				},
				DisplayedObjects: []*ContextDisplay{
					{
						Name:     "local",
						Endpoint: "/home/user/.construct/construct.sock",
						Kind:     "unix",
						Auth:     "none",
						Current:  true,
					},
				},
			},
		},
		{
			Name:    "success - empty context list",
			Command: []string{"context", "list"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupContextFile(t, fs, &api.EndpointContexts{
					Contexts: map[string]api.EndpointContext{},
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ContextDisplay{},
			},
		},
	})
}
