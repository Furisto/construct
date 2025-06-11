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

func TestMessageList(t *testing.T) {
	setup := &TestSetup{}

	messageID1 := uuid.New().String()
	messageID2 := uuid.New().String()
	taskID1 := uuid.New().String()
	taskID2 := uuid.New().String()
	agentID1 := uuid.New().String()
	agentID2 := uuid.New().String()
	modelID1 := uuid.New().String()
	createdAt := time.Now()
	updatedAt := time.Now().Add(time.Hour)

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - list all messages",
			Command: []string{"message", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageListMock(mockClient, nil, nil, nil, []*v1.Message{
					createTestMessage(messageID1, taskID1, agentID1, modelID1, "User message", v1.MessageRole_MESSAGE_ROLE_USER, createdAt, updatedAt),
					createTestMessage(messageID2, taskID2, agentID2, modelID1, "Assistant response", v1.MessageRole_MESSAGE_ROLE_ASSISTANT, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayMessage{
					{
						Id:        messageID1,
						TaskId:    taskID1,
						Agent:     agentID1,
						Model:     modelID1,
						Role:      "user",
						Content:   "User message",
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
					{
						Id:        messageID2,
						TaskId:    taskID2,
						Agent:     agentID2,
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
		},
		{
			Name:    "success - list messages filtered by agent name",
			Command: []string{"message", "list", "--agent", "coder"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForMessageListMock(mockClient, "coder", agentID1)
				setupMessageListMock(mockClient, nil, &agentID1, nil, []*v1.Message{
					createTestMessage(messageID1, taskID1, agentID1, modelID1, "User message", v1.MessageRole_MESSAGE_ROLE_USER, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayMessage{
					{
						Id:        messageID1,
						TaskId:    taskID1,
						Agent:     agentID1,
						Model:     modelID1,
						Role:      "user",
						Content:   "User message",
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
		},
		{
			Name:    "success - list messages filtered by agent ID",
			Command: []string{"message", "list", "--agent", agentID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageListMock(mockClient, nil, &agentID1, nil, []*v1.Message{
					createTestMessage(messageID1, taskID1, agentID1, modelID1, "User message", v1.MessageRole_MESSAGE_ROLE_USER, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayMessage{
					{
						Id:        messageID1,
						TaskId:    taskID1,
						Agent:     agentID1,
						Model:     modelID1,
						Role:      "user",
						Content:   "User message",
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
		},
		{
			Name:    "success - list messages filtered by task ID",
			Command: []string{"message", "list", "--task", taskID1},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageListMock(mockClient, &taskID1, nil, nil, []*v1.Message{
					createTestMessage(messageID1, taskID1, agentID1, modelID1, "User message", v1.MessageRole_MESSAGE_ROLE_USER, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayMessage{
					{
						Id:        messageID1,
						TaskId:    taskID1,
						Agent:     agentID1,
						Model:     modelID1,
						Role:      "user",
						Content:   "User message",
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
		},
		{
			Name:    "success - list messages filtered by role",
			Command: []string{"message", "list", "--role", "user"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				userRole := v1.MessageRole_MESSAGE_ROLE_USER
				setupMessageListMock(mockClient, nil, nil, &userRole, []*v1.Message{
					createTestMessage(messageID1, taskID1, agentID1, modelID1, "User message", v1.MessageRole_MESSAGE_ROLE_USER, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayMessage{
					{
						Id:        messageID1,
						TaskId:    taskID1,
						Agent:     agentID1,
						Model:     modelID1,
						Role:      "user",
						Content:   "User message",
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
		},
		{
			Name:    "success - list messages with JSON output",
			Command: []string{"message", "list", "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageListMock(mockClient, nil, nil, nil, []*v1.Message{
					createTestMessage(messageID1, taskID1, agentID1, modelID1, "User message", v1.MessageRole_MESSAGE_ROLE_USER, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatJSON,
				DisplayedObjects: []*DisplayMessage{
					{
						Id:        messageID1,
						TaskId:    taskID1,
						Agent:     agentID1,
						Model:     modelID1,
						Role:      "user",
						Content:   "User message",
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
		},
		{
			Name:    "success - list messages with multiple filters and short flags",
			Command: []string{"message", "list", "-a", "sql-expert", "-r", "assistant", "-o", "yaml"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentLookupForMessageListMock(mockClient, "sql-expert", agentID2)
				assistantRole := v1.MessageRole_MESSAGE_ROLE_ASSISTANT
				setupMessageListMock(mockClient, nil, &agentID2, &assistantRole, []*v1.Message{
					createTestMessage(messageID2, taskID2, agentID2, modelID1, "Assistant response", v1.MessageRole_MESSAGE_ROLE_ASSISTANT, createdAt, updatedAt),
				})
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatYAML,
				DisplayedObjects: []*DisplayMessage{
					{
						Id:        messageID2,
						TaskId:    taskID2,
						Agent:     agentID2,
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
		},
		{
			Name:    "success - empty message list",
			Command: []string{"message", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupMessageListMock(mockClient, nil, nil, nil, []*v1.Message{})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*DisplayMessage{},
			},
		},
		{
			Name:    "error - agent not found by name",
			Command: []string{"message", "list", "--agent", "nonexistent"},
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
			Name:    "error - list messages API failure",
			Command: []string{"message", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Message.EXPECT().ListMessages(
					gomock.Any(),
					&connect.Request[v1.ListMessagesRequest]{
						Msg: &v1.ListMessagesRequest{
							Filter: &v1.ListMessagesRequest_Filter{},
						},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to list messages: internal",
			},
		},
	})
}

func setupMessageListMock(mockClient *api_client.MockClient, taskID, agentID *string, role *v1.MessageRole, messages []*v1.Message) {
	filter := &v1.ListMessagesRequest_Filter{}
	if taskID != nil {
		filter.TaskId = taskID
	}
	if agentID != nil {
		filter.AgentId = agentID
	}
	if role != nil {
		filter.Role = role
	}

	mockClient.Message.EXPECT().ListMessages(
		gomock.Any(),
		&connect.Request[v1.ListMessagesRequest]{
			Msg: &v1.ListMessagesRequest{
				Filter: filter,
			},
		},
	).Return(&connect.Response[v1.ListMessagesResponse]{
		Msg: &v1.ListMessagesResponse{
			Messages: messages,
		},
	}, nil)
}

func setupAgentLookupForMessageListMock(mockClient *api_client.MockClient, agentName, agentID string) {
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

func createTestMessage(messageID, taskID, agentID, modelID, content string, role v1.MessageRole, createdAt, updatedAt time.Time) *v1.Message {
	return &v1.Message{
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
	}
}
