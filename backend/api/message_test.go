package api

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/memory/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCreateMessage(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateMessageRequest, v1.CreateMessageResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateMessageRequest]) (*connect.Response[v1.CreateMessageResponse], error) {
			return client.Message().CreateMessage(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateMessageResponse{}, v1.Message{}, v1.MessageMetadata{}, v1.MessageUsage{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.Message{}, "id"),
			protocmp.IgnoreFields(&v1.MessageMetadata{}, "created_at", "updated_at"),
		},
	}

	taskID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.CreateMessageRequest, v1.CreateMessageResponse]{
		{
			Name: "invalid task ID",
			Request: &v1.CreateMessageRequest{
				TaskId:  "not-a-valid-uuid",
				Content: "Test message content",
			},
			Expected: ServiceTestExpectation[v1.CreateMessageResponse]{
				Error: "invalid_argument: invalid task ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "task not found",
			Request: &v1.CreateMessageRequest{
				TaskId:  taskID.String(),
				Content: "Test message content",
			},
			Expected: ServiceTestExpectation[v1.CreateMessageResponse]{
				Error: "not_found: task not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, db, model).Build(ctx)

				test.NewTaskBuilder(t, db, agent).
					WithID(taskID).
					Build(ctx)
			},
			Request: &v1.CreateMessageRequest{
				TaskId:  taskID.String(),
				Content: "Test message content",
			},
			Expected: ServiceTestExpectation[v1.CreateMessageResponse]{
				Response: v1.CreateMessageResponse{
					Message: &v1.Message{
						Metadata: &v1.MessageMetadata{
							TaskId: taskID.String(),
							Role:   v1.MessageRole_MESSAGE_ROLE_USER,
						},
						Content: &v1.MessageContent{
							Content: &v1.MessageContent_Text{
								Text: "Test message content",
							},
						},
					},
				},
			},
		},
	})
}

func TestGetMessage(t *testing.T) {
	setup := ServiceTestSetup[v1.GetMessageRequest, v1.GetMessageResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.GetMessageRequest]) (*connect.Response[v1.GetMessageResponse], error) {
			return client.Message().GetMessage(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.GetMessageResponse{}, v1.Message{}, v1.MessageMetadata{}, v1.MessageUsage{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.MessageMetadata{}, "created_at", "updated_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.GetMessageRequest, v1.GetMessageResponse]{
		{
			Name: "invalid id format",
			Request: &v1.GetMessageRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.GetMessageResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "message not found",
			Request: &v1.GetMessageRequest{
				Id: test.MessageID().String(),
			},
			Expected: ServiceTestExpectation[v1.GetMessageResponse]{
				Error: "not_found: message not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, db, model).Build(ctx)
				task := test.NewTaskBuilder(t, db, agent).Build(ctx)

				test.NewMessageBuilder(t, db, task).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Test message content",
							},
						},
					}).
					WithAgent(agent).
					Build(ctx)
			},
			Request: &v1.GetMessageRequest{
				Id: test.MessageID().String(),
			},
			Expected: ServiceTestExpectation[v1.GetMessageResponse]{
				Response: v1.GetMessageResponse{
					Message: &v1.Message{
						Id: test.MessageID().String(),
						Metadata: &v1.MessageMetadata{
							TaskId:  test.TaskID().String(),
							AgentId: strPtr(test.AgentID().String()),
							ModelId: strPtr(test.ModelID().String()),
							Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
						},
						Content: &v1.MessageContent{
							Content: &v1.MessageContent_Text{
								Text: "Test message content",
							},
						},
					},
				},
			},
		},
	})
}

func TestListMessages(t *testing.T) {
	t.Parallel()

	setup := ServiceTestSetup[v1.ListMessagesRequest, v1.ListMessagesResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.ListMessagesRequest]) (*connect.Response[v1.ListMessagesResponse], error) {
			return client.Message().ListMessages(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.ListMessagesResponse{}, v1.Message{}, v1.MessageMetadata{}, v1.MessageUsage{}, v1.ListMessagesRequest_Filter{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.MessageMetadata{}, "created_at", "updated_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.ListMessagesRequest, v1.ListMessagesResponse]{
		{
			Name:    "empty list",
			Request: &v1.ListMessagesRequest{},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{},
				},
			},
		},
		{
			Name: "filter by task ID - invalid format",
			Request: &v1.ListMessagesRequest{
				Filter: &v1.ListMessagesRequest_Filter{
					TaskId: strPtr("not-a-valid-uuid"),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Error: "invalid_argument: invalid task ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "filter by agent ID - invalid format",
			Request: &v1.ListMessagesRequest{
				Filter: &v1.ListMessagesRequest_Filter{
					AgentId: strPtr("not-a-valid-uuid"),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Error: "invalid_argument: invalid agent ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "filter by task ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)
				agent := test.NewAgentBuilder(t, db, model).Build(ctx)

				task1 := test.NewTaskBuilder(t, db, agent).Build(ctx)
				task2 := test.NewTaskBuilder(t, db, agent).
					WithID(test.TaskID2()).
					Build(ctx)

				test.NewMessageBuilder(t, db, task1).Build(ctx)
				test.NewMessageBuilder(t, db, task2).
					WithID(test.MessageID2()).
					WithAgent(agent).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 2 content",
							},
						},
					}).
					Build(ctx)
			},
			Request: &v1.ListMessagesRequest{
				Filter: &v1.ListMessagesRequest_Filter{
					TaskId: strPtr(test.TaskID2().String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Id: test.MessageID2().String(),
							Metadata: &v1.MessageMetadata{
								TaskId:  test.TaskID2().String(),
								AgentId: strPtr(test.AgentID().String()),
								ModelId: strPtr(test.ModelID().String()),
								Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Content: &v1.MessageContent{
								Content: &v1.MessageContent_Text{
									Text: "Message 2 content",
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "filter by agent ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

				agent1 := test.NewAgentBuilder(t, db, model).Build(ctx)
				agent2 := test.NewAgentBuilder(t, db, model).
					WithID(test.AgentID2()).
					Build(ctx)

				task1 := test.NewTaskBuilder(t, db, agent1).
					WithID(test.TaskID()).
					Build(ctx)

				test.NewMessageBuilder(t, db, task1).
					WithID(test.MessageID()).
					WithAgent(agent1).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 1 content",
							},
						},
					}).
					Build(ctx)

				test.NewMessageBuilder(t, db, task1).
					WithID(test.MessageID2()).
					WithAgent(agent2).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 2 content",
							},
						},
					}).
					Build(ctx)
			},
			Request: &v1.ListMessagesRequest{
				Filter: &v1.ListMessagesRequest_Filter{
					AgentId: strPtr(test.AgentID2().String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Id: test.MessageID2().String(),
							Metadata: &v1.MessageMetadata{
								TaskId:  test.TaskID().String(),
								AgentId: strPtr(test.AgentID2().String()),
								ModelId: strPtr(test.ModelID().String()),
								Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Content: &v1.MessageContent{
								Content: &v1.MessageContent_Text{
									Text: "Message 2 content",
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "filter by role",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, db, model).Build(ctx)

				task := test.NewTaskBuilder(t, db, agent).
					WithID(test.TaskID()).
					Build(ctx)

				test.NewMessageBuilder(t, db, task).
					WithID(test.MessageID()).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 1 content",
							},
						},
					}).
					Build(ctx)

				test.NewMessageBuilder(t, db, task).
					WithID(test.MessageID2()).
					WithAgent(agent).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 2 content",
							},
						},
					}).
					Build(ctx)
			},
			Request: &v1.ListMessagesRequest{
				Filter: &v1.ListMessagesRequest_Filter{
					Role: rolePtr(v1.MessageRole_MESSAGE_ROLE_ASSISTANT),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Id: test.MessageID2().String(),
							Metadata: &v1.MessageMetadata{
								TaskId:  test.TaskID().String(),
								AgentId: strPtr(test.AgentID().String()),
								ModelId: strPtr(test.ModelID().String()),
								Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Content: &v1.MessageContent{
								Content: &v1.MessageContent_Text{
									Text: "Message 2 content",
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "all messages",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, db, model).Build(ctx)

				task1 := test.NewTaskBuilder(t, db, agent).
					WithID(test.TaskID()).
					Build(ctx)

				task2 := test.NewTaskBuilder(t, db, agent).
					WithID(test.TaskID2()).
					Build(ctx)

				test.NewMessageBuilder(t, db, task1).
					WithID(test.MessageID()).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 1 content",
							},
						},
					}).
					Build(ctx)

				test.NewMessageBuilder(t, db, task2).
					WithID(test.MessageID2()).
					WithAgent(agent).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 2 content",
							},
						},
					}).
					Build(ctx)
			},
			Request: &v1.ListMessagesRequest{},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Id: test.MessageID().String(),
							Metadata: &v1.MessageMetadata{
								TaskId: test.TaskID().String(),
								Role:   v1.MessageRole_MESSAGE_ROLE_USER,
							},
							Content: &v1.MessageContent{
								Content: &v1.MessageContent_Text{
									Text: "Message 1 content",
								},
							},
						},
						{
							Id: test.MessageID2().String(),
							Metadata: &v1.MessageMetadata{
								TaskId:  test.TaskID2().String(),
								AgentId: strPtr(test.AgentID().String()),
								ModelId: strPtr(test.ModelID().String()),
								Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Content: &v1.MessageContent{
								Content: &v1.MessageContent_Text{
									Text: "Message 2 content",
								},
							},
						},
					},
				},
			},
		},
	})
}

func TestUpdateMessage(t *testing.T) {
	setup := ServiceTestSetup[v1.UpdateMessageRequest, v1.UpdateMessageResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.UpdateMessageRequest]) (*connect.Response[v1.UpdateMessageResponse], error) {
			return client.Message().UpdateMessage(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.UpdateMessageResponse{}, v1.Message{}, v1.MessageMetadata{}, v1.MessageUsage{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.MessageMetadata{}, "created_at", "updated_at"),
		},
	}

	messageID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	taskID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.UpdateMessageRequest, v1.UpdateMessageResponse]{
		{
			Name: "invalid id format",
			Request: &v1.UpdateMessageRequest{
				Id:      "not-a-valid-uuid",
				Content: "Updated content",
			},
			Expected: ServiceTestExpectation[v1.UpdateMessageResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "message not found",
			Request: &v1.UpdateMessageRequest{
				Id:      messageID.String(),
				Content: "Updated content",
			},
			Expected: ServiceTestExpectation[v1.UpdateMessageResponse]{
				Error: "not_found: message not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, db, model).Build(ctx)

				task := test.NewTaskBuilder(t, db, agent).
					WithID(taskID).
					Build(ctx)

				test.NewMessageBuilder(t, db, task).
					WithID(messageID).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Original content",
							},
						},
					}).
					Build(ctx)
			},
			Request: &v1.UpdateMessageRequest{
				Id:      messageID.String(),
				Content: "Updated content",
			},
			Expected: ServiceTestExpectation[v1.UpdateMessageResponse]{
				Response: v1.UpdateMessageResponse{
					Message: &v1.Message{
						Id: messageID.String(),
						Metadata: &v1.MessageMetadata{
							TaskId: taskID.String(),
							Role:   v1.MessageRole_MESSAGE_ROLE_USER,
						},
						Content: &v1.MessageContent{
							Content: &v1.MessageContent_Text{
								Text: "Updated content",
							},
						},
					},
				},
			},
		},
	})
}

func TestDeleteMessage(t *testing.T) {
	setup := ServiceTestSetup[v1.DeleteMessageRequest, v1.DeleteMessageResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.DeleteMessageRequest]) (*connect.Response[v1.DeleteMessageResponse], error) {
			return client.Message().DeleteMessage(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.DeleteMessageResponse{}),
			protocmp.Transform(),
		},
	}

	messageID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	taskID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.DeleteMessageRequest, v1.DeleteMessageResponse]{
		{
			Name: "invalid id format",
			Request: &v1.DeleteMessageRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.DeleteMessageResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "message not found",
			Request: &v1.DeleteMessageRequest{
				Id: messageID.String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteMessageResponse]{
				Error: "not_found: message not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, db, model).Build(ctx)

				task := test.NewTaskBuilder(t, db, agent).
					WithID(taskID).
					Build(ctx)

				test.NewMessageBuilder(t, db, task).
					WithID(messageID).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message content",
							},
						},
					}).
					Build(ctx)
			},
			Request: &v1.DeleteMessageRequest{
				Id: messageID.String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteMessageResponse]{
				Response: v1.DeleteMessageResponse{},
			},
		},
	})
}

func rolePtr(r v1.MessageRole) *v1.MessageRole {
	return &r
}
