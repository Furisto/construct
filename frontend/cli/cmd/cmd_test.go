package cmd

import (
	"bytes"
	"context"
	"testing"

	api_client "github.com/furisto/construct/api/go/client"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
	"go.uber.org/mock/gomock"
)

type MockFormatter struct {
	DisplayedObjects []any
	DisplayFormat    OutputFormat
}

func (m *MockFormatter) Display(resources any, format OutputFormat) error {
	m.DisplayedObjects = append(m.DisplayedObjects, resources)
	m.DisplayFormat = format
	return nil
}

type TestSetup struct {
	CmpOptions []cmp.Option
}

type TestScenario struct {
	Name            string
	Command         []string
	Stdin           string
	SetupMocks      func(mockClient *api_client.MockClient)
	SetupFileSystem func(fs *afero.Afero)
	SetupEnv        map[string]string
	Expected        TestExpectation
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

			testCmd.SetArgs(scenario.Command)

			mockFormatter := &MockFormatter{}
			ctx := context.Background()
			ctx = context.WithValue(ctx, ContextKeyAPI, mockClient.Client())
			ctx = context.WithValue(ctx, ContextKeyFileSystem, fs)
			ctx = context.WithValue(ctx, ContextKeyFormatter, mockFormatter)

			var actual TestExpectation
			err := testCmd.ExecuteContext(ctx)
			if err != nil {
				actual.Error = err.Error()
			} else {
				actual.Stdout = stdout.String()
				if len(mockFormatter.DisplayedObjects) > 0 {
					actual.DisplayedObjects = mockFormatter.DisplayedObjects
					actual.DisplayFormat = mockFormatter.DisplayFormat
				}
			}

			if diff := cmp.Diff(scenario.Expected, actual, s.CmpOptions...); diff != "" {
				t.Errorf("%s() mismatch (-want +got):\n%s", scenario.Name, diff)
			}
		})
	}
}
