package cmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/shared/mocks"
	"github.com/furisto/construct/shared/conv"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
	"go.uber.org/mock/gomock"
)

type MockFormatter struct {
	DisplayedObjects any
	DisplayFormat    *RenderOptions
}

func (m *MockFormatter) Render(resources any, options *RenderOptions) error {
	m.DisplayedObjects = resources
	m.DisplayFormat = options
	return nil
}

var _ OutputRenderer = (*MockFormatter)(nil)

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
	Stdout           *string
	Error            string
	DisplayedObjects any
	DisplayFormat    *RenderOptions
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
			setupDefaultUserInfo(userInfo)
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
			ctx = context.WithValue(ctx, ContextKeyDisableFileLogs, true)
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
			if scenario.Expected.DisplayFormat != nil {
				actual.DisplayFormat = mockFormatter.DisplayFormat
			}

			if scenario.Expected.Stdout != nil {
				actual.Stdout = conv.Ptr(stdout.String())
			}

			if diff := cmp.Diff(scenario.Expected, actual, s.CmpOptions...); diff != "" {
				t.Errorf("%s() mismatch (-want +got):\n%s", scenario.Name, diff)
			}
		})
	}
}

func setupModelNameLookup(mockClient *api_client.MockClient, modelName, modelID string) {
	mockClient.Model.EXPECT().GetModel(
		gomock.Any(),
		&connect.Request[v1.GetModelRequest]{
			Msg: &v1.GetModelRequest{Id: modelID},
		},
	).Return(&connect.Response[v1.GetModelResponse]{
		Msg: &v1.GetModelResponse{
			Model: &v1.Model{
				Metadata: &v1.ModelMetadata{
					Id: modelID,
				},
				Spec: &v1.ModelSpec{
					Name: modelName,
				},
			},
		},
	}, nil)
}

func setupDefaultUserInfo(userInfo *mocks.MockUserInfo) {
	userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
}

// CmpMatcher is a gomock matcher that uses cmp.Equal for comparison
type CmpMatcher struct {
	expected interface{}
	opts     []cmp.Option
}

// CmpEqual returns a matcher that uses cmp.Equal with the given options
func CmpEqual(expected interface{}, opts ...cmp.Option) gomock.Matcher {
	return &CmpMatcher{
		expected: expected,
		opts:     opts,
	}
}

func (m *CmpMatcher) Matches(actual interface{}) bool {
	return cmp.Equal(m.expected, actual, m.opts...)
}

func (m *CmpMatcher) String() string {
	return fmt.Sprintf("cmp.Equal(%v)", m.expected)
}

func (m *CmpMatcher) Got(got interface{}) string {
	diff := cmp.Diff(m.expected, got, m.opts...)
	if diff == "" {
		return fmt.Sprintf("%v (matches)", got)
	}
	return fmt.Sprintf("%v\ndiff:\n%s", got, diff)
}
