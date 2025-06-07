package cmd

import (
	"bytes"
	"context"
	"testing"

	api_client "github.com/furisto/construct/api/go/client"
	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"
)

type TestSetup struct {
	CmpOptions []cmp.Option
}

type TestScenario struct {
	Name       string
	Command    []string
	Stdin      string
	SetupMocks func(mockClient *api_client.MockClient)
	Expected   TestExpectation
}

type TestExpectation struct {
	Stdout string
	Error  string
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

			var stdin bytes.Buffer
			if scenario.Stdin != "" {
				stdin.WriteString(scenario.Stdin)
				rootCmd.SetIn(&stdin)
			}

			var stdout bytes.Buffer
			rootCmd.SetOut(&stdout)
			rootCmd.SetErr(&stdout)

			rootCmd.SetArgs(scenario.Command)

			var actual TestExpectation
			ctx := context.Background()
			ctx = context.WithValue(ctx, "api_test_client", mockClient.Client())
			err := rootCmd.ExecuteContext(ctx)
			if err != nil {
				actual.Error = err.Error()
			} else {
				actual.Stdout = stdout.String()
			}

			if diff := cmp.Diff(scenario.Expected, actual, s.CmpOptions...); diff != "" {
				t.Errorf("%s() mismatch (-want +got):\n%s", scenario.Name, diff)
			}
		})
	}
}
