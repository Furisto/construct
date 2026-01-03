package api

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCreateToken(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateTokenRequest, v1.CreateTokenResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateTokenRequest]) (*connect.Response[v1.CreateTokenResponse], error) {
			return client.Auth().CreateToken(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateTokenResponse{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.CreateTokenResponse{}, "token", "expires_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.CreateTokenRequest, v1.CreateTokenResponse]{
		{
			Name: "empty name",
			Request: &v1.CreateTokenRequest{
				Name: "",
			},
			Expected: ServiceTestExpectation[v1.CreateTokenResponse]{
				Error: "invalid_argument: name is required",
			},
		},
		{
			Name: "success with defaults",
			Request: &v1.CreateTokenRequest{
				Name: "test-token",
			},
			Expected: ServiceTestExpectation[v1.CreateTokenResponse]{
				Response: v1.CreateTokenResponse{},
			},
		},
		{
			Name: "success with description",
			Request: &v1.CreateTokenRequest{
				Name:        "test-token-with-desc",
				Description: strPtr("Test token description"),
			},
			Expected: ServiceTestExpectation[v1.CreateTokenResponse]{
				Response: v1.CreateTokenResponse{},
			},
		},
		{
			Name: "duplicate name",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewTokenBuilder(t, uuid.New(), db).WithName("duplicate-token").Build(ctx)
			},
			Request: &v1.CreateTokenRequest{
				Name: "duplicate-token",
			},
			Expected: ServiceTestExpectation[v1.CreateTokenResponse]{
				Error: "already_exists: token with name \"duplicate-token\" already exists",
			},
		},
	})
}

func TestCreateSetupCode(t *testing.T) {
	setup := ServiceTestSetup[v1.CreateSetupCodeRequest, v1.CreateSetupCodeResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.CreateSetupCodeRequest]) (*connect.Response[v1.CreateSetupCodeResponse], error) {
			return client.Auth().CreateSetupCode(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.CreateSetupCodeResponse{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.CreateSetupCodeResponse{}, "setup_code", "expires_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.CreateSetupCodeRequest, v1.CreateSetupCodeResponse]{
		{
			Name: "empty token name",
			Request: &v1.CreateSetupCodeRequest{
				TokenName: "",
			},
			Expected: ServiceTestExpectation[v1.CreateSetupCodeResponse]{
				Error: "invalid_argument: token_name is required",
			},
		},
		{
			Name: "success with defaults",
			Request: &v1.CreateSetupCodeRequest{
				TokenName: "test-token",
			},
			Expected: ServiceTestExpectation[v1.CreateSetupCodeResponse]{
				Response: v1.CreateSetupCodeResponse{},
			},
		},
	})
}

func TestListTokens(t *testing.T) {
	setup := ServiceTestSetup[v1.ListTokensRequest, v1.ListTokensResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.ListTokensRequest]) (*connect.Response[v1.ListTokensResponse], error) {
			return client.Auth().ListTokens(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.ListTokensResponse{}, v1.TokenInfo{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.TokenInfo{}, "id", "created_at", "expires_at"),
			cmpopts.SortSlices(func(a, b *v1.TokenInfo) bool {
				return a.Name < b.Name
			}),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.ListTokensRequest, v1.ListTokensResponse]{
		{
			Name:    "empty list",
			Request: &v1.ListTokensRequest{},
			Expected: ServiceTestExpectation[v1.ListTokensResponse]{
				Response: v1.ListTokensResponse{
					Tokens: []*v1.TokenInfo{},
				},
			},
		},
		{
			Name: "list all tokens",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewTokenBuilder(t, uuid.New(), db).WithName("token-1").Build(ctx)
				test.NewTokenBuilder(t, uuid.New(), db).WithName("token-2").Build(ctx)
				test.NewTokenBuilder(t, uuid.New(), db).WithName("token-3").Build(ctx)
			},
			Request: &v1.ListTokensRequest{},
			Expected: ServiceTestExpectation[v1.ListTokensResponse]{
				Response: v1.ListTokensResponse{
					Tokens: []*v1.TokenInfo{
						{Name: "token-3", IsActive: true},
						{Name: "token-2", IsActive: true},
						{Name: "token-1", IsActive: true},
					},
				},
			},
		},
		{
			Name: "filter by name prefix",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewTokenBuilder(t, uuid.New(), db).WithName("prod-token").Build(ctx)
				test.NewTokenBuilder(t, uuid.New(), db).WithName("dev-token").Build(ctx)
				test.NewTokenBuilder(t, uuid.New(), db).WithName("staging-token").Build(ctx)
			},
			Request: &v1.ListTokensRequest{
				NamePrefix: "prod",
			},
			Expected: ServiceTestExpectation[v1.ListTokensResponse]{
				Response: v1.ListTokensResponse{
					Tokens: []*v1.TokenInfo{
						{Name: "prod-token", IsActive: true},
					},
				},
			},
		},
		{
			Name: "exclude expired tokens by default",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewTokenBuilder(t, uuid.New(), db).WithName("active-token").Build(ctx)
				test.NewTokenBuilder(t, uuid.New(), db).WithName("expired-token").WithExpiresAt(time.Now().Add(-1 * time.Hour)).Build(ctx)
			},
			Request: &v1.ListTokensRequest{},
			Expected: ServiceTestExpectation[v1.ListTokensResponse]{
				Response: v1.ListTokensResponse{
					Tokens: []*v1.TokenInfo{
						{Name: "active-token", IsActive: true},
					},
				},
			},
		},
		{
			Name: "include expired tokens when requested",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewTokenBuilder(t, uuid.New(), db).WithName("active-token").Build(ctx)
				test.NewTokenBuilder(t, uuid.New(), db).WithName("expired-token").WithExpiresAt(time.Now().Add(-1 * time.Hour)).Build(ctx)
			},
			Request: &v1.ListTokensRequest{
				IncludeExpired: true,
			},
			Expected: ServiceTestExpectation[v1.ListTokensResponse]{
				Response: v1.ListTokensResponse{
					Tokens: []*v1.TokenInfo{
						{Name: "expired-token", IsActive: false},
						{Name: "active-token", IsActive: true},
					},
				},
			},
		},
	})
}

func TestRevokeToken(t *testing.T) {
	tokenID := uuid.New()

	setup := ServiceTestSetup[v1.RevokeTokenRequest, v1.RevokeTokenResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.RevokeTokenRequest]) (*connect.Response[v1.RevokeTokenResponse], error) {
			return client.Auth().RevokeToken(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.RevokeTokenResponse{}),
			protocmp.Transform(),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.RevokeTokenRequest, v1.RevokeTokenResponse]{
		{
			Name: "invalid id format",
			Request: &v1.RevokeTokenRequest{
				Id: "not-a-uuid",
			},
			Expected: ServiceTestExpectation[v1.RevokeTokenResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 10",
			},
		},
		{
			Name: "token not found",
			Request: &v1.RevokeTokenRequest{
				Id: "00000000-0000-0000-0000-000000000001",
			},
			Expected: ServiceTestExpectation[v1.RevokeTokenResponse]{
				Error: "not_found: token not found",
			},
		},
		{
			Name: "success",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				test.NewTokenBuilder(t, tokenID, db).WithName("token-to-revoke").Build(ctx)
			},
			Request: &v1.RevokeTokenRequest{
				Id: tokenID.String(),
			},
			Expected: ServiceTestExpectation[v1.RevokeTokenResponse]{
				Response: v1.RevokeTokenResponse{},
			},
		},
	})
}

func TestExchangeSetupCode(t *testing.T) {
	setup := ServiceTestSetup[v1.ExchangeSetupCodeRequest, v1.ExchangeSetupCodeResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.ExchangeSetupCodeRequest]) (*connect.Response[v1.ExchangeSetupCodeResponse], error) {
			return client.Auth().ExchangeSetupCode(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.ExchangeSetupCodeResponse{}),
			protocmp.Transform(),
			protocmp.IgnoreFields(&v1.ExchangeSetupCodeResponse{}, "token", "expires_at"),
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.ExchangeSetupCodeRequest, v1.ExchangeSetupCodeResponse]{
		{
			Name: "empty setup code",
			Request: &v1.ExchangeSetupCodeRequest{
				SetupCode: "",
			},
			Expected: ServiceTestExpectation[v1.ExchangeSetupCodeResponse]{
				Error: "invalid_argument: setup_code is required",
			},
		},
		{
			Name: "invalid setup code",
			Request: &v1.ExchangeSetupCodeRequest{
				SetupCode: "INVALID-CODE",
			},
			Expected: ServiceTestExpectation[v1.ExchangeSetupCodeResponse]{
				Error: "permission_denied: invalid or expired setup code",
			},
		},
	})
}
