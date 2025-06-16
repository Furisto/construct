package cmd

import (
	"bytes"
	"context"
	"testing"

	api_client "github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/frontend/cli/cmd/mocks"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
	"go.uber.org/mock/gomock"
)

type MockFormatter struct {
	DisplayedObjects any
	DisplayFormat    OutputFormat
}

func (m *MockFormatter) Display(resources any, format OutputFormat) error {
	m.DisplayedObjects = resources
	m.DisplayFormat = format
	return nil
}

type TestRuntimeInfo struct {
	platform string
}

func (t *TestRuntimeInfo) GOOS() string {
	return t.platform
}

type TestSetup struct {
	CmpOptions []cmp.Option
}

type TestScenario struct {
	Name               string
	Command            []string
	Stdin              string
	SetupMocks         func(mockClient *api_client.MockClient)
	SetupFileSystem    func(fs *afero.Afero)
	SetupEnv           map[string]string
	SetupCommandRunner func(commandRunner *mocks.MockCommandRunner)
	SetupUserInfo      func(userInfo *mocks.MockUserInfo)
	Platform           string
	Expected           TestExpectation
}

type TestExpectation struct {
	Stdout           string
	Error            string
	DisplayedObjects any
	DisplayFormat    OutputFormat
}

func (s *TestSetup) RunTests(t *testing.T, scenarios []TestScenario) {
	if len(scenarios) == 0 {
		t.Fatalf("no scenarios provided")
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := api_client.NewMockClient(ctrl)
			if scenario.SetupMocks != nil {
				scenario.SetupMocks(mockClient)
			}

			commandRunner := mocks.NewMockCommandRunner(ctrl)
			if scenario.SetupCommandRunner != nil {
				scenario.SetupCommandRunner(commandRunner)
			}

			userInfo := mocks.NewMockUserInfo(ctrl)
			if scenario.SetupUserInfo != nil {
				scenario.SetupUserInfo(userInfo)
			}

			fs := &afero.Afero{Fs: afero.NewMemMapFs()}
			if scenario.SetupFileSystem != nil {
				scenario.SetupFileSystem(fs)
			}

			for key, value := range scenario.SetupEnv {
				t.Setenv(key, value)
			}

			testCmd := NewRootCmd()

			var stdin bytes.Buffer
			if scenario.Stdin != "" {
				stdin.WriteString(scenario.Stdin)
				testCmd.SetIn(&stdin)
			}

			var stdout bytes.Buffer
			testCmd.SetOut(&stdout)
			testCmd.SetErr(&stdout)

			mockFormatter := &MockFormatter{}
			ctx := context.Background()
			ctx = context.WithValue(ctx, ContextKeyAPIClient, mockClient.Client())
			ctx = context.WithValue(ctx, ContextKeyFileSystem, fs)
			ctx = context.WithValue(ctx, ContextKeyOutputRenderer, mockFormatter)
			ctx = context.WithValue(ctx, ContextKeyCommandRunner, commandRunner)
			ctx = context.WithValue(ctx, ContextKeyUserInfo, userInfo)
			
			// Default to Linux platform, can be overridden
			platform := "linux"
			if scenario.Platform != "" {
				platform = scenario.Platform
			}
			runtimeInfo := &TestRuntimeInfo{platform: platform}
			ctx = context.WithValue(ctx, ContextKeyRuntimeInfo, runtimeInfo)
			

			testCmd.SetArgs(scenario.Command)

			var actual TestExpectation
			err := testCmd.ExecuteContext(ctx)
			if err != nil {
				actual.Error = err.Error()
			}

			actual.DisplayedObjects = mockFormatter.DisplayedObjects
			actual.DisplayFormat = mockFormatter.DisplayFormat
			actual.Stdout = stdout.String()

			if diff := cmp.Diff(scenario.Expected, actual, s.CmpOptions...); diff != "" {
				t.Errorf("%s() mismatch (-want +got):\n%s", scenario.Name, diff)
			}
		})
	}
}
