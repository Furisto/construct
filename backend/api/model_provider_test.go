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

func TestCreateModelProvider(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateModelProviderRequest, v1.CreateModelProviderResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateModelProviderRequest]) (*connect.Response[v1.CreateModelProviderResponse], error) {
			return client.ModelProvider().CreateModelProvider(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateModelProviderResponse{}, v1.ModelProvider{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.ModelProvider{}, "id", "created_at", "updated_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.CreateModelProviderRequest, v1.CreateModelProviderResponse]{
		{
			Name: "invalid provider type",
			Request: &v1.CreateModelProviderRequest{
				Name:         "anthropic",
				ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_UNSPECIFIED,
			},
			Expected: ServiceTestExpectation[v1.CreateModelProviderResponse]{
				Error: "invalid_argument: unsupported provider type: MODEL_PROVIDER_TYPE_UNSPECIFIED",
			},
		},
		{
			Name: "success",
			Request: &v1.CreateModelProviderRequest{
				Name:         "anthropic",
				ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
				Authentication: &v1.CreateModelProviderRequest_ApiKey{
					ApiKey: "sk-ant-api03-1234567890",
				},
			},
			Expected: ServiceTestExpectation[v1.CreateModelProviderResponse]{
				Response: v1.CreateModelProviderResponse{
					ModelProvider: &v1.ModelProvider{
						Name:         "anthropic",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      true,
					},
				},
			},
		},
	})
}

func TestGetModelProvider(t *testing.T) {
	setup := ServiceTestSetup[v1.GetModelProviderRequest, v1.GetModelProviderResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.GetModelProviderRequest]) (*connect.Response[v1.GetModelProviderResponse], error) {
			return client.ModelProvider().GetModelProvider(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.GetModelProviderResponse{}, v1.ModelProvider{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.ModelProvider{}, "created_at", "updated_at"),
		},
	}

	modelProviderID := uuid.New()

	setup.RunServiceTests(t, []ServiceTestScenario[v1.GetModelProviderRequest, v1.GetModelProviderResponse]{
		{
			Name: "invalid id format",
			Request: &v1.GetModelProviderRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.GetModelProviderResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model provider not found",
			Request: &v1.GetModelProviderRequest{
				Id: "01234567-89ab-cdef-0123-456789abcdef",
			},
			Expected: ServiceTestExpectation[v1.GetModelProviderResponse]{
				Error: "not_found: model_provider not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewModelProviderBuilder(t, modelProviderID, db).
					Build(ctx)
			},
			Request: &v1.GetModelProviderRequest{
				Id: modelProviderID.String(),
			},
			Expected: ServiceTestExpectation[v1.GetModelProviderResponse]{
				Response: v1.GetModelProviderResponse{
					ModelProvider: &v1.ModelProvider{
						Id:           modelProviderID.String(),
						Name:         "anthropic",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      true,
					},
				},
			},
		},
	})
}

func TestListModelProviders(t *testing.T) {
	setup := ServiceTestSetup[v1.ListModelProvidersRequest, v1.ListModelProvidersResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.ListModelProvidersRequest]) (*connect.Response[v1.ListModelProvidersResponse], error) {
			return client.ModelProvider().ListModelProviders(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.ListModelProvidersResponse{}, v1.ModelProvider{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.ModelProvider{}, "created_at", "updated_at"),
		},
	}

	anthropicID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
	openaiID := uuid.MustParse("98765432-10fe-dcba-9876-543210fedcba")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.ListModelProvidersRequest, v1.ListModelProvidersResponse]{
		{
			Name:    "empty list",
			Request: &v1.ListModelProvidersRequest{},
			Expected: ServiceTestExpectation[v1.ListModelProvidersResponse]{
				Response: v1.ListModelProvidersResponse{
					ModelProviders: []*v1.ModelProvider{},
				},
			},
		},
		{
			Name: "filter by enabled",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				// Create enabled provider
				test.NewModelProviderBuilder(t, anthropicID, db).
					WithName("anthropic").
					WithProviderType(types.ModelProviderTypeAnthropic).
					WithEnabled(true).
					Build(ctx)

				// Create disabled provider
				test.NewModelProviderBuilder(t, openaiID, db).
					WithName("openai").
					WithProviderType(types.ModelProviderTypeOpenAI).
					WithEnabled(false).
					Build(ctx)
			},
			Request: &v1.ListModelProvidersRequest{
				Filter: &v1.ListModelProvidersRequest_Filter{
					Enabled: &[]bool{true}[0],
				},
			},
			Expected: ServiceTestExpectation[v1.ListModelProvidersResponse]{
				Response: v1.ListModelProvidersResponse{
					ModelProviders: []*v1.ModelProvider{
						{
							Id:           anthropicID.String(),
							Name:         "anthropic",
							ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
							Enabled:      true,
						},
					},
				},
			},
		},
		{
			Name: "filter by provider type",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				// Create Anthropic provider
				test.NewModelProviderBuilder(t, anthropicID, db).
					WithName("anthropic").
					WithProviderType(types.ModelProviderTypeAnthropic).
					WithEnabled(true).
					Build(ctx)

				// Create OpenAI provider
				test.NewModelProviderBuilder(t, openaiID, db).
					WithName("openai").
					WithProviderType(types.ModelProviderTypeOpenAI).
					WithEnabled(true).
					Build(ctx)
			},
			Request: &v1.ListModelProvidersRequest{
				Filter: &v1.ListModelProvidersRequest_Filter{
					ProviderTypes: []v1.ModelProviderType{v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI},
				},
			},
			Expected: ServiceTestExpectation[v1.ListModelProvidersResponse]{
				Response: v1.ListModelProvidersResponse{
					ModelProviders: []*v1.ModelProvider{
						{
							Id:           openaiID.String(),
							Name:         "openai",
							ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
							Enabled:      true,
						},
					},
				},
			},
		},
		{
			Name: "multiple providers",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				// Create Anthropic provider
				test.NewModelProviderBuilder(t, anthropicID, db).
					WithName("anthropic").
					WithProviderType(types.ModelProviderTypeAnthropic).
					WithEnabled(true).
					Build(ctx)

				// Create OpenAI provider
				test.NewModelProviderBuilder(t, openaiID, db).
					WithName("openai").
					WithProviderType(types.ModelProviderTypeOpenAI).
					WithEnabled(true).
					Build(ctx)
			},
			Request: &v1.ListModelProvidersRequest{},
			Expected: ServiceTestExpectation[v1.ListModelProvidersResponse]{
				Response: v1.ListModelProvidersResponse{
					ModelProviders: []*v1.ModelProvider{
						{
							Id:           anthropicID.String(),
							Name:         "anthropic",
							ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
							Enabled:      true,
						},
						{
							Id:           openaiID.String(),
							Name:         "openai",
							ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
							Enabled:      true,
						},
					},
				},
			},
		},
	})
}

func TestDeleteModelProvider(t *testing.T) {
	setup := ServiceTestSetup[v1.DeleteModelProviderRequest, v1.DeleteModelProviderResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.DeleteModelProviderRequest]) (*connect.Response[v1.DeleteModelProviderResponse], error) {
			return client.ModelProvider().DeleteModelProvider(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.DeleteModelProviderResponse{}),
			protocmp.Transform(),
		},
	}

	testProviderID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")

	setup.RunServiceTests(t, []ServiceTestScenario[v1.DeleteModelProviderRequest, v1.DeleteModelProviderResponse]{
		{
			Name: "invalid id format",
			Request: &v1.DeleteModelProviderRequest{
				Id: "not-a-valid-uuid",
			},
			Expected: ServiceTestExpectation[v1.DeleteModelProviderResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 16",
			},
		},
		{
			Name: "model provider not found",
			Request: &v1.DeleteModelProviderRequest{
				Id: "01234567-89ab-cdef-0123-456789abcdef",
			},
			Expected: ServiceTestExpectation[v1.DeleteModelProviderResponse]{
				Error: "not_found: model_provider not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewModelProviderBuilder(t, testProviderID, db).
					WithName("anthropic").
					WithProviderType(types.ModelProviderTypeAnthropic).
					WithEnabled(true).
					Build(ctx)
			},
			Request: &v1.DeleteModelProviderRequest{
				Id: testProviderID.String(),
			},
			Expected: ServiceTestExpectation[v1.DeleteModelProviderResponse]{
				Response: v1.DeleteModelProviderResponse{},
			},
		},
	})
}
