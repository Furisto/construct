package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestTaskDelete(t *testing.T) {
	setup := &TestSetup{}

	taskID1 := uuid.New().String()
	taskID2 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - delete single task",
			Command: []string{"task", "delete", taskID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskDeleteMock(mockClient, taskID1)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "success - delete multiple tasks",
			Command: []string{"task", "delete", taskID1, taskID2},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskDeleteMock(mockClient, taskID1)
				setupTaskDeleteMock(mockClient, taskID2)
			},
			Expected: TestExpectation{},
		},
		{
			Name:    "error - delete task API failure",
			Command: []string{"task", "delete", taskID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Task.EXPECT().DeleteTask(
					gomock.Any(),
					&connect.Request[v1.DeleteTaskRequest]{
						Msg: &v1.DeleteTaskRequest{Id: taskID1},
					},
				).Return(nil, connect.NewError(connect.CodeNotFound, nil))
			},
			Expected: TestExpectation{
				Error: "failed to delete task " + taskID1 + ": not_found",
			},
		},
		{
			Name:    "error - delete multiple tasks with one failure",
			Command: []string{"task", "delete", taskID1, taskID2},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupTaskDeleteMock(mockClient, taskID1)
				mockClient.Task.EXPECT().DeleteTask(
					gomock.Any(),
					&connect.Request[v1.DeleteTaskRequest]{
						Msg: &v1.DeleteTaskRequest{Id: taskID2},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to delete task " + taskID2 + ": internal",
			},
		},
	})
}

func setupTaskDeleteMock(mockClient *api_client.MockClient, taskID string) {
	mockClient.Task.EXPECT().DeleteTask(
		gomock.Any(),
		&connect.Request[v1.DeleteTaskRequest]{
			Msg: &v1.DeleteTaskRequest{Id: taskID},
		},
	).Return(&connect.Response[v1.DeleteTaskResponse]{}, nil)
}
