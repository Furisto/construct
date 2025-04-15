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
	"github.com/googleapis/go-type-adapters/adapters"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCreateModel(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateModelRequest, v1.CreateModelResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateModelRequest]) (*connect.Response[v1.CreateModelResponse], error) {
			return client.Model().CreateModel(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateModelResponse{}, v1.Model{}, v1.ModelMetadata{}, v1.ModelPricing{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.Model{}, "id", "metadata"),
			protocmp.IgnoreFields(&v1.ModelMetadata{}, "created_at", "updated_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.CreateModelRequest, v1.CreateModelResponse]{
		{
			Name: "invalid model provider ID",
			Request: &v1.CreateModelRequest{
				Name:            "test-model",
				ModelProviderId: "not-a-valid-uuid",
				ContextWindow:   4096,
			},
			Expected: ServiceTestExpectation[v1.CreateModelResponse]{
				Error: "invalid_argument: invalid model provider ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model provider not found",
			Request: &v1.CreateModelRequest{
				Name:            "test-model",
				ModelProviderId: test.ModelProviderID().String(),
				ContextWindow:   4096,
			},
			Expected: ServiceTestExpectation[v1.CreateModelResponse]{
				Error: "not_found: model_provider not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewModelProviderBuilder(t, db).
					Build(ctx)
			},
			Request: &v1.CreateModelRequest{
				Name:            "test-model",
				ModelProviderId: test.ModelProviderID().String(),
				ContextWindow:   4096,
				Pricing: &v1.ModelPricing{
					InputCost:      adapters.Float64ToProtoDecimal(0.0001),
					OutputCost:     adapters.Float64ToProtoDecimal(0.0002),
					CacheWriteCost: adapters.Float64ToProtoDecimal(0.00005),
					CacheReadCost:  adapters.Float64ToProtoDecimal(0.00001),
				},
				Capabilities: []v1.ModelCapability{
					v1.ModelCapability_MODEL_CAPABILITY_IMAGE,
				},
			},
			Expected: ServiceTestExpectation[v1.CreateModelResponse]{
				Response: v1.CreateModelResponse{
					Model: &v1.Model{
						Name:            "test-model",
						ModelProviderId: test.ModelProviderID().String(),
						ContextWindow:   4096,
						Enabled:         true,
						Pricing: &v1.ModelPricing{
							InputCost:      adapters.Float64ToProtoDecimal(0.0001),
							OutputCost:     adapters.Float64ToProtoDecimal(0.0002),
							CacheWriteCost: adapters.Float64ToProtoDecimal(0.00005),
							CacheReadCost:  adapters.Float64ToProtoDecimal(0.00001),
						},
						Capabilities: []v1.ModelCapability{
							v1.ModelCapability_MODEL_CAPABILITY_IMAGE,
						},
					},
				},
			},
		},
	})
}

func TestGetModel(t *testing.T) {
	setup := ServiceTestSetup[v1.GetModelRequest, v1.GetModelResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.GetModelRequest]) (*connect.Response[v1.GetModelResponse], error) {
			return client.Model().GetModel(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.GetModelResponse{}, v1.Model{}, v1.ModelMetadata{}, v1.ModelPricing{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.Model{}, "id", "metadata"),
			protocmp.IgnoreFields(&v1.ModelMetadata{}, "created_at", "updated_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.GetModelRequest, v1.GetModelResponse]{
		{
			Name: "invalid id format",
			Request: &v1.GetModelRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.GetModelResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model not found",
			Request: &v1.GetModelRequest{
				Id: test.ModelID().String(),
			},
			Expected: ServiceTestExpectation[v1.GetModelResponse]{
				Error: "not_found: model not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					Build(ctx)
			},
			Request: &v1.GetModelRequest{
				Id: test.ModelID().String(),
			},
			Expected: ServiceTestExpectation[v1.GetModelResponse]{
				Response: v1.GetModelResponse{
					Model: &v1.Model{
						Id:              test.ModelID().String(),
						Name:            "claude-3-7-sonnet-20250219",
						ModelProviderId: test.ModelProviderID().String(),
						ContextWindow:   200_000,
						Pricing: &v1.ModelPricing{
							InputCost:      adapters.Float64ToProtoDecimal(3),
							OutputCost:     adapters.Float64ToProtoDecimal(15),
							CacheWriteCost: adapters.Float64ToProtoDecimal(3.75),
							CacheReadCost:  adapters.Float64ToProtoDecimal(0.3),
						},
						Capabilities: []v1.ModelCapability{
							v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE,
						},
						Enabled: true,
					},
				},
			},
		},
	})
}

func TestListModels(t *testing.T) {
	setup := ServiceTestSetup[v1.ListModelsRequest, v1.ListModelsResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.ListModelsRequest]) (*connect.Response[v1.ListModelsResponse], error) {
			return client.Model().ListModels(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.ListModelsResponse{}, v1.Model{}, v1.ModelMetadata{}, v1.ModelPricing{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.Model{}, "id", "metadata"),
			protocmp.IgnoreFields(&v1.ModelMetadata{}, "created_at", "updated_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.ListModelsRequest, v1.ListModelsResponse]{
		{
			Name:    "empty list",
			Request: &v1.ListModelsRequest{},
			Expected: ServiceTestExpectation[v1.ListModelsResponse]{
				Response: v1.ListModelsResponse{
					Models: []*v1.Model{},
				},
			},
		},
		{
			Name: "invalid model provider ID in filter",
			Request: &v1.ListModelsRequest{
				Filter: &v1.ListModelsRequest_Filter{
					ModelProviderId: strPtr("not-a-valid-uuid"),
				},
			},
			Expected: ServiceTestExpectation[v1.ListModelsResponse]{
				Error: "invalid_argument: invalid model provider ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "filter by model provider ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider1 := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				modelProvider2 := test.NewModelProviderBuilder(t, db).
					WithID(test.ModelProviderID2()).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider1).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider2).
					WithID(test.ModelID2()).
					WithName("o1-preview").
					Build(ctx)
			},
			Request: &v1.ListModelsRequest{
				Filter: &v1.ListModelsRequest_Filter{
					ModelProviderId: strPtr(test.ModelProviderID().String()),
				},
			},
			Expected: ServiceTestExpectation[v1.ListModelsResponse]{
				Response: v1.ListModelsResponse{
					Models: []*v1.Model{
						{
							Id:              test.ModelID().String(),
							Name:            "claude-3-7-sonnet-20250219",
							ModelProviderId: test.ModelProviderID().String(),
							ContextWindow:   200_000,
							Enabled:         true,
							Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE},
							Pricing: &v1.ModelPricing{
								InputCost:      adapters.Float64ToProtoDecimal(3),
								OutputCost:     adapters.Float64ToProtoDecimal(15),
								CacheWriteCost: adapters.Float64ToProtoDecimal(3.75),
								CacheReadCost:  adapters.Float64ToProtoDecimal(0.3),
							},
						},
					},
				},
			},
		},
		{
			Name: "filter by enabled status",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					WithID(test.ModelID2()).
					WithName("o1-preview").
					WithEnabled(false).
					Build(ctx)
			},
			Request: &v1.ListModelsRequest{
				Filter: &v1.ListModelsRequest_Filter{
					Enabled: boolPtr(true),
				},
			},
			Expected: ServiceTestExpectation[v1.ListModelsResponse]{
				Response: v1.ListModelsResponse{
					Models: []*v1.Model{
						{
							Id:              test.ModelID().String(),
							Name:            "claude-3-7-sonnet-20250219",
							ModelProviderId: test.ModelProviderID().String(),
							ContextWindow:   200_000,
							Enabled:         true,
							Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE},
							Pricing: &v1.ModelPricing{
								InputCost:      adapters.Float64ToProtoDecimal(3),
								OutputCost:     adapters.Float64ToProtoDecimal(15),
								CacheWriteCost: adapters.Float64ToProtoDecimal(3.75),
								CacheReadCost:  adapters.Float64ToProtoDecimal(0.3),
							},
						},
					},
				},
			},
		},
		{
			Name: "multiple models",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					WithID(test.ModelID2()).
					WithName("o1-preview").
					Build(ctx)
			},
			Request: &v1.ListModelsRequest{},
			Expected: ServiceTestExpectation[v1.ListModelsResponse]{
				Response: v1.ListModelsResponse{
					Models: []*v1.Model{
						{
							Id:              test.ModelID().String(),
							Name:            "claude-3-7-sonnet-20250219",
							ModelProviderId: test.ModelProviderID().String(),
							ContextWindow:   200_000,
							Enabled:         true,
							Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE},
							Pricing: &v1.ModelPricing{
								InputCost:      adapters.Float64ToProtoDecimal(3),
								OutputCost:     adapters.Float64ToProtoDecimal(15),
								CacheWriteCost: adapters.Float64ToProtoDecimal(3.75),
								CacheReadCost:  adapters.Float64ToProtoDecimal(0.3),
							},
						},
						{
							Id:              test.ModelID2().String(),
							Name:            "o1-preview",
							ModelProviderId: test.ModelProviderID().String(),
							ContextWindow:   200_000,
							Enabled:         true,
							Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE},
							Pricing: &v1.ModelPricing{
								InputCost:      adapters.Float64ToProtoDecimal(3),
								OutputCost:     adapters.Float64ToProtoDecimal(15),
								CacheWriteCost: adapters.Float64ToProtoDecimal(3.75),
								CacheReadCost:  adapters.Float64ToProtoDecimal(0.3),
							},
						},
					},
				},
			},
		},
	})
}

func TestUpdateModel(t *testing.T) {
	setup := ServiceTestSetup[v1.UpdateModelRequest, v1.UpdateModelResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.UpdateModelRequest]) (*connect.Response[v1.UpdateModelResponse], error) {
			return client.Model().UpdateModel(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.UpdateModelResponse{}, v1.Model{}, v1.ModelMetadata{}, v1.ModelPricing{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.Model{}, "id", "metadata"),
			protocmp.IgnoreFields(&v1.ModelMetadata{}, "created_at", "updated_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.UpdateModelRequest, v1.UpdateModelResponse]{
		{
			Name: "invalid id format",
			Request: &v1.UpdateModelRequest{
				Id:   "not-a-valid-uuid",
				Name: strPtr("updated-model"),
			},
			Expected: ServiceTestExpectation[v1.UpdateModelResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model not found",
			Request: &v1.UpdateModelRequest{
				Id:   test.ModelID2().String(),
				Name: strPtr("updated-model"),
			},
			Expected: ServiceTestExpectation[v1.UpdateModelResponse]{
				Error: "not_found: model not found",
			},
		},
		{
			Name: "invalid model provider ID",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					Build(ctx)
			},
			Request: &v1.UpdateModelRequest{
				Id:              test.ModelID().String(),
				ModelProviderId: strPtr("not-a-valid-uuid"),
			},
			Expected: ServiceTestExpectation[v1.UpdateModelResponse]{
				Error: "invalid_argument: invalid model provider ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model provider not found",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					Build(ctx)
			},
			Request: &v1.UpdateModelRequest{
				Id:              test.ModelID().String(),
				ModelProviderId: strPtr(test.ModelProviderID2().String()),
			},
			Expected: ServiceTestExpectation[v1.UpdateModelResponse]{
				Error: "not_found: model_provider not found",
			},
		},
		{
			Name: "success - update fields",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					Build(ctx)
			},
			Request: &v1.UpdateModelRequest{
				Id:            test.ModelID().String(),
				Name:          strPtr("updated-model"),
				ContextWindow: ptr(int64(500_000)),
				Enabled:       boolPtr(false),
				Capabilities: []v1.ModelCapability{
					v1.ModelCapability_MODEL_CAPABILITY_THINKING,
				},
			},
			Expected: ServiceTestExpectation[v1.UpdateModelResponse]{
				Response: v1.UpdateModelResponse{
					Model: &v1.Model{
						Id:              test.ModelID().String(),
						Name:            "updated-model",
						ModelProviderId: test.ModelProviderID().String(),
						ContextWindow:   500_000,
						Enabled:         false,
						Capabilities: []v1.ModelCapability{
							v1.ModelCapability_MODEL_CAPABILITY_THINKING,
						},
						Pricing: &v1.ModelPricing{
							InputCost:      adapters.Float64ToProtoDecimal(3),
							OutputCost:     adapters.Float64ToProtoDecimal(15),
							CacheWriteCost: adapters.Float64ToProtoDecimal(3.75),
							CacheReadCost:  adapters.Float64ToProtoDecimal(0.3),
						},
					},
				},
			},
		},
		{
			Name: "success - update model provider",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider1 := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelProviderBuilder(t, db).
					WithID(test.ModelProviderID2()).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider1).
					Build(ctx)
			},
			Request: &v1.UpdateModelRequest{
				Id:              test.ModelID().String(),
				ModelProviderId: ptr(test.ModelProviderID2().String()),
			},
			Expected: ServiceTestExpectation[v1.UpdateModelResponse]{
				Response: v1.UpdateModelResponse{
					Model: &v1.Model{
						Id:              test.ModelID().String(),
						Name:            "claude-3-7-sonnet-20250219",
						ModelProviderId: test.ModelProviderID2().String(),
						ContextWindow:   200_000,
						Enabled:         true,
						Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE},
						Pricing: &v1.ModelPricing{
							InputCost:      adapters.Float64ToProtoDecimal(3),
							OutputCost:     adapters.Float64ToProtoDecimal(15),
							CacheWriteCost: adapters.Float64ToProtoDecimal(3.75),
							CacheReadCost:  adapters.Float64ToProtoDecimal(0.3),
						},
					},
				},
			},
		},
		{
			Name: "success - update pricing",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					Build(ctx)
			},
			Request: &v1.UpdateModelRequest{
				Id: test.ModelID().String(),
				Pricing: &v1.ModelPricing{
					InputCost:      adapters.Float64ToProtoDecimal(0.0001),
					OutputCost:     adapters.Float64ToProtoDecimal(0.0002),
					CacheWriteCost: adapters.Float64ToProtoDecimal(0.00005),
					CacheReadCost:  adapters.Float64ToProtoDecimal(0.00001),
				},
			},
			Expected: ServiceTestExpectation[v1.UpdateModelResponse]{
				Response: v1.UpdateModelResponse{
					Model: &v1.Model{
						Id:              test.ModelID().String(),
						Name:            "claude-3-7-sonnet-20250219",
						ModelProviderId: test.ModelProviderID().String(),
						ContextWindow:   200_000,
						Enabled:         true,
						Capabilities:    []v1.ModelCapability{v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE},
						Pricing: &v1.ModelPricing{
							InputCost:      adapters.Float64ToProtoDecimal(0.0001),
							OutputCost:     adapters.Float64ToProtoDecimal(0.0002),
							CacheWriteCost: adapters.Float64ToProtoDecimal(0.00005),
							CacheReadCost:  adapters.Float64ToProtoDecimal(0.00001),
						},
					},
				},
			},
		},
	})
}

func TestDeleteModel(t *testing.T) {
	setup := ServiceTestSetup[v1.DeleteModelRequest, v1.DeleteModelResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.DeleteModelRequest]) (*connect.Response[v1.DeleteModelResponse], error) {
			return client.Model().DeleteModel(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.DeleteModelResponse{}),
			protocmp.Transform(),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.DeleteModelRequest, v1.DeleteModelResponse]{
		{
			Name: "invalid id format",
			Request: &v1.DeleteModelRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.DeleteModelResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model not found",
			Request: &v1.DeleteModelRequest{
				Id: test.ModelID2().String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteModelResponse]{
				Error: "not_found: model not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).
					Build(ctx)

				test.NewModelBuilder(t, db, modelProvider).
					Build(ctx)
			},
			Request: &v1.DeleteModelRequest{
				Id: test.ModelID().String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteModelResponse]{
				Response: v1.DeleteModelResponse{},
			},
		},
	})
}
