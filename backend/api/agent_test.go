package api

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCreateAgent(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateAgentRequest, v1.CreateAgentResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateAgentRequest]) (*connect.Response[v1.CreateAgentResponse], error) {
			return client.Agent().CreateAgent(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateAgentResponse{}, v1.Agent{}, v1.AgentMetadata{}, v1.AgentSpec{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.Agent{}, "id"),
			protocmp.IgnoreFields(&v1.AgentMetadata{}, "created_at", "updated_at"),
		},
	}

	modelID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")

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
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				_, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					SetEnabled(false).
					Save(ctx)
				return err
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
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				_, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					Save(ctx)
				return err
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
						Metadata: &v1.AgentMetadata{
							Name:        "architect-agent",
							Description: "Architect agent",
						},
						Spec: &v1.AgentSpec{
							Instructions: "Instructions for architect agent",
							ModelId:      modelID.String(),
							DelegateIds:  []string{},
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

	agentID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	modelID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")

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
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agentID).
					SetName("architect-agent").
					SetDescription("Architect agent description").
					SetInstructions("Architect agent instructions").
					SetModel(model).
					Save(ctx)
				return err
			},
			Request: &v1.GetAgentRequest{
				Id: agentID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetAgentResponse]{
				Response: v1.GetAgentResponse{
					Agent: &v1.Agent{
						Id: agentID.String(),
						Metadata: &v1.AgentMetadata{
							Name:        "architect-agent",
							Description: "Architect agent description",
						},
						Spec: &v1.AgentSpec{
							Instructions: "Architect agent instructions",
							ModelId:      modelID.String(),
							DelegateIds:  []string{},
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

	agent1ID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	agent2ID := uuid.MustParse("fedcba98-7654-3210-fedc-ba9876543210")
	model1ID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")
	model2ID := uuid.MustParse("abcdef01-2345-6789-abcd-ef0123456789")

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
			Name: "filter by model ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				model1, err := db.Model.Create().
					SetID(model1ID).
					SetName("model-1").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				model2, err := db.Model.Create().
					SetID(model2ID).
					SetName("model-2").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agent1ID).
					SetName("architect-agent-1").
					SetDescription("Architect agent 1 description").
					SetInstructions("Architect agent 1 instructions").
					SetModel(model1).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agent2ID).
					SetName("architect-agent-2").
					SetDescription("Architect agent 2 description").
					SetInstructions("Architect agent 2 instructions").
					SetModel(model2).
					Save(ctx)
				return err
			},
			Request: &v1.ListAgentsRequest{
				Filter: &v1.ListAgentsRequest_Filter{
					ModelId: strPtr(model1ID.String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListAgentsResponse]{
				Response: v1.ListAgentsResponse{
					Agents: []*v1.Agent{
						{
							Id: agent1ID.String(),
							Metadata: &v1.AgentMetadata{
								Name:        "architect-agent-1",
								Description: "Architect agent 1 description",
							},
							Spec: &v1.AgentSpec{
								Instructions: "Architect agent 1 instructions",
								ModelId:      model1ID.String(),
								DelegateIds:  []string{},
							},
						},
					},
				},
			},
		},
		{
			Name: "multiple agents",
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				model1, err := db.Model.Create().
					SetID(model1ID).
					SetName("model-1").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agent1ID).
					SetName("architect-agent-1").
					SetDescription("Architect agent 1 description").
					SetInstructions("Architect agent 1 instructions").
					SetModel(model1).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agent2ID).
					SetName("architect-agent-2").
					SetDescription("Architect agent 2 description").
					SetInstructions("Architect agent 2 instructions").
					SetModel(model1).
					Save(ctx)
				return err
			},
			Request: &v1.ListAgentsRequest{},
			Expected: ServiceTestExpectation[v1.ListAgentsResponse]{
				Response: v1.ListAgentsResponse{
					Agents: []*v1.Agent{
						{
							Id: agent1ID.String(),
							Metadata: &v1.AgentMetadata{
								Name:        "architect-agent-1",
								Description: "Architect agent 1 description",
							},
							Spec: &v1.AgentSpec{
								Instructions: "Architect agent 1 instructions",
								ModelId:      model1ID.String(),
								DelegateIds:  []string{},
							},
						},
						{
							Id: agent2ID.String(),
							Metadata: &v1.AgentMetadata{
								Name:        "architect-agent-2",
								Description: "Architect agent 2 description",
							},
							Spec: &v1.AgentSpec{
								Instructions: "Architect agent 2 instructions",
								ModelId:      model1ID.String(),
								DelegateIds:  []string{},
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

	agentID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	modelID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")
	newModelID := uuid.MustParse("abcdef01-2345-6789-abcd-ef0123456789")

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
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agentID).
					SetName("architect-agent").
					SetDescription("Architect agent description").
					SetInstructions("Architect agent instructions").
					SetModel(model).
					Save(ctx)
				return err
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
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agentID).
					SetName("architect-agent").
					SetDescription("Architect agent description").
					SetInstructions("Architect agent instructions").
					SetModel(model).
					Save(ctx)
				return err
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
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agentID).
					SetName("architect-agent").
					SetDescription("Architect agent description").
					SetInstructions("Architect agent instructions").
					SetModel(model).
					Save(ctx)
				return err
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
						Id: agentID.String(),
						Metadata: &v1.AgentMetadata{
							Name:        "updated-agent",
							Description: "Updated description",
						},
						Spec: &v1.AgentSpec{
							Instructions: "Updated instructions",
							ModelId:      modelID.String(),
							DelegateIds:  []string{},
						},
					},
				},
			},
		},
		{
			Name: "success - update model",
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Model.Create().
					SetID(newModelID).
					SetName("new-model").
					SetContextWindow(32000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agentID).
					SetName("architect-agent").
					SetDescription("Architect agent description").
					SetInstructions("Architect agent instructions").
					SetModel(model).
					Save(ctx)
				return err
			},
			Request: &v1.UpdateAgentRequest{
				Id:      agentID.String(),
				ModelId: strPtr(newModelID.String()),
			},
			Expected: ServiceTestExpectation[v1.UpdateAgentResponse]{
				Response: v1.UpdateAgentResponse{
					Agent: &v1.Agent{
						Id: agentID.String(),
						Metadata: &v1.AgentMetadata{
							Name:        "architect-agent",
							Description: "Architect agent description",
						},
						Spec: &v1.AgentSpec{
							Instructions: "Architect agent instructions",
							ModelId:      newModelID.String(),
							DelegateIds:  []string{},
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

	agentID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	modelID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")

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
			SeedDatabase: func(ctx context.Context, db *memory.Client) error {
				model, err := db.Model.Create().
					SetID(modelID).
					SetName("test-model").
					SetContextWindow(16000).
					Save(ctx)
				if err != nil {
					return err
				}

				_, err = db.Agent.Create().
					SetID(agentID).
					SetName("architect-agent").
					SetDescription("Architect agent description").
					SetInstructions("Architect agent instructions").
					SetModel(model).
					Save(ctx)
				return err
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

