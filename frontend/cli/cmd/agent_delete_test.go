package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestAgentDelete(t *testing.T) {
	setup := &TestSetup{}

	agentID1 := uuid.New().String()
	agentID2 := uuid.New().String()
	agentID3 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - delete by agent name with force flag",
			Command: []string{"agent", "delete", "--force", "coder"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListMock(mockClient, "coder", agentID1)
				setupAgentDeletionMock(mockClient, agentID1)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete by agent ID with force flag",
			Command: []string{"agent", "delete", "--force", agentID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentDeletionMock(mockClient, agentID1)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete multiple agents by name with force flag",
			Command: []string{"agent", "delete", "--force", "coder", "architect"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListMock(mockClient, "coder", agentID1)
				setupAgentListMock(mockClient, "architect", agentID2)
				setupAgentDeletionMock(mockClient, agentID1)
				setupAgentDeletionMock(mockClient, agentID2)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete multiple agents by ID with force flag",
			Command: []string{"agent", "delete", "--force", agentID1, agentID2},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentDeletionMock(mockClient, agentID1)
				setupAgentDeletionMock(mockClient, agentID2)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete mixed IDs and names with force flag",
			Command: []string{"agent", "delete", "--force", agentID1, "architect", agentID3},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListMock(mockClient, "architect", agentID2)
				setupAgentDeletionMock(mockClient, agentID1)
				setupAgentDeletionMock(mockClient, agentID2)
				setupAgentDeletionMock(mockClient, agentID3)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete by agent name with user confirmation",
			Command: []string{"agent", "delete", "coder"},
			Stdin:   "y\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListMock(mockClient, "coder", agentID1)
				setupAgentDeletionMock(mockClient, agentID1)
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete agent coder? (y/n): ",
			},
		},
		{
			Name:    "success - cancel deletion when user denies confirmation",
			Command: []string{"agent", "delete", "coder"},
			Stdin:   "n\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListMock(mockClient, "coder", agentID1)
				// No deletion mocks needed since operation should be cancelled
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete agent coder? (y/n): ",
			},
		},
		{
			Name:    "success - delete multiple agents with user confirmation",
			Command: []string{"agent", "delete", "coder", "architect"},
			Stdin:   "y\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListMock(mockClient, "coder", agentID1)
				setupAgentListMock(mockClient, "architect", agentID2)
				setupAgentDeletionMock(mockClient, agentID1)
				setupAgentDeletionMock(mockClient, agentID2)
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete agents coder architect? (y/n): ",
			},
		},
		{
			Name:    "error - agent not found by name with force flag",
			Command: []string{"agent", "delete", "--force", "nonexistent"},
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
				Error: "agent nonexistent not found",
			},
		},
		{
			// if one agent does not exist, the others should not be deleted
			Name:    "error - first agent succeeds, second fails lookup with force flag",
			Command: []string{"agent", "delete", "--force", "coder", "nonexistent"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentListMock(mockClient, "coder", agentID1)
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
				Error: "agent nonexistent not found",
			},
		},
	})
}

func setupAgentListMock(mockClient *api_client.MockClient, agentName, agentID string) {
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

func setupAgentDeletionMock(mockClient *api_client.MockClient, agentID string) {
	mockClient.Agent.EXPECT().DeleteAgent(
		gomock.Any(),
		&connect.Request[v1.DeleteAgentRequest]{
			Msg: &v1.DeleteAgentRequest{Id: agentID},
		},
	).Return(&connect.Response[v1.DeleteAgentResponse]{
		Msg: &v1.DeleteAgentResponse{},
	}, nil)
}
