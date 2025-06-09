package cmd

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestModelProviderCreate(t *testing.T) {
	setup := &TestSetup{}

	providerID := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success with API key flag",
			Command: []string{"modelprovider", "create", "my-openai", "--type", "openai", "--api-key", "sk-test123"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderCreationMock(mockClient, "my-openai", v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, "sk-test123", providerID)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(providerID),
			},
		},
		{
			Name:    "success with environment variable",
			Command: []string{"modelprovider", "create", "my-anthropic-env", "--type", "anthropic"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderCreationMock(mockClient, "my-anthropic-env", v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC, "sk-ant-env-test123", providerID)
			},
			SetupEnv: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-env-test123",
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(providerID),
			},
		},
		{
			Name:    "success - API key flag takes precedence over environment variable",
			Command: []string{"modelprovider", "create", "my-openai", "--type", "openai", "--api-key", "sk-flag-test123"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderCreationMock(mockClient, "my-openai", v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, "sk-flag-test123", providerID)
			},
			SetupEnv: map[string]string{
				"OPENAI_API_KEY": "sk-env-should-not-be-used",
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(providerID),
			},
		},
		{
			Name:    "error - missing provider type",
			Command: []string{"modelprovider", "create", "my-provider"},
			Expected: TestExpectation{
				Error: "required flag(s) \"type\" not set",
			},
		},
		{
			Name:    "error - missing provider name",
			Command: []string{"modelprovider", "create", "--type", "openai"},
			Expected: TestExpectation{
				Error: "accepts 1 arg(s), received 0",
			},
		},
		{
			Name:    "error - invalid provider type",
			Command: []string{"modelprovider", "create", "my-provider", "--type", "invalid"},
			Expected: TestExpectation{
				Error: "invalid argument \"invalid\" for \"-t, --type\" flag: must be one of \"openai\" or \"anthropic\"",
			},
		},
		{
			Name:    "error - API creation fails",
			Command: []string{"modelprovider", "create", "my-openai", "--type", "openai", "--api-key", "sk-test123"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.ModelProvider.EXPECT().CreateModelProvider(
					gomock.Any(),
					gomock.Any(),
				).Return(nil, fmt.Errorf("provider name already exists"))
			},
			Expected: TestExpectation{
				Error: "failed to create model provider: provider name already exists",
			},
		},
	})
}

func setupModelProviderCreationMock(mockClient *api_client.MockClient, name string, providerType v1.ModelProviderType, apiKey, providerID string) {
	req := &v1.CreateModelProviderRequest{
		Name:           name,
		ProviderType:   providerType,
		Authentication: &v1.CreateModelProviderRequest_ApiKey{ApiKey: apiKey},
	}

	mockClient.ModelProvider.EXPECT().CreateModelProvider(
		gomock.Any(),
		connect.NewRequest(req),
	).Return(&connect.Response[v1.CreateModelProviderResponse]{
		Msg: &v1.CreateModelProviderResponse{
			ModelProvider: &v1.ModelProvider{
				Id:           providerID,
				Name:         name,
				ProviderType: providerType,
				Enabled:      true,
			},
		},
	}, nil)
}
