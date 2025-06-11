package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestModelList(t *testing.T) {
	setup := &TestSetup{}

	modelID1 := uuid.New().String()
	modelID2 := uuid.New().String()
	modelProviderID1 := uuid.New().String()
	modelProviderID2 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - list enabled models",
			Command: []string{"model", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				enabled := true
				setupModelListMock(mockClient, nil, &enabled, []*v1.Model{
					createTestModel(modelID1, "gpt-4", modelProviderID1, 8192, true),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelDisplay{
					{
						Id:            modelID1,
						Name:          "gpt-4",
						ModelProvider: modelProviderID1,
						ContextWindow: 8192,
						Enabled:       true,
						Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
					},
				},
			},
		},
		{
			Name:    "success - list models filtered by provider name",
			Command: []string{"model", "list", "--model-provider", "openai-dev"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForListMock(mockClient, "openai-dev", modelProviderID1)
				enabled := true
				setupModelListMock(mockClient, &modelProviderID1, &enabled, []*v1.Model{
					createTestModel(modelID1, "gpt-4", modelProviderID1, 8192, true),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelDisplay{
					{
						Id:            modelID1,
						Name:          "gpt-4",
						ModelProvider: modelProviderID1,
						ContextWindow: 8192,
						Enabled:       true,
						Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
					},
				},
			},
		},
		{
			Name:    "success - list models filtered by provider ID",
			Command: []string{"model", "list", "--model-provider", modelProviderID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				enabled := true
				setupModelListMock(mockClient, &modelProviderID1, &enabled, []*v1.Model{
					createTestModel(modelID1, "gpt-4", modelProviderID1, 8192, true),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelDisplay{
					{
						Id:            modelID1,
						Name:          "gpt-4",
						ModelProvider: modelProviderID1,
						ContextWindow: 8192,
						Enabled:       true,
						Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
					},
				},
			},
		},

		{
			Name:    "success - list all models including disabled ones",
			Command: []string{"model", "list", "--show-disabled"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelListMock(mockClient, nil, nil, []*v1.Model{
					createTestModel(modelID1, "gpt-4", modelProviderID1, 8192, true),
					createTestModel(modelID2, "claude-3-5-sonnet", modelProviderID2, 200000, false),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelDisplay{
					{
						Id:            modelID1,
						Name:          "gpt-4",
						ModelProvider: modelProviderID1,
						ContextWindow: 8192,
						Enabled:       true,
						Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
					},
					{
						Id:            modelID2,
						Name:          "claude-3-5-sonnet",
						ModelProvider: modelProviderID2,
						ContextWindow: 200000,
						Enabled:       false,
						Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
					},
				},
			},
		},
		{
			Name:    "success - list models with JSON output",
			Command: []string{"model", "list", "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				enabled := true
				setupModelListMock(mockClient, nil, &enabled, []*v1.Model{
					createTestModel(modelID1, "gpt-4", modelProviderID1, 8192, true),
				})
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatJSON,
				DisplayedObjects: []*ModelDisplay{
					{
						Id:            modelID1,
						Name:          "gpt-4",
						ModelProvider: modelProviderID1,
						ContextWindow: 8192,
						Enabled:       true,
						Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
					},
				},
			},
		},
		{
			Name:    "success - list models with short flags",
			Command: []string{"model", "list", "-p", "anthropic-dev", "-d", "-o", "yaml"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderLookupForListMock(mockClient, "anthropic-dev", modelProviderID2)
				setupModelListMock(mockClient, &modelProviderID2, nil, []*v1.Model{
					createTestModel(modelID2, "claude-3-5-sonnet", modelProviderID2, 200000, true),
				})
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatYAML,
				DisplayedObjects: []*ModelDisplay{
					{
						Id:            modelID2,
						Name:          "claude-3-5-sonnet",
						ModelProvider: modelProviderID2,
						ContextWindow: 200000,
						Enabled:       true,
						Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
					},
				},
			},
		},
		{
			Name:    "success - empty model list",
			Command: []string{"model", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				enabled := true
				setupModelListMock(mockClient, nil, &enabled, []*v1.Model{})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelDisplay{},
			},
		},
		{
			Name:    "error - model provider not found by name",
			Command: []string{"model", "list", "--model-provider", "nonexistent"},
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
			Name:    "error - list models API failure",
			Command: []string{"model", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				enabled := true
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Enabled: &enabled,
							},
						},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to list models: internal",
			},
		},
	})
}

func setupModelListMock(mockClient *api_client.MockClient, modelProviderID *string, enabled *bool, models []*v1.Model) {
	filter := &v1.ListModelsRequest_Filter{}
	if modelProviderID != nil {
		filter.ModelProviderId = modelProviderID
	}
	if enabled != nil {
		filter.Enabled = enabled
	}

	mockClient.Model.EXPECT().ListModels(
		gomock.Any(),
		&connect.Request[v1.ListModelsRequest]{
			Msg: &v1.ListModelsRequest{
				Filter: filter,
			},
		},
	).Return(&connect.Response[v1.ListModelsResponse]{
		Msg: &v1.ListModelsResponse{
			Models: models,
		},
	}, nil)
}

func setupModelProviderLookupForListMock(mockClient *api_client.MockClient, providerName, providerID string) {
	mockClient.ModelProvider.EXPECT().ListModelProviders(
		gomock.Any(),
		&connect.Request[v1.ListModelProvidersRequest]{
			Msg: &v1.ListModelProvidersRequest{},
		},
	).Return(&connect.Response[v1.ListModelProvidersResponse]{
		Msg: &v1.ListModelProvidersResponse{
			ModelProviders: []*v1.ModelProvider{
				{
					Id:           providerID,
					Name:         providerName,
					ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
					Enabled:      true,
				},
			},
		},
	}, nil)
}

func createTestModel(modelID, name, modelProviderID string, contextWindow int64, enabled bool) *v1.Model {
	return &v1.Model{
		Id:              modelID,
		Name:            name,
		ModelProviderId: modelProviderID,
		ContextWindow:   contextWindow,
		Enabled:         enabled,
		Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_IMAGE},
	}
}
