package cmd

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/shared/conv"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTaskList(t *testing.T) {
	setup := &TestSetup{}

	taskID1 := uuid.New().String()
	taskID2 := uuid.New().String()
	agentID1 := uuid.New().String()
	agentID2 := uuid.New().String()
	createdAt := time.Now()
	updatedAt := time.Now().Add(time.Hour)

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - list all tasks",
			Command: []string{"task", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskListMock(mockClient, nil, []*v1.Task{
					createTestTask(taskID1, agentID1, createdAt, updatedAt),
					createTestTask(taskID2, agentID2, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayTask{
					{
						Id:        taskID1,
						AgentId:   agentID1,
						CreatedAt: createdAt,
						UpdatedAt: updatedAt,
						Usage: DisplayTaskUsage{
							InputTokens:      1000,
							OutputTokens:     500,
							CacheWriteTokens: 100,
							CacheReadTokens:  50,
							Cost:             0.05,
						},
					},
					{
						Id:        taskID2,
						AgentId:   agentID2,
						CreatedAt: createdAt,
						UpdatedAt: updatedAt,
						Usage: DisplayTaskUsage{
							InputTokens:      1000,
							OutputTokens:     500,
							CacheWriteTokens: 100,
							CacheReadTokens:  50,
							Cost:             0.05,
						},
					},
				},
			},
		},
		{
			Name:    "success - list tasks filtered by agent name",
			Command: []string{"task", "list", "--agent", "coder"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForTaskListMock(mockClient, "coder", agentID1)
				setupTaskListMock(mockClient, &agentID1, []*v1.Task{
					createTestTask(taskID1, agentID1, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayTask{
					{
						Id:        taskID1,
						AgentId:   agentID1,
						CreatedAt: createdAt,
						UpdatedAt: updatedAt,
						Usage: DisplayTaskUsage{
							InputTokens:      1000,
							OutputTokens:     500,
							CacheWriteTokens: 100,
							CacheReadTokens:  50,
							Cost:             0.05,
						},
					},
				},
			},
		},
		{
			Name:    "success - list tasks filtered by agent ID",
			Command: []string{"task", "list", "--agent", agentID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskListMock(mockClient, &agentID1, []*v1.Task{
					createTestTask(taskID1, agentID1, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayTask{
					{
						Id:        taskID1,
						AgentId:   agentID1,
						CreatedAt: createdAt,
						UpdatedAt: updatedAt,
						Usage: DisplayTaskUsage{
							InputTokens:      1000,
							OutputTokens:     500,
							CacheWriteTokens: 100,
							CacheReadTokens:  50,
							Cost:             0.05,
						},
					},
				},
			},
		},
		{
			Name:    "success - list tasks with JSON output",
			Command: []string{"task", "list", "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskListMock(mockClient, nil, []*v1.Task{
					createTestTask(taskID1, agentID1, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayFormat: &RenderOptions{
					Format: OutputFormatJSON,
				},
				DisplayedObjects: []*DisplayTask{
					{
						Id:        taskID1,
						AgentId:   agentID1,
						CreatedAt: createdAt,
						UpdatedAt: updatedAt,
						Usage: DisplayTaskUsage{
							InputTokens:      1000,
							OutputTokens:     500,
							CacheWriteTokens: 100,
							CacheReadTokens:  50,
							Cost:             0.05,
						},
					},
				},
			},
		},
		{
			Name:    "success - empty task list",
			Command: []string{"task", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskListMock(mockClient, nil, []*v1.Task{})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayTask{},
			},
		},
		{
			Name:    "error - agent not found by name",
			Command: []string{"task", "list", "--agent", "nonexistent"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Agent.EXPECT().ListAgents(
					gomock.Any(),
					&connect.Request[v1.ListAgentsRequest]{
						Msg: &v1.ListAgentsRequest{
							Filter: &v1.ListAgentsRequest_Filter{
								Names: []string{"nonexistent"},
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
			Name:    "error - list tasks API failure",
			Command: []string{"task", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Task.EXPECT().ListTasks(
					gomock.Any(),
					&connect.Request[v1.ListTasksRequest]{
						Msg: &v1.ListTasksRequest{
							Filter: &v1.ListTasksRequest_Filter{},
							PageSize: conv.Ptr(int32(0)),
						},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to list tasks: internal",
			},
		},
	})
}

func setupTaskListMock(mockClient *api_client.MockClient, agentID *string, tasks []*v1.Task) {
	filter := &v1.ListTasksRequest_Filter{}
	if agentID != nil {
		filter.AgentId = agentID
	}

	mockClient.Task.EXPECT().ListTasks(
		gomock.Any(),
		CmpEqual(&connect.Request[v1.ListTasksRequest]{
			Msg: &v1.ListTasksRequest{
				Filter:   filter,
				PageSize: conv.Ptr(int32(0)),
			},
		}, protocmp.Transform(),
			cmpopts.IgnoreUnexported(connect.Request[v1.ListTasksRequest]{}),
			cmpopts.IgnoreFields(v1.ListTasksRequest{}, "state"),
		),
	).Return(&connect.Response[v1.ListTasksResponse]{
		Msg: &v1.ListTasksResponse{
			Tasks: tasks,
		},
	}, nil)
}

func setupAgentLookupForTaskListMock(mockClient *api_client.MockClient, agentName, agentID string) {
	mockClient.Agent.EXPECT().ListAgents(
		gomock.Any(),
		CmpEqual(&connect.Request[v1.ListAgentsRequest]{
			Msg: &v1.ListAgentsRequest{
				Filter: &v1.ListAgentsRequest_Filter{
					Names: []string{agentName},
				},
			},
		}, protocmp.Transform(),
			cmpopts.IgnoreUnexported(connect.Request[v1.ListAgentsRequest]{}),
			cmpopts.IgnoreFields(v1.ListAgentsRequest{}, "state"),
		),
	).Return(&connect.Response[v1.ListAgentsResponse]{
		Msg: &v1.ListAgentsResponse{
			Agents: []*v1.Agent{
				{
					Metadata: &v1.AgentMetadata{
						Id: agentID,
					},
					Spec: &v1.AgentSpec{
						Name: agentName,
					},
				},
			},
		},
	}, nil)
}

func createTestTask(taskID, agentID string, createdAt, updatedAt time.Time) *v1.Task {
	return &v1.Task{
		Metadata: &v1.TaskMetadata{
			Id:        taskID,
			CreatedAt: timestamppb.New(createdAt),
			UpdatedAt: timestamppb.New(updatedAt),
		},
		Spec: &v1.TaskSpec{
			AgentId: &agentID,
		},
		Status: &v1.TaskStatus{
			Usage: &v1.TaskUsage{
				InputTokens:      1000,
				OutputTokens:     500,
				CacheWriteTokens: 100,
				CacheReadTokens:  50,
				Cost:             0.05,
			},
		},
	}
}
