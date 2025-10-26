package api

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"google.golang.org/protobuf/testing/protocmp"
	_ "modernc.org/sqlite"
)

func TestCreateAgent(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateAgentRequest, v1.CreateAgentResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateAgentRequest]) (*connect.Response[v1.CreateAgentResponse], error) {
			return client.Agent().CreateAgent(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateAgentResponse{}, v1.Agent{}, v1.AgentMetadata{}, v1.AgentSpec{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.AgentMetadata{}, "id", "created_at", "updated_at"),
			cmpopts.IgnoreMapEntries(func(key string, value interface{}) bool {
				return key == "agent_id"
			}),
		},
	}

	modelID := uuid.New()

	setup.RunServiceTests(t, []ServiceTestScenario[v1.CreateAgentRequest, v1.CreateAgentResponse]{
		{
			Name: "invalid model ID",
			Request: &v1.CreateAgentRequest{
				Name:         "architect-agent",
				Description:  "Architect agent",
				Instructions: "Instructions for architect agent",
				ModelId:      "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.CreateAgentResponse]{
				Error: "invalid_argument: invalid model ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model not found",
			Request: &v1.CreateAgentRequest{
				Name:         "architect-agent",
				Description:  "Architect agent",
				Instructions: "Instructions for architect agent",
				ModelId:      modelID.String(),
			},
			Expected: ServiceTestExpectation[v1.CreateAgentResponse]{
				Error: "not_found: model not found",
			},
		},
		{
			Name: "model is disabled",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				test.NewModelBuilder(t, modelID, db, modelProvider).
					WithEnabled(false).
					Build(ctx)
			},
			Request: &v1.CreateAgentRequest{
				Name:         "architect-agent",
				Description:  "Architect agent",
				Instructions: "Instructions for architect agent",
				ModelId:      modelID.String(),
			},
			Expected: ServiceTestExpectation[v1.CreateAgentResponse]{
				Error: "invalid_argument: default model is disabled",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				test.NewModelBuilder(t, modelID, db, modelProvider).
					Build(ctx)
			},
			Request: &v1.CreateAgentRequest{
				Name:         "architect-agent",
				Description:  "Architect agent",
				Instructions: "Instructions for architect agent",
				ModelId:      modelID.String(),
			},
			Expected: ServiceTestExpectation[v1.CreateAgentResponse]{
				Response: v1.CreateAgentResponse{
					Agent: &v1.Agent{
						Metadata: &v1.AgentMetadata{},
						Spec: &v1.AgentSpec{
							Name:         "architect-agent",
							Description:  "Architect agent",
							Instructions: "Instructions for architect agent",
							ModelId:      modelID.String(),
						},
					},
				},
				Analytics: []analytics.Event{
					{
						DistinctId: "user",
						Event:      "agent_created",
						Properties: map[string]interface{}{
							"agent_id":   "ignored",
							"agent_name": "architect-agent",
							"model_name": "claude-3-7-sonnet-20250219",
						},
					},
				},
			},
		},
	})
}

func TestGetAgent(t *testing.T) {
	setup := ServiceTestSetup[v1.GetAgentRequest, v1.GetAgentResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.GetAgentRequest]) (*connect.Response[v1.GetAgentResponse], error) {
			return client.Agent().GetAgent(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.GetAgentResponse{}, v1.Agent{}, v1.AgentMetadata{}, v1.AgentSpec{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.AgentMetadata{}, "created_at", "updated_at"),
		},
	}

	agentID := uuid.New()
	modelID := uuid.New()

	setup.RunServiceTests(t, []ServiceTestScenario[v1.GetAgentRequest, v1.GetAgentResponse]{
		{
			Name: "invalid id format",
			Request: &v1.GetAgentRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.GetAgentResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "agent not found",
			Request: &v1.GetAgentRequest{
				Id: agentID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetAgentResponse]{
				Error: "not_found: agent not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).
					Build(ctx)

				test.NewAgentBuilder(t, agentID, db, model).
					WithName("architect-agent").
					WithDescription("Architect agent description").
					WithInstructions("Architect agent instructions").
					Build(ctx)
			},
			Request: &v1.GetAgentRequest{
				Id: agentID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetAgentResponse]{
				Response: v1.GetAgentResponse{
					Agent: &v1.Agent{
						Metadata: &v1.AgentMetadata{
							Id: agentID.String(),
						},
						Spec: &v1.AgentSpec{
							Name:         "architect-agent",
							Description:  "Architect agent description",
							Instructions: "Architect agent instructions",
							ModelId:      modelID.String(),
						},
					},
				},
			},
		},
	})
}

func TestListAgents(t *testing.T) {
	setup := ServiceTestSetup[v1.ListAgentsRequest, v1.ListAgentsResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.ListAgentsRequest]) (*connect.Response[v1.ListAgentsResponse], error) {
			return client.Agent().ListAgents(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.ListAgentsResponse{}, v1.Agent{}, v1.AgentMetadata{}, v1.AgentSpec{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.AgentMetadata{}, "created_at", "updated_at"),
		},
	}

	agent1ID := uuid.New()
	agent2ID := uuid.New()
	model1ID := uuid.New()
	model2ID := uuid.New()

	setup.RunServiceTests(t, []ServiceTestScenario[v1.ListAgentsRequest, v1.ListAgentsResponse]{
		{
			Name:    "empty list",
			Request: &v1.ListAgentsRequest{},
			Expected: ServiceTestExpectation[v1.ListAgentsResponse]{
				Response: v1.ListAgentsResponse{
					Agents: []*v1.Agent{},
				},
			},
		},
		{
			Name: "filter by name",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)

				model1 := test.NewModelBuilder(t, model1ID, db, modelProvider).
					WithName("claude-3-7-sonnet-model-1").
					Build(ctx)

				model2 := test.NewModelBuilder(t, model2ID, db, modelProvider).
					WithName("claude-3-7-sonnet-model-2").
					Build(ctx)
				test.NewAgentBuilder(t, agent1ID, db, model1).
					WithName("architect-agent-1").
					WithDescription("Architect agent 1 description").
					WithInstructions("Architect agent 1 instructions").
					Build(ctx)

				test.NewAgentBuilder(t, agent2ID, db, model2).
					WithName("architect-agent-2").
					WithDescription("Architect agent 2 description").
					WithInstructions("Architect agent 2 instructions").
					Build(ctx)
			},
			Request: &v1.ListAgentsRequest{
				Filter: &v1.ListAgentsRequest_Filter{
					Names: []string{"architect-agent-1"},
				},
			},
			Expected: ServiceTestExpectation[v1.ListAgentsResponse]{
				Response: v1.ListAgentsResponse{
					Agents: []*v1.Agent{
						{
							Metadata: &v1.AgentMetadata{
								Id: agent1ID.String(),
							},
							Spec: &v1.AgentSpec{
								Name:         "architect-agent-1",
								Description:  "Architect agent 1 description",
								Instructions: "Architect agent 1 instructions",
								ModelId:      model1ID.String(),
							},
						},
					},
				},
			},
		},
		{
			Name: "multiple agents",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)

				model1 := test.NewModelBuilder(t, model1ID, db, modelProvider).
					Build(ctx)

				test.NewAgentBuilder(t, agent1ID, db, model1).
					WithName("architect-agent-1").
					WithDescription("Architect agent 1 description").
					WithInstructions("Architect agent 1 instructions").
					Build(ctx)

				test.NewAgentBuilder(t, agent2ID, db, model1).
					WithName("architect-agent-2").
					WithDescription("Architect agent 2 description").
					WithInstructions("Architect agent 2 instructions").
					Build(ctx)
			},
			Request: &v1.ListAgentsRequest{},
			Expected: ServiceTestExpectation[v1.ListAgentsResponse]{
				Response: v1.ListAgentsResponse{
					Agents: []*v1.Agent{
						{
							Metadata: &v1.AgentMetadata{
								Id: agent1ID.String(),
							},
							Spec: &v1.AgentSpec{
								Name:         "architect-agent-1",
								Description:  "Architect agent 1 description",
								Instructions: "Architect agent 1 instructions",
								ModelId:      model1ID.String(),
							},
						},
						{
							Metadata: &v1.AgentMetadata{
								Id: agent2ID.String(),
							},
							Spec: &v1.AgentSpec{
								Name:         "architect-agent-2",
								Description:  "Architect agent 2 description",
								Instructions: "Architect agent 2 instructions",
								ModelId:      model1ID.String(),
							},
						},
					},
				},
			},
		},
	})
}

func TestUpdateAgent(t *testing.T) {
	setup := ServiceTestSetup[v1.UpdateAgentRequest, v1.UpdateAgentResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.UpdateAgentRequest]) (*connect.Response[v1.UpdateAgentResponse], error) {
			return client.Agent().UpdateAgent(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.UpdateAgentResponse{}, v1.Agent{}, v1.AgentMetadata{}, v1.AgentSpec{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.AgentMetadata{}, "created_at", "updated_at"),
		},
	}

	agentID := uuid.New()
	modelID := uuid.New()
	newModelID := uuid.New()

	setup.RunServiceTests(t, []ServiceTestScenario[v1.UpdateAgentRequest, v1.UpdateAgentResponse]{
		{
			Name: "invalid id format",
			Request: &v1.UpdateAgentRequest{
				Id:   "not-a-valid-uuid",
				Name: strPtr("updated-agent"),
			},
			Expected: ServiceTestExpectation[v1.UpdateAgentResponse]{
				Error: "invalid_argument: invalid agent ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "agent not found",
			Request: &v1.UpdateAgentRequest{
				Id:   agentID.String(),
				Name: strPtr("updated-agent"),
			},
			Expected: ServiceTestExpectation[v1.UpdateAgentResponse]{
				Error: "not_found: agent not found",
			},
		},
		{
			Name: "invalid model ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).
					Build(ctx)

				test.NewAgentBuilder(t, agentID, db, model).
					WithName("architect-agent").
					WithDescription("Architect agent description").
					WithInstructions("Architect agent instructions").
					Build(ctx)
			},
			Request: &v1.UpdateAgentRequest{
				Id:      agentID.String(),
				ModelId: strPtr("not-a-valid-uuid"),
			},
			Expected: ServiceTestExpectation[v1.UpdateAgentResponse]{
				Error: "invalid_argument: invalid model ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model not found",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).
					Build(ctx)

				test.NewAgentBuilder(t, agentID, db, model).
					WithName("architect-agent").
					WithDescription("Architect agent description").
					WithInstructions("Architect agent instructions").
					Build(ctx)
			},
			Request: &v1.UpdateAgentRequest{
				Id:      agentID.String(),
				ModelId: strPtr(newModelID.String()),
			},
			Expected: ServiceTestExpectation[v1.UpdateAgentResponse]{
				Error: "not_found: model not found",
			},
		},
		{
			Name: "success - update fields",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).
					Build(ctx)

				test.NewAgentBuilder(t, agentID, db, model).
					WithName("architect-agent").
					WithDescription("Architect agent description").
					WithInstructions("Architect agent instructions").
					Build(ctx)
			},
			Request: &v1.UpdateAgentRequest{
				Id:           agentID.String(),
				Name:         strPtr("updated-agent"),
				Description:  strPtr("Updated description"),
				Instructions: strPtr("Updated instructions"),
			},
			Expected: ServiceTestExpectation[v1.UpdateAgentResponse]{
				Response: v1.UpdateAgentResponse{
					Agent: &v1.Agent{
						Metadata: &v1.AgentMetadata{
							Id: agentID.String(),
						},
						Spec: &v1.AgentSpec{
							Name:         "updated-agent",
							Description:  "Updated description",
							Instructions: "Updated instructions",
							ModelId:      modelID.String(),
						},
					},
				},
			},
		},
		{
			Name: "success - update model",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)

				model1 := test.NewModelBuilder(t, modelID, db, modelProvider).
					WithName("claude-3-7-sonnet-original").
					Build(ctx)

				// Create the new model that will be used in the update
				test.NewModelBuilder(t, newModelID, db, modelProvider).
					WithName("claude-3-7-sonnet-new").
					Build(ctx)
				test.NewAgentBuilder(t, agentID, db, model1).
					WithName("architect-agent").
					WithDescription("Architect agent description").
					WithInstructions("Architect agent instructions").
					Build(ctx)
			},
			Request: &v1.UpdateAgentRequest{
				Id:      agentID.String(),
				ModelId: strPtr(newModelID.String()),
			},
			Expected: ServiceTestExpectation[v1.UpdateAgentResponse]{
				Response: v1.UpdateAgentResponse{
					Agent: &v1.Agent{
						Metadata: &v1.AgentMetadata{
							Id: agentID.String(),
						},
						Spec: &v1.AgentSpec{
							Name:         "architect-agent",
							Description:  "Architect agent description",
							Instructions: "Architect agent instructions",
							ModelId:      newModelID.String(),
						},
					},
				},
			},
		},
	})
}

func TestDeleteAgent(t *testing.T) {
	setup := ServiceTestSetup[v1.DeleteAgentRequest, v1.DeleteAgentResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.DeleteAgentRequest]) (*connect.Response[v1.DeleteAgentResponse], error) {
			return client.Agent().DeleteAgent(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.DeleteAgentResponse{}),
			protocmp.Transform(),
		},
	}

	agentID := uuid.New()
	modelID := uuid.New()

	setup.RunServiceTests(t, []ServiceTestScenario[v1.DeleteAgentRequest, v1.DeleteAgentResponse]{
		{
			Name: "invalid id format",
			Request: &v1.DeleteAgentRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.DeleteAgentResponse]{
				Error: "invalid_argument: invalid agent ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "agent not found",
			Request: &v1.DeleteAgentRequest{
				Id: agentID.String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteAgentResponse]{
				Error: "not_found: agent not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, modelID, db, modelProvider).
					Build(ctx)

				test.NewAgentBuilder(t, agentID, db, model).
					WithName("architect-agent").
					WithDescription("Architect agent description").
					WithInstructions("Architect agent instructions").
					Build(ctx)
			},
			Request: &v1.DeleteAgentRequest{
				Id: agentID.String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteAgentResponse]{
				Response: v1.DeleteAgentResponse{},
			},
		},
	})
}
