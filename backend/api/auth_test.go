package api

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
				createTestToken(t, ctx, db, "duplicate-token")
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
				createTestToken(t, ctx, db, "token-1")
				createTestToken(t, ctx, db, "token-2")
				createTestToken(t, ctx, db, "token-3")
			},
			Request: &v1.ListTokensRequest{},
			Expected: ServiceTestExpectation[v1.ListTokensResponse]{
				Response: v1.ListTokensResponse{
					Tokens: []*v1.TokenInfo{
						{Name: "token-1", IsActive: true},
						{Name: "token-2", IsActive: true},
						{Name: "token-3", IsActive: true},
					},
				},
			},
		},
		{
			Name: "filter by name prefix",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				createTestToken(t, ctx, db, "prod-token")
				createTestToken(t, ctx, db, "dev-token")
				createTestToken(t, ctx, db, "staging-token")
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
				createTestToken(t, ctx, db, "active-token")
				createExpiredToken(t, ctx, db, "expired-token")
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
				createTestToken(t, ctx, db, "active-token")
				createExpiredToken(t, ctx, db, "expired-token")
			},
			Request: &v1.ListTokensRequest{
				IncludeExpired: true,
			},
			Expected: ServiceTestExpectation[v1.ListTokensResponse]{
				Response: v1.ListTokensResponse{
					Tokens: []*v1.TokenInfo{
						{Name: "active-token", IsActive: true},
						{Name: "expired-token", IsActive: false},
					},
				},
			},
		},
	})
}

func TestRevokeToken(t *testing.T) {
	setup := ServiceTestSetup[v1.RevokeTokenRequest, v1.RevokeTokenResponse]{
		Call: func(ctx context.Context, client *client.Client, req *connect.Request[v1.RevokeTokenRequest]) (*connect.Response[v1.RevokeTokenResponse], error) {
			return client.Auth().RevokeToken(ctx, req)
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreUnexported(v1.RevokeTokenResponse{}),
			protocmp.Transform(),
		},
		QueryDatabase: func(ctx context.Context, db *memory.Client) (any, error) {
			tokens, err := db.Token.Query().All(ctx)
			if err != nil {
				return nil, err
			}
			for _, tok := range tokens {
				tokenIDCache[tok.Name] = tok.ID.String()
			}
			return nil, nil
		},
	}

	setup.RunServiceTests(t, []ServiceTestScenario[v1.RevokeTokenRequest, v1.RevokeTokenResponse]{
		{
			Name: "invalid id format",
			Request: &v1.RevokeTokenRequest{
				Id: "not-a-uuid",
			},
			Expected: ServiceTestExpectation[v1.RevokeTokenResponse]{
				Error: "invalid_argument: invalid ID format: invalid UUID length: 11",
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
				tok := createTestTokenWithReturn(t, ctx, db, "token-to-revoke")
				tokenIDCache["token-to-revoke"] = tok.ID.String()
			},
			Request: &v1.RevokeTokenRequest{
				Id: getTokenID(t, "token-to-revoke"),
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

func createTestToken(t *testing.T, ctx context.Context, db *memory.Client, name string) {
	t.Helper()
	_ = createTestTokenWithReturn(t, ctx, db, name)
}

func createTestTokenWithReturn(t *testing.T, ctx context.Context, db *memory.Client, name string) *memory.Token {
	t.Helper()
	hash := []byte("test-hash-" + name)
	tok, err := db.Token.Create().
		SetName(name).
		SetType("api_token").
		SetTokenHash(hash).
		SetExpiresAt(time.Now().Add(90 * 24 * time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create test token: %v", err)
	}
	return tok
}

func createExpiredToken(t *testing.T, ctx context.Context, db *memory.Client, name string) {
	t.Helper()
	hash := []byte("test-hash-" + name)
	_, err := db.Token.Create().
		SetName(name).
		SetType("api_token").
		SetTokenHash(hash).
		SetExpiresAt(time.Now().Add(-24 * time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create expired token: %v", err)
	}
}

var tokenIDCache = make(map[string]string)

func getTokenID(t *testing.T, name string) string {
	t.Helper()
	if id, ok := tokenIDCache[name]; ok {
		return id
	}
	return "00000000-0000-0000-0000-000000000000"
}
