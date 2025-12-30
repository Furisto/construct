package cmd

import (
	"testing"

	api "github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/shared"
	"github.com/furisto/construct/shared/mocks"
	"github.com/spf13/afero"
	"go.uber.org/mock/gomock"
)

func TestContextAdd(t *testing.T) {
	setup := &TestSetup{}

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - add unix context with auto-detected kind",
			Command: []string{"context", "add", "local", "--endpoint", "/home/user/.construct/construct.sock"},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.MkdirAll("/home/user/.construct", 0700)
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Context \"local\" created\n"),
			},
		},
		{
			Name:    "success - add http context with auto-detected kind",
			Command: []string{"context", "add", "production", "--endpoint", "https://construct.prod.example.com:8443"},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.MkdirAll("/home/user/.construct", 0700)
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Context \"production\" created\n"),
			},
		},
		{
			Name:    "success - add with explicit kind",
			Command: []string{"context", "add", "dev", "--endpoint", "https://localhost:8443", "--kind", "http"},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.MkdirAll("/home/user/.construct", 0700)
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Context \"dev\" created\n"),
			},
		},
		{
			Name:    "success - add with set-current",
			Command: []string{"context", "add", "staging", "--endpoint", "https://staging.example.com:8443", "--set-current"},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.MkdirAll("/home/user/.construct", 0700)
			},
			Expected: TestExpectation{
				Stdout: stringPtr("Context \"staging\" created and set as current\n"),
			},
		},
		{
			Name:    "success - update existing context",
			Command: []string{"context", "add", "local", "--endpoint", "/tmp/construct.sock"},
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
				Stdout: stringPtr("Context \"local\" updated\n"),
			},
		},
		{
			Name:    "error - missing endpoint flag",
			Command: []string{"context", "add", "mycontext"},
			Expected: TestExpectation{
				Error: "required flag(s) \"endpoint\" not set",
			},
		},
	})
}

func TestContextAddWithAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKeyring := mocks.NewMockProvider(ctrl)
	mockKeyring.EXPECT().Set("production", "secret-token-123").Return(nil)

	userInfo := mocks.NewMockUserInfo(ctrl)
	setupDefaultUserInfo(userInfo)

	fs := &afero.Afero{Fs: afero.NewMemMapFs()}
	fs.MkdirAll("/home/user/.construct", 0700)

	contextManager := shared.NewContextManagerWithKeyring(fs, userInfo, mockKeyring)

	_, err := contextManager.UpsertContext(
		"production",
		"http",
		"https://construct.prod.example.com:8443",
		false,
		&api.AuthConfig{
			Type:     api.AuthTypeToken,
			TokenRef: api.KeyringRefPrefix + "production",
		},
	)
	if err != nil {
		t.Fatalf("failed to add context with auth: %v", err)
	}

	err = contextManager.StoreToken("production", "secret-token-123")
	if err != nil {
		t.Fatalf("failed to store token: %v", err)
	}

	endpointContexts, err := contextManager.LoadContext()
	if err != nil {
		t.Fatalf("failed to load contexts: %v", err)
	}

	ctx, ok := endpointContexts.Contexts["production"]
	if !ok {
		t.Fatal("context not found")
	}

	if ctx.Auth == nil || ctx.Auth.TokenRef != api.KeyringRefPrefix+"production" {
		t.Errorf("expected token ref, got: %+v", ctx.Auth)
	}
}
