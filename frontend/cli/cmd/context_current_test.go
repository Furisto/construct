package cmd

import (
	"testing"

	api "github.com/furisto/construct/api/go/client"
	"github.com/spf13/afero"
)

func TestContextCurrent(t *testing.T) {
	setup := &TestSetup{}

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - display current context",
			Command: []string{"context", "current"},
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
					},
				})
			},
			Expected: TestExpectation{
				Stdout: stringPtr("local\n"),
			},
		},
		{
			Name:    "no current context",
			Command: []string{"context", "current"},
			Expected: TestExpectation{
				Error: "no current context set",
			},
		},
	})
}
