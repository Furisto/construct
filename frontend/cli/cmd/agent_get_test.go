package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestAgentGetCLI(t *testing.T) {
	setup := &TestSetup{}

	agentID1 := uuid.New().String()
	agentID2 := uuid.New().String()
	modelID := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - get agent by name",
			Command: []string{"agent", "get", "coder"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForGetMock(mockClient, "coder", agentID1)
				setupAgentGetMock(mockClient, agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID, []string{})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					&AgentDisplay{
						ID:           agentID1,
						Name:         "coder",
						Description:  "Description for coder",
						Instructions: "A helpful coding assistant",
						Model:        modelID,
					},
				},
			},
		},
		{
			Name:    "success - get agent by ID",
			Command: []string{"agent", "get", agentID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentGetMock(mockClient, agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID, []string{})
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					&AgentDisplay{
						ID:           agentID1,
						Name:         "coder",
						Description:  "Description for coder",
						Instructions: "A helpful coding assistant",
						Model:        modelID,
					},
				},
			},
		},
		{
			Name:    "success - get agent with JSON output format",
			Command: []string{"agent", "get", "coder", "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForGetMock(mockClient, "coder", agentID1)
				setupAgentGetMock(mockClient, agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID, []string{})
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatJSON,
				DisplayedObjects: []any{
					&AgentDisplay{
						ID:           agentID1,
						Name:         "coder",
						Description:  "Description for coder",
						Instructions: "A helpful coding assistant",
						Model:        modelID,
					},
				},
			},
		},
		{
			Name:    "success - get agent with YAML output format",
			Command: []string{"agent", "get", "coder", "--output", "yaml"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForGetMock(mockClient, "coder", agentID1)
				setupAgentGetMock(mockClient, agentID1, "coder", "A helpful coding assistant", "Description for coder", modelID, []string{})
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatYAML,
				DisplayedObjects: []any{
					&AgentDisplay{
						ID:           agentID1,
						Name:         "coder",
						Description:  "Description for coder",
						Instructions: "A helpful coding assistant",
						Model:        modelID,
					},
				},
			},
		},
		{
			Name:    "error - agent not found by name",
			Command: []string{"agent", "get", "nonexistent"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Agent.EXPECT().ListAgents(
					gomock.Any(),
					&connect.Request[v1.ListAgentsRequest]{
						Msg: &v1.ListAgentsRequest{
							Filter: &v1.ListAgentsRequest_Filter{
								Name: []string{"nonexistent"},
							},
						},
					},
				).Return(&connect.Response[v1.ListAgentsResponse]{
					Msg: &v1.ListAgentsResponse{
						Agents: []*v1.Agent{},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Error: "failed to resolve agent nonexistent: agent nonexistent not found",
			},
		},
		{
			Name:    "error - multiple agents found for name",
			Command: []string{"agent", "get", "duplicate"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Agent.EXPECT().ListAgents(
					gomock.Any(),
					&connect.Request[v1.ListAgentsRequest]{
						Msg: &v1.ListAgentsRequest{
							Filter: &v1.ListAgentsRequest_Filter{
								Name: []string{"duplicate"},
							},
						},
					},
				).Return(&connect.Response[v1.ListAgentsResponse]{
					Msg: &v1.ListAgentsResponse{
						Agents: []*v1.Agent{
							{
								Id: agentID1,
								Metadata: &v1.AgentMetadata{
									Name: "duplicate",
								},
							},
							{
								Id: agentID2,
								Metadata: &v1.AgentMetadata{
									Name: "duplicate",
								},
							},
						},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Error: "failed to resolve agent duplicate: multiple agents found for duplicate",
			},
		},
		{
			Name:    "error - get agent API failure",
			Command: []string{"agent", "get", agentID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Agent.EXPECT().GetAgent(
					gomock.Any(),
					&connect.Request[v1.GetAgentRequest]{
						Msg: &v1.GetAgentRequest{Id: agentID1},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to get agent " + agentID1 + ": internal",
			},
		},
		{
			Name:    "error - list agents API failure during name lookup",
			Command: []string{"agent", "get", "coder"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Agent.EXPECT().ListAgents(
					gomock.Any(),
					&connect.Request[v1.ListAgentsRequest]{
						Msg: &v1.ListAgentsRequest{
							Filter: &v1.ListAgentsRequest_Filter{
								Name: []string{"coder"},
							},
						},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to resolve agent coder: failed to list agents: internal",
			},
		},
	})
}

func setupAgentLookupForGetMock(mockClient *api_client.MockClient, agentName, agentID string) {
	mockClient.Agent.EXPECT().ListAgents(
		gomock.Any(),
		&connect.Request[v1.ListAgentsRequest]{
			Msg: &v1.ListAgentsRequest{
				Filter: &v1.ListAgentsRequest_Filter{
					Name: []string{agentName},
				},
			},
		},
	).Return(&connect.Response[v1.ListAgentsResponse]{
		Msg: &v1.ListAgentsResponse{
			Agents: []*v1.Agent{
				{
					Id: agentID,
					Metadata: &v1.AgentMetadata{
						Name: agentName,
					},
				},
			},
		},
	}, nil)
}

func setupAgentGetMock(mockClient *api_client.MockClient, agentID, name, instructions, description, modelID string, delegateIDs []string) {
	agent := &v1.Agent{
		Id: agentID,
		Metadata: &v1.AgentMetadata{
			Name: name,
		},
		Spec: &v1.AgentSpec{
			Instructions: instructions,
			ModelId:      modelID,
			DelegateIds:  delegateIDs,
		},
	}

	if description != "" {
		agent.Metadata.Description = description
	}

	mockClient.Agent.EXPECT().GetAgent(
		gomock.Any(),
		&connect.Request[v1.GetAgentRequest]{
			Msg: &v1.GetAgentRequest{Id: agentID},
		},
	).Return(&connect.Response[v1.GetAgentResponse]{
		Msg: &v1.GetAgentResponse{
			Agent: agent,
		},
	}, nil)
}
