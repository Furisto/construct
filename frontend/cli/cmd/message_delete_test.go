package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestMessageDelete(t *testing.T) {
	setup := &TestSetup{}

	messageID1 := uuid.New().String()
	messageID2 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - delete single message with force flag",
			Command: []string{"message", "delete", "--force", messageID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageDeleteMock(mockClient, messageID1)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:    "success - delete multiple messages with force flag",
			Command: []string{"message", "delete", "--force", messageID1, messageID2},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageDeleteMock(mockClient, messageID1)
				setupMessageDeleteMock(mockClient, messageID2)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:    "success - delete single message with user confirmation",
			Command: []string{"message", "delete", messageID1},
			Stdin:   "y\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageDeleteMock(mockClient, messageID1)
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete message " + messageID1 + "? (y/n): ",
			},
		},
		{
			Name:    "success - cancel deletion when user denies confirmation",
			Command: []string{"message", "delete", messageID1},
			Stdin:   "n\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				// No delete mocks needed since operation should be cancelled
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete message " + messageID1 + "? (y/n): ",
			},
		},
		{
			Name:    "success - delete multiple messages with user confirmation",
			Command: []string{"message", "delete", messageID1, messageID2},
			Stdin:   "y\n",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageDeleteMock(mockClient, messageID1)
				setupMessageDeleteMock(mockClient, messageID2)
			},
			Expected: TestExpectation{
				Stdout: "Are you sure you want to delete messages " + messageID1 + " " + messageID2 + "? (y/n): ",
			},
		},
		{
			Name:    "error - delete message API failure on first message with force flag",
			Command: []string{"message", "delete", "--force", messageID1, messageID2},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Message.EXPECT().DeleteMessage(
					gomock.Any(),
					&connect.Request[v1.DeleteMessageRequest]{
						Msg: &v1.DeleteMessageRequest{Id: messageID1},
					},
				).Return(nil, connect.NewError(connect.CodeNotFound, nil))
				// Second deletion should not be called due to early return
			},
			Expected: TestExpectation{
				Error: "failed to delete message " + messageID1 + ": not_found",
			},
		},
	})
}

func setupMessageDeleteMock(mockClient *api_client.MockClient, messageID string) {
	mockClient.Message.EXPECT().DeleteMessage(
		gomock.Any(),
		&connect.Request[v1.DeleteMessageRequest]{
			Msg: &v1.DeleteMessageRequest{Id: messageID},
		},
	).Return(&connect.Response[v1.DeleteMessageResponse]{
		Msg: &v1.DeleteMessageResponse{},
	}, nil)
}
