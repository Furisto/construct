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
	agentID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")
	modelID := uuid.MustParse("abcdef01-2345-6789-abcd-ef0123456789")

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
			SeedDatabase: func(ctx context.Context, db *memory.Client)  {
				modelProvider, err := db.ModelProvider.Create().
					SetName("test-model-provider").
					SetProviderType(types.ModelProviderTypeOpenAI).
					SetSecret([]byte("test-secret")).
					Save(ctx)
				if err != nil {
					return err
				}

				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					SetModelProvider(modelProvider).
					Save(ctx)
				if err != nil {
					return err
				}

				agent, err := db.Agent.Create().
					SetID(agentID).
					SetName("test-agent").
					SetInstructions("Test instructions").
					SetModel(model).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Task.Create().
					SetID(taskID).
					SetAgent(agent).
					Save(ctx)
				return err
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

	messageID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	taskID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")
	agentID := uuid.MustParse("abcdef01-2345-6789-abcd-ef0123456789")
	modelID := uuid.MustParse("12345678-90ab-cdef-0123-456789abcdef")
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
			SeedDatabase: func(ctx context.Context, db *memory.Client)  {
				task, err := db.Task.Create().
					SetID(taskID).
					Save(ctx)
				if err != nil {
					return err
				}

				modelProvider, err := db.ModelProvider.Create().
					SetName("test-model-provider").
					SetProviderType(types.ModelProviderTypeOpenAI).
					SetSecret([]byte("test-secret")).
					Save(ctx)
				if err != nil {
					return err
				}

				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					SetModelProvider(modelProvider).
					Save(ctx)
				if err != nil {
					return err
				}

				agent, err := db.Agent.Create().
					SetID(agentID).
					SetName("test-agent").
					SetInstructions("Test instructions").
					SetModel(model).
					Save(ctx)
				if err != nil {
					return err
				}

				content := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Test message content",
						},
					},
				}

				_, err = db.Message.Create().
					SetID(messageID).
					SetTask(task).
					SetAgent(agent).
					SetModel(model).
					SetContent(content).
					SetRole(types.MessageRoleUser).
					Save(ctx)
				return err
			},
			Request: &v1.GetMessageRequest{
				Id: messageID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetMessageResponse]{
				Response: v1.GetMessageResponse{
					Message: &v1.Message{
						Id: messageID.String(),
						Metadata: &v1.MessageMetadata{
							TaskId:  taskID.String(),
							AgentId: strPtr(agentID.String()),
							ModelId: strPtr(modelID.String()),
							Role:    v1.MessageRole_MESSAGE_ROLE_USER,
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

	message2ID := uuid.MustParse("0195fbbd-d9ad-7ed1-9c05-171114d5a559")
	task1ID := uuid.MustParse("0195fbbe-0be8-74b1-af7a-6e76e80e2462")
	task2ID := uuid.MustParse("0195fbbe-42e1-75fe-8e08-28758035ff95")
	agent1ID := uuid.MustParse("0195fbbe-42e1-75fe-8e08-28758035ff95")
	agent2ID := uuid.MustParse("0195fbbe-8321-7800-b5cb-8012f8c36734")
	modelID := uuid.MustParse("0195fbbe-adda-76cf-be67-9f1b64b50a4a")

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
			SeedDatabase: func(ctx context.Context, db *memory.Client)  {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)
				agent := test.NewAgentBuilder(t, db, model).Build(ctx)

				task1 := test.NewTaskBuilder(t, db, agent).Build(ctx)
				task2 := test.NewTaskBuilder(t, db, agent).
					WithID(task2ID).
					Build(ctx)

				test.NewMessageBuilder(t, db, task1).Build(ctx)
				test.NewMessageBuilder(t, db, task2).
					WithAgent(agent).
					WithContent(&types.MessageContent{
						Blocks: []types.MessageContentBlock{
							{
								Type: types.MessageContentBlockTypeText,
								Text: "Message 2 content",
							},
						},
					}).
					Build(ctx)
			},
			Request: &v1.ListMessagesRequest{
				Filter: &v1.ListMessagesRequest_Filter{
					TaskId: strPtr(task2ID.String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Id: message2ID.String(),
							Metadata: &v1.MessageMetadata{
								TaskId:  task2ID.String(),
								AgentId: strPtr(agent1ID.String()),
								ModelId: strPtr(modelID.String()),
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
			SeedDatabase: func(ctx context.Context, db *memory.Client)  {
				task1, err := db.Task.Create().
					SetID(task1ID).
					Save(ctx)
				if err != nil {
					return err
				}

				content1 := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Message 1 content",
						},
					},
				}

				content2 := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Message 2 content",
						},
					},
				}

				modelProvider, err := db.ModelProvider.Create().
					SetName("test-model-provider").
					SetProviderType(types.ModelProviderTypeOpenAI).
					SetSecret([]byte("test-secret")).
					Save(ctx)
				if err != nil {
					return err
				}

				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					SetModelProvider(modelProvider).
					Save(ctx)
				if err != nil {
					return err
				}

				agent1, err := db.Agent.Create().
					SetID(agent1ID).
					SetName("test-agent").
					SetInstructions("Test instructions").
					SetModel(model).
					Save(ctx)
				if err != nil {
					return err
				}

				agent2, err := db.Agent.Create().
					SetID(agent2ID).
					SetName("test-agent").
					SetInstructions("Test instructions").
					SetModel(model).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Message.Create().
					SetID(message1ID).
					SetTask(task1).
					SetAgent(agent1).
					SetModel(model).
					SetContent(content1).
					SetRole(types.MessageRoleAssistant).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Message.Create().
					SetID(message2ID).
					SetTask(task1).
					SetAgent(agent2).
					SetModel(model).
					SetContent(content2).
					SetRole(types.MessageRoleAssistant).
					Save(ctx)
				return err
			},
			Request: &v1.ListMessagesRequest{
				Filter: &v1.ListMessagesRequest_Filter{
					AgentId: strPtr(agent2ID.String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Id: message2ID.String(),
							Metadata: &v1.MessageMetadata{
								TaskId:  task1ID.String(),
								AgentId: strPtr(agent2ID.String()),
								ModelId: strPtr(modelID.String()),
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
			SeedDatabase: func(ctx context.Context, db *memory.Client)  {
				task, err := db.Task.Create().
					SetID(task1ID).
					Save(ctx)
				if err != nil {
					return err
				}

				modelProvider, err := db.ModelProvider.Create().
					SetName("test-model-provider").
					SetProviderType(types.ModelProviderTypeOpenAI).
					SetSecret([]byte("test-secret")).
					Save(ctx)
				if err != nil {
					return err
				}

				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					SetModelProvider(modelProvider).
					Save(ctx)
				if err != nil {
					return err
				}

				agent, err := db.Agent.Create().
					SetID(agent1ID).
					SetName("test-agent").
					SetInstructions("Test instructions").
					SetModel(model).
					Save(ctx)
				if err != nil {
					return err
				}

				content1 := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Message 1 content",
						},
					},
				}

				content2 := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Message 2 content",
						},
					},
				}

				_, err = db.Message.Create().
					SetID(message1ID).
					SetTask(task).
					SetContent(content1).
					SetRole(types.MessageRoleUser).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Message.Create().
					SetID(message2ID).
					SetTask(task).
					SetAgent(agent).
					SetModel(model).
					SetContent(content2).
					SetRole(types.MessageRoleAssistant).
					Save(ctx)
				return err
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
							Id: message2ID.String(),
							Metadata: &v1.MessageMetadata{
								TaskId:  task1ID.String(),
								AgentId: strPtr(agent1ID.String()),
								ModelId: strPtr(modelID.String()),
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
			SeedDatabase: func(ctx context.Context, db *memory.Client)  {
				task1, err := db.Task.Create().
					SetID(task1ID).
					Save(ctx)
				if err != nil {
					return err
				}

				task2, err := db.Task.Create().
					SetID(task2ID).
					Save(ctx)
				if err != nil {
					return err
				}

				content1 := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Message 1 content",
						},
					},
				}

				content2 := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Message 2 content",
						},
					},
				}

				_, err = db.Message.Create().
					SetID(message1ID).
					SetTask(task1).
					SetContent(content1).
					SetRole(types.MessageRoleUser).
					Save(ctx)
				if err != nil {
					return err
				}

				modelProvider, err := db.ModelProvider.Create().
					SetName("test-model-provider").
					SetProviderType(types.ModelProviderTypeOpenAI).
					SetSecret([]byte("test-secret")).
					Save(ctx)
				if err != nil {
					return err
				}

				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					SetModelProvider(modelProvider).
					Save(ctx)
				if err != nil {
					return err
				}

				agent, err := db.Agent.Create().
					SetID(agent1ID).
					SetName("test-agent").
					SetInstructions("Test instructions").
					SetModel(model).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Message.Create().
					SetID(message2ID).
					SetTask(task2).
					SetAgent(agent).
					SetModel(model).
					SetContent(content2).
					SetRole(types.MessageRoleAssistant).
					Save(ctx)
				return err
			},
			Request: &v1.ListMessagesRequest{},
			Expected: ServiceTestExpectation[v1.ListMessagesResponse]{
				Response: v1.ListMessagesResponse{
					Messages: []*v1.Message{
						{
							Id: message1ID.String(),
							Metadata: &v1.MessageMetadata{
								TaskId: task1ID.String(),
								Role:   v1.MessageRole_MESSAGE_ROLE_USER,
							},
							Content: &v1.MessageContent{
								Content: &v1.MessageContent_Text{
									Text: "Message 1 content",
								},
							},
						},
						{
							Id: message2ID.String(),
							Metadata: &v1.MessageMetadata{
								TaskId:  task2ID.String(),
								AgentId: strPtr(agent1ID.String()),
								ModelId: strPtr(modelID.String()),
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
			SeedDatabase: func(ctx context.Context, db *memory.Client)  {
				task, err := db.Task.Create().
					SetID(taskID).
					Save(ctx)
				if err != nil {
					return err
				}

				content := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Original content",
						},
					},
				}

				_, err = db.Message.Create().
					SetID(messageID).
					SetTask(task).
					SetContent(content).
					SetRole(types.MessageRoleUser).
					Save(ctx)
				return err
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
			SeedDatabase: func(ctx context.Context, db *memory.Client)  {
				task, err := db.Task.Create().
					SetID(taskID).
					Save(ctx)
				if err != nil {
					return err
				}

				content := &types.MessageContent{
					Blocks: []types.MessageContentBlock{
						{
							Type: types.MessageContentBlockTypeText,
							Text: "Message content",
						},
					},
				}

				_, err = db.Message.Create().
					SetID(messageID).
					SetTask(task).
					SetContent(content).
					SetRole(types.MessageRoleUser).
					Save(ctx)
				return err
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
