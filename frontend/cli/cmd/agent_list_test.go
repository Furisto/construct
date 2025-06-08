package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestAgentList(t *testing.T) {
	setup := &TestSetup{}

	agentID1 := uuid.New().String()
	agentID2 := uuid.New().String()
	modelID1 := uuid.New().String()
	modelID2 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - list all agents",
			Command: []string{"agent", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListRequestMock(mockClient, nil, []*v1.Agent{
					createTestAgent(agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID1),
					createTestAgent(agentID2, "reviewer", "A code reviewer", "Description for reviewer", modelID2),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					[]*AgentDisplay{
						{
							ID:           agentID1,
							Name:         "coder",
							Description:  "Description for coder",
							Instructions: "A helpful coding assistant",
							Model:        modelID1,
						},
						{
							ID:           agentID2,
							Name:         "reviewer",
							Description:  "Description for reviewer",
							Instructions: "A code reviewer",
							Model:        modelID2,
						},
					},
				},
			},
		},
		{
			Name:    "success - list agents with JSON output",
			Command: []string{"agent", "list", "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListRequestMock(mockClient, nil, []*v1.Agent{
					createTestAgent(agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID1),
				})
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatJSON,
				DisplayedObjects: []any{
					[]*AgentDisplay{
						{
							ID:           agentID1,
							Name:         "coder",
							Description:  "Description for coder",
							Instructions: "A helpful coding assistant",
							Model:        modelID1,
						},
					},
				},
			},
		},
		{
			Name:    "success - filter agents by model ID",
			Command: []string{"agent", "list", "--model", modelID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				filter := &v1.ListAgentsRequest_Filter{
					ModelIds: []string{modelID1},
				}
				setupAgentListRequestMock(mockClient, filter, []*v1.Agent{
					createTestAgent(agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID1),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					[]*AgentDisplay{
						{
							ID:           agentID1,
							Name:         "coder",
							Description:  "Description for coder",
							Instructions: "A helpful coding assistant",
							Model:        modelID1,
						},
					},
				},
			},
		},
		{
			Name:    "success - filter agents by model name",
			Command: []string{"agent", "list", "--model", "claude-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupMock(mockClient, "claude-4", modelID1)
				filter := &v1.ListAgentsRequest_Filter{
					ModelIds: []string{modelID1},
				}
				setupAgentListRequestMock(mockClient, filter, []*v1.Agent{
					createTestAgent(agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID1),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					[]*AgentDisplay{
						{
							ID:           agentID1,
							Name:         "coder",
							Description:  "Description for coder",
							Instructions: "A helpful coding assistant",
							Model:        modelID1,
						},
					},
				},
			},
		},
		{
			Name:    "success - filter agents by multiple models",
			Command: []string{"agent", "list", "--model", modelID1, "--model", "claude-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupMock(mockClient, "claude-4", modelID2)
				filter := &v1.ListAgentsRequest_Filter{
					ModelIds: []string{modelID1, modelID2},
				}
				setupAgentListRequestMock(mockClient, filter, []*v1.Agent{
					createTestAgent(agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID1),
					createTestAgent(agentID2, "reviewer", "A code reviewer", "Description for reviewer", modelID2),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					[]*AgentDisplay{
						{
							ID:           agentID1,
							Name:         "coder",
							Description:  "Description for coder",
							Instructions: "A helpful coding assistant",
							Model:        modelID1,
						},
						{
							ID:           agentID2,
							Name:         "reviewer",
							Description:  "Description for reviewer",
							Instructions: "A code reviewer",
							Model:        modelID2,
						},
					},
				},
			},
		},
		{
			Name:    "success - filter agents by name",
			Command: []string{"agent", "list", "--name", "coder"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				filter := &v1.ListAgentsRequest_Filter{
					Name: []string{"coder"},
				}
				setupAgentListRequestMock(mockClient, filter, []*v1.Agent{
					createTestAgent(agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID1),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					[]*AgentDisplay{
						{
							ID:           agentID1,
							Name:         "coder",
							Description:  "Description for coder",
							Instructions: "A helpful coding assistant",
							Model:        modelID1,
						},
					},
				},
			},
		},
		{
			Name:    "success - filter agents by multiple names",
			Command: []string{"agent", "list", "--name", "coder", "--name", "reviewer"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				filter := &v1.ListAgentsRequest_Filter{
					Name: []string{"coder", "reviewer"},
				}
				setupAgentListRequestMock(mockClient, filter, []*v1.Agent{
					createTestAgent(agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID1),
					createTestAgent(agentID2, "reviewer", "A code reviewer", "Description for reviewer", modelID2),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					[]*AgentDisplay{
						{
							ID:           agentID1,
							Name:         "coder",
							Description:  "Description for coder",
							Instructions: "A helpful coding assistant",
							Model:        modelID1,
						},
						{
							ID:           agentID2,
							Name:         "reviewer",
							Description:  "Description for reviewer",
							Instructions: "A code reviewer",
							Model:        modelID2,
						},
					},
				},
			},
		},
		{
			Name:    "success - combined filters (model and name)",
			Command: []string{"agent", "list", "--model", modelID1, "--name", "coder"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				filter := &v1.ListAgentsRequest_Filter{
					ModelIds: []string{modelID1},
					Name:     []string{"coder"},
				}
				setupAgentListRequestMock(mockClient, filter, []*v1.Agent{
					createTestAgent(agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID1),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					[]*AgentDisplay{
						{
							ID:           agentID1,
							Name:         "coder",
							Description:  "Description for coder",
							Instructions: "A helpful coding assistant",
							Model:        modelID1,
						},
					},
				},
			},
		},
		{
			Name:    "success - empty result set",
			Command: []string{"agent", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListRequestMock(mockClient, nil, []*v1.Agent{})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					[]*AgentDisplay{},
				},
			},
		},
		{
			Name:    "error - list agents API failure",
			Command: []string{"agent", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Agent.EXPECT().ListAgents(
					gomock.Any(),
					&connect.Request[v1.ListAgentsRequest]{
						Msg: &v1.ListAgentsRequest{
							Filter: &v1.ListAgentsRequest_Filter{},
						},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to list agents: internal",
			},
		},
		{
			Name:    "error - model not found",
			Command: []string{"agent", "list", "--model", "nonexistent-model"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("nonexistent-model"),
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
				Error: "failed to resolve model nonexistent-model: model nonexistent-model not found",
			},
		},
		{
			Name:    "error - multiple models found for name",
			Command: []string{"agent", "list", "--model", "duplicate-model"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("duplicate-model"),
							},
						},
					},
				).Return(&connect.Response[v1.ListModelsResponse]{
					Msg: &v1.ListModelsResponse{
						Models: []*v1.Model{
							{Id: modelID1, Name: "duplicate-model"},
							{Id: modelID2, Name: "duplicate-model"},
						},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Error: "failed to resolve model duplicate-model: multiple models found for duplicate-model",
			},
		},
		{
			Name:    "error - list models API failure during name lookup",
			Command: []string{"agent", "list", "--model", "claude-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("claude-4"),
							},
						},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to resolve model claude-4: failed to list models: internal",
			},
		},
	})
}

func setupAgentListRequestMock(mockClient *api_client.MockClient, filter *v1.ListAgentsRequest_Filter, agents []*v1.Agent) {
	if filter == nil {
		filter = &v1.ListAgentsRequest_Filter{}
	}

	mockClient.Agent.EXPECT().ListAgents(
		gomock.Any(),
		&connect.Request[v1.ListAgentsRequest]{
			Msg: &v1.ListAgentsRequest{
				Filter: filter,
			},
		},
	).Return(&connect.Response[v1.ListAgentsResponse]{
		Msg: &v1.ListAgentsResponse{
			Agents: agents,
		},
	}, nil)
}

func createTestAgent(id, name, instructions, description, modelID string) *v1.Agent {
	agent := &v1.Agent{
		Id: id,
		Metadata: &v1.AgentMetadata{
			Name: name,
		},
		Spec: &v1.AgentSpec{
			Instructions: instructions,
			ModelId:      modelID,
		},
	}

	if description != "" {
		agent.Metadata.Description = description
	}

	return agent
}
