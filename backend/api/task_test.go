package api

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCreateTask(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateTaskRequest, v1.CreateTaskResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateTaskRequest]) (*connect.Response[v1.CreateTaskResponse], error) {
			return client.Task().CreateTask(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateTaskResponse{}, v1.Task{}, v1.TaskMetadata{}, v1.TaskSpec{}, v1.TaskStatus{}, v1.TaskUsage{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.Task{}, "id"),
			protocmp.IgnoreFields(&v1.TaskMetadata{}, "created_at", "updated_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.CreateTaskRequest, v1.CreateTaskResponse]{
		{
			Name:    "success",
			Request: &v1.CreateTaskRequest{},
			Expected: ServiceTestExpectation[v1.CreateTaskResponse]{
				Response: v1.CreateTaskResponse{
					Task: &v1.Task{
						Metadata: &v1.TaskMetadata{},
						Spec:     &v1.TaskSpec{},
						Status:   &v1.TaskStatus{Usage: &v1.TaskUsage{}},
					},
				},
			},
		},
	})
}

func TestGetTask(t *testing.T) {
	setup := ServiceTestSetup[v1.GetTaskRequest, v1.GetTaskResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.GetTaskRequest]) (*connect.Response[v1.GetTaskResponse], error) {
			return client.Task().GetTask(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.GetTaskResponse{}, v1.Task{}, v1.TaskMetadata{}, v1.TaskSpec{}, v1.TaskStatus{}, v1.TaskUsage{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.TaskMetadata{}, "created_at", "updated_at"),
		},
	}

	taskID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	agentID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")
	modelID := uuid.MustParse("abcdef01-2345-6789-abcd-ef0123456789")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.GetTaskRequest, v1.GetTaskResponse]{
		{
			Name: "invalid id format",
			Request: &v1.GetTaskRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.GetTaskResponse]{
				Error: "invalid_argument: invalid task ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "task not found",
			Request: &v1.GetTaskRequest{
				Id: taskID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetTaskResponse]{
				Error: "not_found: task not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).
					WithID(modelID).
					Build(ctx)
				
				agent := test.NewAgentBuilder(t, db, model).
					WithID(agentID).
					WithName("test-agent").
					WithDescription("Test agent description").
					WithInstructions("Test agent instructions").
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent).
					WithID(taskID).
					Build(ctx)
			},
			Request: &v1.GetTaskRequest{
				Id: taskID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetTaskResponse]{
				Response: v1.GetTaskResponse{
					Task: &v1.Task{
						Id:       taskID.String(),
						Metadata: &v1.TaskMetadata{},
						Spec: &v1.TaskSpec{
							AgentId: strPtr(agentID.String()),
						},
						Status: &v1.TaskStatus{
							Usage: &v1.TaskUsage{},
						},
					},
				},
			},
		},
	})
}

func TestListTasks(t *testing.T) {
	setup := ServiceTestSetup[v1.ListTasksRequest, v1.ListTasksResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.ListTasksRequest]) (*connect.Response[v1.ListTasksResponse], error) {
			return client.Task().ListTasks(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.ListTasksResponse{}, v1.Task{}, v1.TaskMetadata{}, v1.TaskSpec{}, v1.TaskStatus{}, v1.TaskUsage{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.TaskMetadata{}, "created_at", "updated_at"),
		},
	}

	task1ID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	task2ID := uuid.MustParse("fedcba98-7654-3210-fedc-ba9876543210")
	agent1ID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")
	modelID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.ListTasksRequest, v1.ListTasksResponse]{
		{
			Name:    "empty list",
			Request: &v1.ListTasksRequest{},
			Expected: ServiceTestExpectation[v1.ListTasksResponse]{
				Response: v1.ListTasksResponse{
					Tasks: []*v1.Task{},
				},
			},
		},
		{
			Name: "filter by agent ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).
					WithID(modelID).
					Build(ctx)
				
				agent1 := test.NewAgentBuilder(t, db, model).
					WithID(agent1ID).
					WithName("test-agent-1").
					WithDescription("Test agent 1 description").
					WithInstructions("Test agent 1 instructions").
					Build(ctx)
				
				agent2 := test.NewAgentBuilder(t, db, model).
					WithID(uuid.New()).
					WithName("test-agent-2").
					WithDescription("Test agent 2 description").
					WithInstructions("Test agent 2 instructions").
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent1).
					WithID(task1ID).
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent2).
					WithID(task2ID).
					Build(ctx)
			},
			Request: &v1.ListTasksRequest{
				Filter: &v1.ListTasksRequest_Filter{
					AgentId: strPtr(agent1ID.String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListTasksResponse]{
				Response: v1.ListTasksResponse{
					Tasks: []*v1.Task{
						{
							Id:       task1ID.String(),
							Metadata: &v1.TaskMetadata{},
							Spec: &v1.TaskSpec{
								AgentId: strPtr(agent1ID.String()),
							},
							Status: &v1.TaskStatus{
								Usage: &v1.TaskUsage{},
							},
						},
					},
				},
			},
		},
		{
			Name: "invalid agent ID format",
			Request: &v1.ListTasksRequest{
				Filter: &v1.ListTasksRequest_Filter{
					AgentId: strPtr("not-a-valid-uuid"),
				},
			},
			Expected: ServiceTestExpectation[v1.ListTasksResponse]{
				Error: "invalid_argument: invalid agent ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "multiple tasks",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).
					WithID(modelID).
					Build(ctx)
				
				agent1 := test.NewAgentBuilder(t, db, model).
					WithID(agent1ID).
					WithName("test-agent-1").
					WithDescription("Test agent 1 description").
					WithInstructions("Test agent 1 instructions").
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent1).
					WithID(task1ID).
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent1).
					WithID(task2ID).
					Build(ctx)
			},
			Request: &v1.ListTasksRequest{},
			Expected: ServiceTestExpectation[v1.ListTasksResponse]{
				Response: v1.ListTasksResponse{
					Tasks: []*v1.Task{
						{
							Id:       task1ID.String(),
							Metadata: &v1.TaskMetadata{},
							Spec: &v1.TaskSpec{
								AgentId: strPtr(agent1ID.String()),
							},
							Status: &v1.TaskStatus{
								Usage: &v1.TaskUsage{},
							},
						},
						{
							Id:       task2ID.String(),
							Metadata: &v1.TaskMetadata{},
							Spec: &v1.TaskSpec{
								AgentId: strPtr(agent1ID.String()),
							},
							Status: &v1.TaskStatus{
								Usage: &v1.TaskUsage{},
							},
						},
					},
				},
			},
		},
	})
}

func TestUpdateTask(t *testing.T) {
	setup := ServiceTestSetup[v1.UpdateTaskRequest, v1.UpdateTaskResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.UpdateTaskRequest]) (*connect.Response[v1.UpdateTaskResponse], error) {
			return client.Task().UpdateTask(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.UpdateTaskResponse{}, v1.Task{}, v1.TaskMetadata{}, v1.TaskSpec{}, v1.TaskStatus{}, v1.TaskUsage{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.TaskMetadata{}, "created_at", "updated_at"),
		},
	}

	taskID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	agentID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")
	newAgentID := uuid.MustParse("abcdef01-2345-6789-abcd-ef0123456789")
	modelID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.UpdateTaskRequest, v1.UpdateTaskResponse]{
		{
			Name: "invalid task ID format",
			Request: &v1.UpdateTaskRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.UpdateTaskResponse]{
				Error: "invalid_argument: invalid task ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "task not found",
			Request: &v1.UpdateTaskRequest{
				Id:      taskID.String(),
				AgentId: strPtr(agentID.String()),
			},
			Expected: ServiceTestExpectation[v1.UpdateTaskResponse]{
				Error: "not_found: task not found",
			},
		},
		{
			Name: "invalid agent ID format",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).
					WithID(modelID).
					Build(ctx)
				
				agent := test.NewAgentBuilder(t, db, model).
					WithID(agentID).
					WithName("test-agent").
					WithDescription("Test agent description").
					WithInstructions("Test agent instructions").
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent).
					WithID(taskID).
					Build(ctx)
			},
			Request: &v1.UpdateTaskRequest{
				Id:      taskID.String(),
				AgentId: strPtr("not-a-valid-uuid"),
			},
			Expected: ServiceTestExpectation[v1.UpdateTaskResponse]{
				Error: "invalid_argument: invalid agent ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "agent not found",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).
					WithID(modelID).
					Build(ctx)
				
				agent := test.NewAgentBuilder(t, db, model).
					WithID(agentID).
					WithName("test-agent").
					WithDescription("Test agent description").
					WithInstructions("Test agent instructions").
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent).
					WithID(taskID).
					Build(ctx)
			},
			Request: &v1.UpdateTaskRequest{
				Id:      taskID.String(),
				AgentId: strPtr(newAgentID.String()),
			},
			Expected: ServiceTestExpectation[v1.UpdateTaskResponse]{
				Error: "not_found: agent not found",
			},
		},
		{
			Name: "success - update agent",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).
					WithID(modelID).
					Build(ctx)
				
				agent1 := test.NewAgentBuilder(t, db, model).
					WithID(agentID).
					WithName("test-agent").
					WithDescription("Test agent description").
					WithInstructions("Test agent instructions").
					Build(ctx)
				
				test.NewAgentBuilder(t, db, model).
					WithID(newAgentID).
					WithName("new-agent").
					WithDescription("New agent description").
					WithInstructions("New agent instructions").
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent1).
					WithID(taskID).
					Build(ctx)
			},
			Request: &v1.UpdateTaskRequest{
				Id:      taskID.String(),
				AgentId: strPtr(newAgentID.String()),
			},
			Expected: ServiceTestExpectation[v1.UpdateTaskResponse]{
				Response: v1.UpdateTaskResponse{
					Task: &v1.Task{
						Id:       taskID.String(),
						Metadata: &v1.TaskMetadata{},
						Spec: &v1.TaskSpec{
							AgentId: strPtr(newAgentID.String()),
						},
						Status: &v1.TaskStatus{
							Usage: &v1.TaskUsage{},
						},
					},
				},
			},
		},
	})
}

func TestDeleteTask(t *testing.T) {
	setup := ServiceTestSetup[v1.DeleteTaskRequest, v1.DeleteTaskResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.DeleteTaskRequest]) (*connect.Response[v1.DeleteTaskResponse], error) {
			return client.Task().DeleteTask(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.DeleteTaskResponse{}),
			protocmp.Transform(),
		},
	}

	taskID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	agentID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")
	modelID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.DeleteTaskRequest, v1.DeleteTaskResponse]{
		{
			Name: "invalid id format",
			Request: &v1.DeleteTaskRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.DeleteTaskResponse]{
				Error: "invalid_argument: invalid task ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "task not found",
			Request: &v1.DeleteTaskRequest{
				Id: taskID.String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteTaskResponse]{
				Error: "not_found: task not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).
					WithID(modelID).
					Build(ctx)
				
				agent := test.NewAgentBuilder(t, db, model).
					WithID(agentID).
					WithName("test-agent").
					WithDescription("Test agent description").
					WithInstructions("Test agent instructions").
					Build(ctx)
				
				test.NewTaskBuilder(t, db, agent).
					WithID(taskID).
					Build(ctx)
			},
			Request: &v1.DeleteTaskRequest{
				Id: taskID.String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteTaskResponse]{
				Response: v1.DeleteTaskResponse{},
			},
		},
	})
}
