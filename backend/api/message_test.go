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
	_ "modernc.org/sqlite"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCreateMessage(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateMessageRequest, v1.CreateMessageResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateMessageRequest]) (*connect.Response[v1.CreateMessageResponse], error) {
			return client.Message().CreateMessage(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateMessageResponse{}, v1.Message{}, v1.MessageMetadata{}, v1.MessageSpec{}, v1.MessageStatus{}, v1.MessageUsage{}, v1.MessagePart{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.MessageMetadata{}, "id", "created_at", "updated_at"),
		},
	}

	taskID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.CreateMessageRequest, v1.CreateMessageResponse]{
		{
			Name: "invalid task ID",
			Request: &v1.CreateMessageRequest{
				TaskId: "not-a-valid-uuid",
				Content: []*v1.MessagePart{
					{
						Data: &v1.MessagePart_Text_{
							Text: &v1.MessagePart_Text{
								Content: "Test message content",
							},
						},
					},
				},
			},
			Expected: ServiceTestExpectation[v1.CreateMessageResponse]{
				Error: "invalid_argument: invalid task ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "task not found",
			Request: &v1.CreateMessageRequest{
				TaskId: taskID.String(),
				Content: []*v1.MessagePart{
					{
						Data: &v1.MessagePart_Text_{
							Text: &v1.MessagePart_Text{
								Content: "Test message content",
							},
						},
					},
				},
			},
			Expected: ServiceTestExpectation[v1.CreateMessageResponse]{
				Error: "not_found: task not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, uuid.New(), db, model).Build(ctx)

				test.NewTaskBuilder(t, taskID, db, agent).Build(ctx)
			},
			Request: &v1.CreateMessageRequest{
				TaskId: taskID.String(),
				Content: []*v1.MessagePart{
					{
						Data: &v1.MessagePart_Text_{
							Text: &v1.MessagePart_Text{
								Content: "Test message content",
							},
						},
					},
				},
			},
			Expected: ServiceTestExpectation[v1.CreateMessageResponse]{
				Response: v1.CreateMessageResponse{
					Message: &v1.Message{
						Metadata: &v1.MessageMetadata{
							TaskId: taskID.String(),
							Role:   v1.MessageRole_MESSAGE_ROLE_USER,
						},
						Spec: &v1.MessageSpec{
							Content: []*v1.MessagePart{
								{
									Data: &v1.MessagePart_Text_{
										Text: &v1.MessagePart_Text{
											Content: "Test message content",
										},
									},
								},
							},
						},
						Status: &v1.MessageStatus{},
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
			cmpopts.IgnoreUnexported(v1.GetMessageResponse{}, v1.Message{}, v1.MessageMetadata{}, v1.MessageSpec{}, v1.MessageStatus{}, v1.MessageUsage{}, v1.MessagePart{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.MessageMetadata{}, "created_at", "updated_at"),
		},
	}

	messageID := uuid.New()
	taskID := uuid.New()
	agentID := uuid.New()
	modelID := uuid.New()

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
				Id: messageID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetMessageResponse]{
				Error: "not_found: message not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, agentID, db, model).Build(ctx)
				task := test.NewTaskBuilder(t, taskID, db, agent).Build(ctx)

				test.NewMessageBuilder(t, messageID, db, task).
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
				Id: messageID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetMessageResponse]{
				Response: v1.GetMessageResponse{
					Message: &v1.Message{
						Metadata: &v1.MessageMetadata{
							Id:      messageID.String(),
							TaskId:  taskID.String(),
							AgentId: strPtr(agentID.String()),
							ModelId: strPtr(modelID.String()),
							Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
						},
						Spec: &v1.MessageSpec{
							Content: []*v1.MessagePart{
								{
									Data: &v1.MessagePart_Text_{
										Text: &v1.MessagePart_Text{
											Content: "Test message content",
										},
									},
								},
							},
						},
						Status: &v1.MessageStatus{},
					},
				},
			},
		},
	})
}

func TestListMessages(t *testing.T) {
	setup := ServiceTestSetup[v1.ListMessagesRequest, v1.ListMessagesResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.ListMessagesRequest]) (*connect.Response[v1.ListMessagesResponse], error) {
			return client.Message().ListMessages(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.ListMessagesResponse{}, v1.Message{}, v1.MessageMetadata{}, v1.MessageSpec{}, v1.MessageStatus{}, v1.MessageUsage{}, v1.MessagePart{}, v1.ListMessagesRequest_Filter{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.MessageMetadata{}, "created_at", "updated_at"),
		},
	}

	taskID1 := uuid.New()
	taskID2 := uuid.New()
	messageID1 := uuid.New()
	messageID2 := uuid.New()
	agentID1 := uuid.New()
	agentID2 := uuid.New()
	modelID := uuid.New()

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
					TaskIds: strPtr("not-a-valid-uuid"),
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
					AgentIds: strPtr("not-a-valid-uuid"),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Error: "invalid_argument: invalid agent ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "filter by task ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).Build(ctx)
				agent := test.NewAgentBuilder(t, agentID1, db, model).Build(ctx)

				task1 := test.NewTaskBuilder(t, taskID1, db, agent).Build(ctx)
				task2 := test.NewTaskBuilder(t, taskID2, db, agent).Build(ctx)

				test.NewMessageBuilder(t, messageID1, db, task1).Build(ctx)
				test.NewMessageBuilder(t, messageID2, db, task2).
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
					TaskIds: strPtr(taskID2.String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Metadata: &v1.MessageMetadata{
								Id:      messageID2.String(),
								TaskId:  taskID2.String(),
								AgentId: strPtr(agentID1.String()),
								ModelId: strPtr(modelID.String()),
								Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Spec: &v1.MessageSpec{
								Content: []*v1.MessagePart{
									{
										Data: &v1.MessagePart_Text_{
											Text: &v1.MessagePart_Text{
												Content: "Message 2 content",
											},
										},
									},
								},
							},
							Status: &v1.MessageStatus{},
						},
					},
				},
			},
		},
		{
			Name: "filter by agent ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).Build(ctx)

			agent1 := test.NewAgentBuilder(t, agentID1, db, model).
				WithName("agent-1").
				Build(ctx)
			agent2 := test.NewAgentBuilder(t, agentID2, db, model).
				WithName("agent-2").
				Build(ctx)
				task1 := test.NewTaskBuilder(t, taskID1, db, agent1).Build(ctx)

				test.NewMessageBuilder(t, messageID1, db, task1).
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

				test.NewMessageBuilder(t, messageID2, db, task1).
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
					AgentIds: strPtr(agentID2.String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Metadata: &v1.MessageMetadata{
								Id:      messageID2.String(),
								TaskId:  taskID1.String(),
								AgentId: strPtr(agentID2.String()),
								ModelId: strPtr(modelID.String()),
								Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Spec: &v1.MessageSpec{
								Content: []*v1.MessagePart{
									{
										Data: &v1.MessagePart_Text_{
											Text: &v1.MessagePart_Text{
												Content: "Message 2 content",
											},
										},
									},
								},
							},
							Status: &v1.MessageStatus{},
						},
					},
				},
			},
		},
		{
			Name: "filter by role",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, agentID1, db, model).Build(ctx)

				task := test.NewTaskBuilder(t, taskID1, db, agent).Build(ctx)

				test.NewMessageBuilder(t, messageID1, db, task).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 1 content",
							},
						},
					}).
					Build(ctx)

				test.NewMessageBuilder(t, messageID2, db, task).
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
					Roles: rolePtr(v1.MessageRole_MESSAGE_ROLE_ASSISTANT),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Metadata: &v1.MessageMetadata{
								Id:      messageID2.String(),
								TaskId:  taskID1.String(),
								AgentId: strPtr(agentID1.String()),
								ModelId: strPtr(modelID.String()),
								Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Spec: &v1.MessageSpec{
								Content: []*v1.MessagePart{
									{
										Data: &v1.MessagePart_Text_{
											Text: &v1.MessagePart_Text{
												Content: "Message 2 content",
											},
										},
									},
								},
							},
							Status: &v1.MessageStatus{},
						},
					},
				},
			},
		},
		{
			Name: "all messages",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, agentID1, db, model).Build(ctx)

				task1 := test.NewTaskBuilder(t, taskID1, db, agent).Build(ctx)
				task2 := test.NewTaskBuilder(t, taskID2, db, agent).Build(ctx)

				test.NewMessageBuilder(t, messageID1, db, task1).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageBlock{
							{
								Kind:    types.MessageBlockKindText,
								Payload: "Message 1 content",
							},
						},
					}).
					Build(ctx)

				test.NewMessageBuilder(t, messageID2, db, task2).
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
							Metadata: &v1.MessageMetadata{
								Id:     messageID1.String(),
								TaskId: taskID1.String(),
								Role:   v1.MessageRole_MESSAGE_ROLE_USER,
							},
							Spec: &v1.MessageSpec{
								Content: []*v1.MessagePart{
									{
										Data: &v1.MessagePart_Text_{
											Text: &v1.MessagePart_Text{
												Content: "Message 1 content",
											},
										},
									},
								},
							},
							Status: &v1.MessageStatus{},
						},
						{
							Metadata: &v1.MessageMetadata{
								Id:      messageID2.String(),
								TaskId:  taskID2.String(),
								AgentId: strPtr(agentID1.String()),
								ModelId: strPtr(modelID.String()),
								Role:    v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Spec: &v1.MessageSpec{
								Content: []*v1.MessagePart{
									{
										Data: &v1.MessagePart_Text_{
											Text: &v1.MessagePart_Text{
												Content: "Message 2 content",
											},
										},
									},
								},
							},
							Status: &v1.MessageStatus{},
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
			cmpopts.IgnoreUnexported(v1.UpdateMessageResponse{}, v1.Message{}, v1.MessageMetadata{}, v1.MessageSpec{}, v1.MessageStatus{}, v1.MessageUsage{}, v1.MessagePart{}),
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
				Id: "not-a-valid-uuid",
				Content: []*v1.MessagePart{
					{
						Data: &v1.MessagePart_Text_{
							Text: &v1.MessagePart_Text{
								Content: "Updated content",
							},
						},
					},
				},
			},
			Expected: ServiceTestExpectation[v1.UpdateMessageResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "message not found",
			Request: &v1.UpdateMessageRequest{
				Id: messageID.String(),
				Content: []*v1.MessagePart{
					{
						Data: &v1.MessagePart_Text_{
							Text: &v1.MessagePart_Text{
								Content: "Updated content",
							},
						},
					},
				},
			},
			Expected: ServiceTestExpectation[v1.UpdateMessageResponse]{
				Error: "not_found: message not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, uuid.New(), db, model).Build(ctx)

				task := test.NewTaskBuilder(t, taskID, db, agent).Build(ctx)

				test.NewMessageBuilder(t, messageID, db, task).
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
				Id: messageID.String(),
				Content: []*v1.MessagePart{
					{
						Data: &v1.MessagePart_Text_{
							Text: &v1.MessagePart_Text{
								Content: "Updated content",
							},
						},
					},
				},
			},
			Expected: ServiceTestExpectation[v1.UpdateMessageResponse]{
				Response: v1.UpdateMessageResponse{
					Message: &v1.Message{
						Metadata: &v1.MessageMetadata{
							Id:     messageID.String(),
							TaskId: taskID.String(),
							Role:   v1.MessageRole_MESSAGE_ROLE_USER,
						},
						Spec: &v1.MessageSpec{
							Content: []*v1.MessagePart{
								{
									Data: &v1.MessagePart_Text_{
										Text: &v1.MessagePart_Text{
											Content: "Updated content",
										},
									},
								},
							},
						},
						Status: &v1.MessageStatus{},
					},
				},
			},
		},
	})
}

func TestDeleteMessage(t *testing.T) {
	t.Parallel()
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
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				agent := test.NewAgentBuilder(t, uuid.New(), db, model).Build(ctx)

				task := test.NewTaskBuilder(t, taskID, db, agent).Build(ctx)

				test.NewMessageBuilder(t, messageID, db, task).
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
