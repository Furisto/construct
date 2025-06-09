package cmd

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTaskGet(t *testing.T) {
	setup := &TestSetup{
		
	}

	taskID1 := uuid.New().String()
	agentID1 := uuid.New().String()
	createdAt := time.Now()
	updatedAt := time.Now().Add(time.Hour)

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - get task by ID",
			Command: []string{"task", "get", taskID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskGetMock(mockClient, taskID1, agentID1, createdAt, updatedAt)
			},
			Expected: TestExpectation{
				DisplayedObjects: []any{
					&DisplayTask{
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
			Name:    "success - get task with JSON output",
			Command: []string{"task", "get", taskID1, "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskGetMock(mockClient, taskID1, agentID1, createdAt, updatedAt)
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatJSON,
				DisplayedObjects: []any{
					&DisplayTask{
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
			Name:    "success - get task with YAML output",
			Command: []string{"task", "get", taskID1, "--output", "yaml"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskGetMock(mockClient, taskID1, agentID1, createdAt, updatedAt)
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatYAML,
				DisplayedObjects: []any{
					&DisplayTask{
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
			Name:    "error - get task API failure",
			Command: []string{"task", "get", taskID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Task.EXPECT().GetTask(
					gomock.Any(),
					&connect.Request[v1.GetTaskRequest]{
						Msg: &v1.GetTaskRequest{Id: taskID1},
					},
				).Return(nil, connect.NewError(connect.CodeNotFound, nil))
			},
			Expected: TestExpectation{
				Error: "failed to get task " + taskID1 + ": not_found",
			},
		},
	})
}

func setupTaskGetMock(mockClient *api_client.MockClient, taskID, agentID string, createdAt, updatedAt time.Time) {
	mockClient.Task.EXPECT().GetTask(
		gomock.Any(),
		&connect.Request[v1.GetTaskRequest]{
			Msg: &v1.GetTaskRequest{Id: taskID},
		},
	).Return(&connect.Response[v1.GetTaskResponse]{
		Msg: &v1.GetTaskResponse{
			Task: &v1.Task{
				Id: taskID,
				Metadata: &v1.TaskMetadata{
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
			},
		},
	}, nil)
}
