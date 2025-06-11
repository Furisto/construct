package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestModelGet(t *testing.T) {
	setup := &TestSetup{}

	modelID1 := uuid.New().String()
	modelProviderID1 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - get model by name",
			Command: []string{"model", "get", "gpt-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForGetMock(mockClient, "gpt-4", modelID1)
				setupModelGetMock(mockClient, modelID1, "gpt-4", modelProviderID1, 8192, true)
			},
			Expected: TestExpectation{
				DisplayedObjects: &ModelDisplay{
					Id:            modelID1,
					Name:          "gpt-4",
					ModelProvider: modelProviderID1,
					ContextWindow: 8192,
					Enabled:       true,
					Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
				},
			},
		},
		{
			Name:    "success - get model by ID",
			Command: []string{"model", "get", modelID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelGetMock(mockClient, modelID1, "claude-3-5-sonnet", modelProviderID1, 200000, false)
			},
			Expected: TestExpectation{
				DisplayedObjects: &ModelDisplay{
					Id:            modelID1,
					Name:          "claude-3-5-sonnet",
					ModelProvider: modelProviderID1,
					ContextWindow: 200000,
					Enabled:       false,
					Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
				},
			},
		},
		{
			Name:    "success - get model with JSON output",
			Command: []string{"model", "get", "llama-3.1-8b", "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForGetMock(mockClient, "llama-3.1-8b", modelID1)
				setupModelGetMock(mockClient, modelID1, "llama-3.1-8b", modelProviderID1, 32768, true)
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatJSON,
				DisplayedObjects: &ModelDisplay{
					Id:            modelID1,
					Name:          "llama-3.1-8b",
					ModelProvider: modelProviderID1,
					ContextWindow: 32768,
					Enabled:       true,
					Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
				},
			},
		},
		{
			Name:    "success - get model with YAML output",
			Command: []string{"model", "get", "claude-3-5-sonnet", "--output", "yaml"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForGetMock(mockClient, "claude-3-5-sonnet", modelID1)
				setupModelGetMock(mockClient, modelID1, "claude-3-5-sonnet", modelProviderID1, 200000, true)
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatYAML,
				DisplayedObjects: &ModelDisplay{
					Id:            modelID1,
					Name:          "claude-3-5-sonnet",
					ModelProvider: modelProviderID1,
					ContextWindow: 200000,
					Enabled:       true,
					Capabilities:  []string{"MODEL_CAPABILITY_IMAGE"},
				},
			},
		},
		{
			Name:    "error - model not found by name",
			Command: []string{"model", "get", "nonexistent"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("nonexistent"),
							},
						},
					},
				).Return(&connect.Response[v1.ListModelsResponse]{
					Msg: &v1.ListModelsResponse{
						Models: []*v1.Model{},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Error: "failed to resolve model nonexistent: model nonexistent not found",
			},
		},
		{
			Name:    "error - get model API failure",
			Command: []string{"model", "get", modelID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().GetModel(
					gomock.Any(),
					&connect.Request[v1.GetModelRequest]{
						Msg: &v1.GetModelRequest{Id: modelID1},
					},
				).Return(nil, connect.NewError(connect.CodeNotFound, nil))
			},
			Expected: TestExpectation{
				Error: "failed to get model " + modelID1 + ": not_found",
			},
		},
		{
			Name:    "error - model lookup API failure",
			Command: []string{"model", "get", "gpt-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("gpt-4"),
							},
						},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to resolve model gpt-4: failed to list models: internal",
			},
		},
	})
}

func setupModelLookupForGetMock(mockClient *api_client.MockClient, modelName, modelID string) {
	mockClient.Model.EXPECT().ListModels(
		gomock.Any(),
		&connect.Request[v1.ListModelsRequest]{
			Msg: &v1.ListModelsRequest{
				Filter: &v1.ListModelsRequest_Filter{
					Name: api_client.Ptr(modelName),
				},
			},
		},
	).Return(&connect.Response[v1.ListModelsResponse]{
		Msg: &v1.ListModelsResponse{
			Models: []*v1.Model{
				{
					Id:              modelID,
					Name:            modelName,
					ModelProviderId: uuid.New().String(),
					ContextWindow:   8192,
					Enabled:         true,
					Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_IMAGE},
				},
			},
		},
	}, nil)
}

func setupModelGetMock(mockClient *api_client.MockClient, modelID, name, modelProviderID string, contextWindow int64, enabled bool) {
	mockClient.Model.EXPECT().GetModel(
		gomock.Any(),
		&connect.Request[v1.GetModelRequest]{
			Msg: &v1.GetModelRequest{Id: modelID},
		},
	).Return(&connect.Response[v1.GetModelResponse]{
		Msg: &v1.GetModelResponse{
			Model: &v1.Model{
				Id:              modelID,
				Name:            name,
				ModelProviderId: modelProviderID,
				ContextWindow:   contextWindow,
				Enabled:         enabled,
				Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_IMAGE},
			},
		},
	}, nil)
}
