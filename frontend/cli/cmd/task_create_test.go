package cmd

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTaskCreate(t *testing.T) {
	setup := &TestSetup{}

	taskID1 := uuid.NewString()
	agentID1 := uuid.NewString()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - create task with agent by name",
			Command: []string{"task", "create", "--agent", "coder"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForTaskCreateMock(mockClient, "coder", agentID1)
				setupTaskCreateMock(mockClient, agentID1, "", taskID1)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(taskID1),
			},
		},
		{
			Name:    "success - create task with agent by ID",
			Command: []string{"task", "create", "--agent", agentID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskCreateMock(mockClient, agentID1, "", taskID1)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(taskID1),
			},
		},
		{
			Name:    "success - create task with both agent and workspace directory",
			Command: []string{"task", "create", "--agent", "sql-expert", "--workspace", "/path/to/repo"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForTaskCreateMock(mockClient, "sql-expert", agentID1)
				setupTaskCreateMock(mockClient, agentID1, "/path/to/repo", taskID1)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.MkdirAll("/path/to/repo", 0755)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(taskID1),
			},
		},
		{
			Name:    "success - create task with short form flags",
			Command: []string{"task", "create", "-a", "reviewer", "-w", "/path/to/repo"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForTaskCreateMock(mockClient, "reviewer", agentID1)
				setupTaskCreateMock(mockClient, agentID1, "/path/to/repo", taskID1)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.MkdirAll("/path/to/repo", 0755)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(taskID1),
			},
		},
		{
			Name:    "error - agent not provided",
			Command: []string{"task", "create", "-w", "/path/to/repo"},
			Expected: TestExpectation{
				Error: "required flag(s) \"agent\" not set",
			},
		},
		{
			Name:    "error - agent not found by name",
			Command: []string{"task", "create", "--agent", "nonexistent"},
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
			Name:    "error - workspace directory does not exist",
			Command: []string{"task", "create", "--agent", "coder", "--workspace", "/path/to/nonexistent"},
			Expected: TestExpectation{
				Error: "workspace directory /path/to/nonexistent does not exist",
			},
		},
		{
			Name:    "error - create task API failure",
			Command: []string{"task", "create", "--agent", "coder", "--workspace", "/path/to/repo"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForTaskCreateMock(mockClient, "coder", agentID1)
				mockClient.Task.EXPECT().CreateTask(
					gomock.Any(),
					&connect.Request[v1.CreateTaskRequest]{
						Msg: &v1.CreateTaskRequest{
							AgentId:          agentID1,
							ProjectDirectory: "/path/to/repo",
						},
					},
				).Return(nil, connect.NewError(connect.CodeAlreadyExists, nil))
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.MkdirAll("/path/to/repo", 0755)
			},
			Expected: TestExpectation{
				Error: "failed to create task: already_exists",
			},
		},
		{
			Name:    "error - agent lookup API failure",
			Command: []string{"task", "create", "--agent", "coder"},
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

func setupTaskCreateMock(mockClient *api_client.MockClient, agentID, workspace, taskID string) {
	mockClient.Task.EXPECT().CreateTask(
		gomock.Any(),
		&connect.Request[v1.CreateTaskRequest]{
			Msg: &v1.CreateTaskRequest{
				AgentId:          agentID,
				ProjectDirectory: workspace,
			},
		},
	).Return(&connect.Response[v1.CreateTaskResponse]{
		Msg: &v1.CreateTaskResponse{
			Task: &v1.Task{
				Id: taskID,
				Metadata: &v1.TaskMetadata{
					CreatedAt: timestamppb.Now(),
					UpdatedAt: timestamppb.Now(),
				},
				Spec: &v1.TaskSpec{},
			},
		},
	}, nil)
}

func setupAgentLookupForTaskCreateMock(mockClient *api_client.MockClient, agentName, agentID string) {
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
