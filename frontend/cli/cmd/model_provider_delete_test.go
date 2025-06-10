package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestModelProviderDelete(t *testing.T) {
	setup := &TestSetup{}

	modelProviderID1 := uuid.New().String()
	modelProviderID2 := uuid.New().String()
	modelID1 := uuid.New().String()
	modelID2 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - delete single model provider by name with force flag",
			Command: []string{"modelprovider", "delete", "--force", "anthropic-dev"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForDeleteMock(mockClient, "anthropic-dev", modelProviderID1)
				setupModelListForDeleteMock(mockClient, modelProviderID1, []*v1.Model{
					{Id: modelID1, Name: "claude-3-5-sonnet"},
				})
				setupModelDeleteMock(mockClient, modelID1)
				setupModelProviderDeleteMock(mockClient, modelProviderID1)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete single model provider by ID with force flag",
			Command: []string{"modelprovider", "delete", "--force", modelProviderID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelListForDeleteMock(mockClient, modelProviderID1, []*v1.Model{
					{Id: modelID1, Name: "gpt-4"},
					{Id: modelID2, Name: "gpt-3.5-turbo"},
				})
				setupModelDeleteMock(mockClient, modelID1)
				setupModelDeleteMock(mockClient, modelID2)
				setupModelProviderDeleteMock(mockClient, modelProviderID1)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete multiple model providers with force flag",
			Command: []string{"modelprovider", "delete", "--force", "anthropic-dev", "openai-prod"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				// Setup lookup for both providers in a single call
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
								Name:         "anthropic-dev",
								ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
								Enabled:      true,
							},
							{
								Id:           modelProviderID2,
								Name:         "openai-prod",
								ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
								Enabled:      true,
							},
						},
					},
				}, nil).Times(2) // Called once for each provider name lookup

				// Setup for first provider (anthropic-dev)
				setupModelListForDeleteMock(mockClient, modelProviderID1, []*v1.Model{
					{Id: modelID1, Name: "claude-3-5-sonnet"},
				})
				setupModelDeleteMock(mockClient, modelID1)
				setupModelProviderDeleteMock(mockClient, modelProviderID1)

				// Setup for second provider (openai-prod)
				setupModelListForDeleteMock(mockClient, modelProviderID2, []*v1.Model{
					{Id: modelID2, Name: "gpt-4"},
				})
				setupModelDeleteMock(mockClient, modelID2)
				setupModelProviderDeleteMock(mockClient, modelProviderID2)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete model provider with no models with force flag",
			Command: []string{"modelprovider", "delete", "--force", "empty-provider"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForDeleteMock(mockClient, "empty-provider", modelProviderID1)
				setupModelListForDeleteMock(mockClient, modelProviderID1, []*v1.Model{})
				setupModelProviderDeleteMock(mockClient, modelProviderID1)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete model provider with user confirmation",
			Command: []string{"modelprovider", "delete", "anthropic-dev"},
			Stdin:   "y\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForDeleteMock(mockClient, "anthropic-dev", modelProviderID1)
				setupModelListForDeleteMock(mockClient, modelProviderID1, []*v1.Model{
					{Id: modelID1, Name: "claude-3-5-sonnet"},
				})
				setupModelDeleteMock(mockClient, modelID1)
				setupModelProviderDeleteMock(mockClient, modelProviderID1)
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete model-provider anthropic-dev? (y/n): ",
			},
		},
		{
			Name:    "success - cancel deletion when user denies confirmation",
			Command: []string{"modelprovider", "delete", "anthropic-dev"},
			Stdin:   "n\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForDeleteMock(mockClient, "anthropic-dev", modelProviderID1)
				// No delete mocks needed since operation should be cancelled
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete model-provider anthropic-dev? (y/n): ",
			},
		},
		{
			Name:    "success - delete multiple model providers with user confirmation",
			Command: []string{"modelprovider", "delete", "anthropic-dev", "openai-prod"},
			Stdin:   "y\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				// Setup lookup for both providers in a single call
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
								Name:         "anthropic-dev",
								ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
								Enabled:      true,
							},
							{
								Id:           modelProviderID2,
								Name:         "openai-prod",
								ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
								Enabled:      true,
							},
						},
					},
				}, nil).Times(2) // Called once for each provider name lookup

				// Setup for first provider (anthropic-dev)
				setupModelListForDeleteMock(mockClient, modelProviderID1, []*v1.Model{
					{Id: modelID1, Name: "claude-3-5-sonnet"},
				})
				setupModelDeleteMock(mockClient, modelID1)
				setupModelProviderDeleteMock(mockClient, modelProviderID1)

				// Setup for second provider (openai-prod)
				setupModelListForDeleteMock(mockClient, modelProviderID2, []*v1.Model{
					{Id: modelID2, Name: "gpt-4"},
				})
				setupModelDeleteMock(mockClient, modelID2)
				setupModelProviderDeleteMock(mockClient, modelProviderID2)
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete model-providers anthropic-dev openai-prod? (y/n): ",
			},
		},
		{
			Name:    "error - model provider not found by name with force flag",
			Command: []string{"modelprovider", "delete", "--force", "nonexistent"},
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
			Name:    "error - model deletion API failure with force flag",
			Command: []string{"modelprovider", "delete", "--force", "anthropic-dev"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForDeleteMock(mockClient, "anthropic-dev", modelProviderID1)
				setupModelListForDeleteMock(mockClient, modelProviderID1, []*v1.Model{
					{Id: modelID1, Name: "claude-3-5-sonnet"},
				})
				mockClient.Model.EXPECT().DeleteModel(
					gomock.Any(),
					&connect.Request[v1.DeleteModelRequest]{
						Msg: &v1.DeleteModelRequest{Id: modelID1},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to delete model claude-3-5-sonnet for model provider anthropic-dev: internal",
			},
		},
		{
			Name:    "error - model provider deletion API failure with force flag",
			Command: []string{"modelprovider", "delete", "--force", "anthropic-dev"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForDeleteMock(mockClient, "anthropic-dev", modelProviderID1)
				setupModelListForDeleteMock(mockClient, modelProviderID1, []*v1.Model{
					{Id: modelID1, Name: "claude-3-5-sonnet"},
				})
				setupModelDeleteMock(mockClient, modelID1)
				mockClient.ModelProvider.EXPECT().DeleteModelProvider(
					gomock.Any(),
					&connect.Request[v1.DeleteModelProviderRequest]{
						Msg: &v1.DeleteModelProviderRequest{Id: modelProviderID1},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to delete model provider anthropic-dev: internal",
			},
		},
	})
}

func setupModelProviderLookupForDeleteMock(mockClient *api_client.MockClient, modelProviderName, modelProviderID string) {
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

func setupModelListForDeleteMock(mockClient *api_client.MockClient, modelProviderID string, models []*v1.Model) {
	mockClient.Model.EXPECT().ListModels(
		gomock.Any(),
		&connect.Request[v1.ListModelsRequest]{
			Msg: &v1.ListModelsRequest{
				Filter: &v1.ListModelsRequest_Filter{
					ModelProviderId: &modelProviderID,
				},
			},
		},
	).Return(&connect.Response[v1.ListModelsResponse]{
		Msg: &v1.ListModelsResponse{
			Models: models,
		},
	}, nil)
}

func setupModelDeleteMock(mockClient *api_client.MockClient, modelID string) {
	mockClient.Model.EXPECT().DeleteModel(
		gomock.Any(),
		&connect.Request[v1.DeleteModelRequest]{
			Msg: &v1.DeleteModelRequest{Id: modelID},
		},
	).Return(&connect.Response[v1.DeleteModelResponse]{}, nil)
}

func setupModelProviderDeleteMock(mockClient *api_client.MockClient, modelProviderID string) {
	mockClient.ModelProvider.EXPECT().DeleteModelProvider(
		gomock.Any(),
		&connect.Request[v1.DeleteModelProviderRequest]{
			Msg: &v1.DeleteModelProviderRequest{Id: modelProviderID},
		},
	).Return(&connect.Response[v1.DeleteModelProviderResponse]{}, nil)
}
