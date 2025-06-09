package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestModelProviderGet(t *testing.T) {
	setup := &TestSetup{}

	modelProviderID1 := uuid.New().String()
	modelProviderID2 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - get model provider by name",
			Command: []string{"modelprovider", "get", "anthropic-dev"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForGetMock(mockClient, "anthropic-dev", modelProviderID1)
				setupModelProviderGetMock(mockClient, modelProviderID1, "anthropic-dev", v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC, true)
			},
			Expected: TestExpectation{
				DisplayedObjects: &ModelProviderDisplay{
					Id:           modelProviderID1,
					Name:         "anthropic-dev",
					ProviderType: ModelProviderTypeAnthropic,
					Enabled:      true,
				},
			},
		},
		{
			Name:    "success - get model provider by ID",
			Command: []string{"modelprovider", "get", modelProviderID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderGetMock(mockClient, modelProviderID1, "openai-prod", v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, true)
			},
			Expected: TestExpectation{
				DisplayedObjects: &ModelProviderDisplay{
					Id:           modelProviderID1,
					Name:         "openai-prod",
					ProviderType: ModelProviderTypeOpenAI,
					Enabled:      true,
				},
			},
		},
		{
			Name:    "success - get model provider with YAML output format",
			Command: []string{"modelprovider", "get", "openai-prod", "--output", "yaml"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForGetMock(mockClient, "openai-prod", modelProviderID1)
				setupModelProviderGetMock(mockClient, modelProviderID1, "openai-prod", v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, false)
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatYAML,
				DisplayedObjects: &ModelProviderDisplay{
					Id:           modelProviderID1,
					Name:         "openai-prod",
					ProviderType: ModelProviderTypeOpenAI,
					Enabled:      false,
				},
			},
		},
		{
			Name:    "error - model provider not found by name",
			Command: []string{"modelprovider", "get", "nonexistent"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.ModelProvider.EXPECT().ListModelProviders(
					gomock.Any(),
					&connect.Request[v1.ListModelProvidersRequest]{
						Msg: &v1.ListModelProvidersRequest{},
					},
				).Return(&connect.Response[v1.ListModelProvidersResponse]{
					Msg: &v1.ListModelProvidersResponse{
						ModelProviders: []*v1.ModelProvider{},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Error: "failed to resolve model provider nonexistent: model provider nonexistent not found",
			},
		},
		{
			Name:    "error - multiple model providers found for name",
			Command: []string{"modelprovider", "get", "duplicate"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.ModelProvider.EXPECT().ListModelProviders(
					gomock.Any(),
					&connect.Request[v1.ListModelProvidersRequest]{
						Msg: &v1.ListModelProvidersRequest{},
					},
				).Return(&connect.Response[v1.ListModelProvidersResponse]{
					Msg: &v1.ListModelProvidersResponse{
						ModelProviders: []*v1.ModelProvider{
							{
								Id:           modelProviderID1,
								Name:         "duplicate",
								ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
								Enabled:      true,
							},
							{
								Id:           modelProviderID2,
								Name:         "duplicate",
								ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
								Enabled:      true,
							},
						},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Error: "failed to resolve model provider duplicate: multiple model providers found for duplicate",
			},
		},
		{
			Name:    "error - get model provider API failure",
			Command: []string{"modelprovider", "get", modelProviderID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.ModelProvider.EXPECT().GetModelProvider(
					gomock.Any(),
					&connect.Request[v1.GetModelProviderRequest]{
						Msg: &v1.GetModelProviderRequest{Id: modelProviderID1},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to get model provider " + modelProviderID1 + ": internal",
			},
		},
	})
}

func setupModelProviderLookupForGetMock(mockClient *api_client.MockClient, modelProviderName, modelProviderID string) {
	mockClient.ModelProvider.EXPECT().ListModelProviders(
		gomock.Any(),
		&connect.Request[v1.ListModelProvidersRequest]{
			Msg: &v1.ListModelProvidersRequest{},
		},
	).Return(&connect.Response[v1.ListModelProvidersResponse]{
		Msg: &v1.ListModelProvidersResponse{
			ModelProviders: []*v1.ModelProvider{
				{
					Id:           modelProviderID,
					Name:         modelProviderName,
					ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
					Enabled:      true,
				},
			},
		},
	}, nil)
}

func setupModelProviderGetMock(mockClient *api_client.MockClient, modelProviderID, name string, providerType v1.ModelProviderType, enabled bool) {
	mockClient.ModelProvider.EXPECT().GetModelProvider(
		gomock.Any(),
		&connect.Request[v1.GetModelProviderRequest]{
			Msg: &v1.GetModelProviderRequest{Id: modelProviderID},
		},
	).Return(&connect.Response[v1.GetModelProviderResponse]{
		Msg: &v1.GetModelProviderResponse{
			ModelProvider: &v1.ModelProvider{
				Id:           modelProviderID,
				Name:         name,
				ProviderType: providerType,
				Enabled:      enabled,
			},
		},
	}, nil)
}
