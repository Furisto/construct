package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestModelDelete(t *testing.T) {
	setup := &TestSetup{}

	modelID1 := uuid.New().String()
	modelID2 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - delete model by name with force flag",
			Command: []string{"model", "delete", "--force", "gpt-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForDeleteMock(mockClient, "gpt-4", modelID1)
				setupModelDeleteMock(mockClient, modelID1)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete model by ID with force flag",
			Command: []string{"model", "delete", "--force", modelID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelDeleteMock(mockClient, modelID1)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete multiple models with force flag",
			Command: []string{"model", "delete", "--force", "gpt-4", "claude-3-5-sonnet"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForDeleteMock(mockClient, "gpt-4", modelID1)
				setupModelLookupForDeleteMock(mockClient, "claude-3-5-sonnet", modelID2)
				setupModelDeleteMock(mockClient, modelID1)
				setupModelDeleteMock(mockClient, modelID2)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete multiple models by ID and name with force flag",
			Command: []string{"model", "delete", "--force", modelID1, "llama-3.1-8b"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForDeleteMock(mockClient, "llama-3.1-8b", modelID2)
				setupModelDeleteMock(mockClient, modelID1)
				setupModelDeleteMock(mockClient, modelID2)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete model by name with user confirmation",
			Command: []string{"model", "delete", "gpt-4"},
			Stdin:   "y\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForDeleteMock(mockClient, "gpt-4", modelID1)
				setupModelDeleteMock(mockClient, modelID1)
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete model gpt-4? (y/n): ",
			},
		},
		{
			Name:    "success - cancel deletion when user denies confirmation",
			Command: []string{"model", "delete", "gpt-4"},
			Stdin:   "n\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForDeleteMock(mockClient, "gpt-4", modelID1)
				// No delete mocks needed since operation should be cancelled
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete model gpt-4? (y/n): ",
			},
		},
		{
			Name:    "success - delete multiple models with user confirmation",
			Command: []string{"model", "delete", "gpt-4", "claude-3-5-sonnet"},
			Stdin:   "y\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForDeleteMock(mockClient, "gpt-4", modelID1)
				setupModelLookupForDeleteMock(mockClient, "claude-3-5-sonnet", modelID2)
				setupModelDeleteMock(mockClient, modelID1)
				setupModelDeleteMock(mockClient, modelID2)
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete models gpt-4 claude-3-5-sonnet? (y/n): ",
			},
		},
		{
			Name:    "error - model not found by name with force flag",
			Command: []string{"model", "delete", "--force", "nonexistent"},
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
			Name:    "error - delete model API failure with force flag",
			Command: []string{"model", "delete", "--force", modelID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().DeleteModel(
					gomock.Any(),
					&connect.Request[v1.DeleteModelRequest]{
						Msg: &v1.DeleteModelRequest{Id: modelID1},
					},
				).Return(nil, connect.NewError(connect.CodeNotFound, nil))
			},
			Expected: TestExpectation{
				Error: "failed to delete model " + modelID1 + ": not_found",
			},
		},
		{
			Name:    "error - delete multiple models with one failure with force flag",
			Command: []string{"model", "delete", "--force", "gpt-4", "claude-3-5-sonnet"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupForDeleteMock(mockClient, "gpt-4", modelID1)
				setupModelLookupForDeleteMock(mockClient, "claude-3-5-sonnet", modelID2)
				setupModelDeleteMock(mockClient, modelID1)
				mockClient.Model.EXPECT().DeleteModel(
					gomock.Any(),
					&connect.Request[v1.DeleteModelRequest]{
						Msg: &v1.DeleteModelRequest{Id: modelID2},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to delete model claude-3-5-sonnet: internal",
			},
		},
		{
			Name:    "error - model lookup API failure with force flag",
			Command: []string{"model", "delete", "--force", "gpt-4"},
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

func setupModelLookupForDeleteMock(mockClient *api_client.MockClient, modelName, modelID string) {
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
