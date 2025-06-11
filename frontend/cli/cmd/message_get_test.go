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

func TestMessageGet(t *testing.T) {
	setup := &TestSetup{}

	messageID1 := uuid.New().String()
	taskID1 := uuid.New().String()
	agentID1 := uuid.New().String()
	modelID1 := uuid.New().String()
	createdAt := time.Now()
	updatedAt := time.Now().Add(time.Hour)

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - get message by ID",
			Command: []string{"message", "get", messageID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageGetMock(mockClient, messageID1, taskID1, agentID1, modelID1, "Test message content", v1.MessageRole_MESSAGE_ROLE_USER, createdAt, updatedAt)
			},
			Expected: TestExpectation{
				DisplayedObjects: &DisplayMessage{
					Id:        messageID1,
					TaskId:    taskID1,
					Agent:     agentID1,
					Model:     modelID1,
					Role:      "user",
					Content:   "Test message content",
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
					Usage: DisplayMessageUsage{
						InputTokens:      100,
						OutputTokens:     50,
						CacheWriteTokens: 10,
						CacheReadTokens:  5,
						Cost:             0.01,
					},
				},
			},
		},
		{
			Name:    "success - get assistant message with JSON output",
			Command: []string{"message", "get", messageID1, "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageGetMock(mockClient, messageID1, taskID1, agentID1, modelID1, "Assistant response", v1.MessageRole_MESSAGE_ROLE_ASSISTANT, createdAt, updatedAt)
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatJSON,
				DisplayedObjects: &DisplayMessage{
					Id:        messageID1,
					TaskId:    taskID1,
					Agent:     agentID1,
					Model:     modelID1,
					Role:      "assistant",
					Content:   "Assistant response",
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
					Usage: DisplayMessageUsage{
						InputTokens:      100,
						OutputTokens:     50,
						CacheWriteTokens: 10,
						CacheReadTokens:  5,
						Cost:             0.01,
					},
				},
			},
		},
		{
			Name:    "success - get message with YAML output",
			Command: []string{"message", "get", messageID1, "--output", "yaml"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageGetMock(mockClient, messageID1, taskID1, agentID1, modelID1, "YAML test message", v1.MessageRole_MESSAGE_ROLE_USER, createdAt, updatedAt)
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatYAML,
				DisplayedObjects: &DisplayMessage{
					Id:        messageID1,
					TaskId:    taskID1,
					Agent:     agentID1,
					Model:     modelID1,
					Role:      "user",
					Content:   "YAML test message",
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
					Usage: DisplayMessageUsage{
						InputTokens:      100,
						OutputTokens:     50,
						CacheWriteTokens: 10,
						CacheReadTokens:  5,
						Cost:             0.01,
					},
				},
			},
		},
		{
			Name:    "error - get message API failure",
			Command: []string{"message", "get", messageID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Message.EXPECT().GetMessage(
					gomock.Any(),
					&connect.Request[v1.GetMessageRequest]{
						Msg: &v1.GetMessageRequest{Id: messageID1},
					},
				).Return(nil, connect.NewError(connect.CodeNotFound, nil))
			},
			Expected: TestExpectation{
				Error: "failed to get message " + messageID1 + ": not_found",
			},
		},
	})
}

func setupMessageGetMock(mockClient *api_client.MockClient, messageID, taskID, agentID, modelID, content string, role v1.MessageRole, createdAt, updatedAt time.Time) {
	mockClient.Message.EXPECT().GetMessage(
		gomock.Any(),
		&connect.Request[v1.GetMessageRequest]{
			Msg: &v1.GetMessageRequest{Id: messageID},
		},
	).Return(&connect.Response[v1.GetMessageResponse]{
		Msg: &v1.GetMessageResponse{
			Message: &v1.Message{
				Id: messageID,
				Metadata: &v1.MessageMetadata{
					TaskId:    taskID,
					AgentId:   &agentID,
					ModelId:   &modelID,
					Role:      role,
					CreatedAt: timestamppb.New(createdAt),
					UpdatedAt: timestamppb.New(updatedAt),
					Usage: &v1.MessageUsage{
						InputTokens:      100,
						OutputTokens:     50,
						CacheWriteTokens: 10,
						CacheReadTokens:  5,
						Cost:             0.01,
					},
				},
				Content: &v1.MessageContent{
					Content: &v1.MessageContent_Text{
						Text: content,
					},
				},
			},
		},
	}, nil)
}
