package cmd

import (
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestModelProviderList(t *testing.T) {
	setup := &TestSetup{}

	modelProviderID1 := uuid.New().String()
	modelProviderID2 := uuid.New().String()
	modelProviderID3 := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success - list all model providers",
			Command: []string{"modelprovider", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderListMock(mockClient, nil, []*v1.ModelProvider{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      true,
					},
					{
						Id:           modelProviderID2,
						Name:         "openai-prod",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
						Enabled:      true,
					},
					{
						Id:           modelProviderID3,
						Name:         "anthropic-disabled",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      false,
					},
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelProviderDisplay{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: ModelProviderTypeAnthropic,
						Enabled:      true,
					},
					{
						Id:           modelProviderID2,
						Name:         "openai-prod",
						ProviderType: ModelProviderTypeOpenAI,
						Enabled:      true,
					},
					{
						Id:           modelProviderID3,
						Name:         "anthropic-disabled",
						ProviderType: ModelProviderTypeAnthropic,
						Enabled:      false,
					},
				},
			},
		},
		{
			Name:    "success - list model providers with single provider type filter",
			Command: []string{"modelprovider", "list", "--provider-type", "anthropic"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderListMock(mockClient, []v1.ModelProviderType{
					v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
				}, []*v1.ModelProvider{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      true,
					},
					{
						Id:           modelProviderID3,
						Name:         "anthropic-disabled",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      false,
					},
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelProviderDisplay{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: ModelProviderTypeAnthropic,
						Enabled:      true,
					},
					{
						Id:           modelProviderID3,
						Name:         "anthropic-disabled",
						ProviderType: ModelProviderTypeAnthropic,
						Enabled:      false,
					},
				},
			},
		},
		{
			Name:    "success - list model providers with multiple provider type filters",
			Command: []string{"modelprovider", "list", "--provider-type", "anthropic", "--provider-type", "openai"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderListMock(mockClient, []v1.ModelProviderType{
					v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
					v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
				}, []*v1.ModelProvider{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      true,
					},
					{
						Id:           modelProviderID2,
						Name:         "openai-prod",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
						Enabled:      true,
					},
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelProviderDisplay{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: ModelProviderTypeAnthropic,
						Enabled:      true,
					},
					{
						Id:           modelProviderID2,
						Name:         "openai-prod",
						ProviderType: ModelProviderTypeOpenAI,
						Enabled:      true,
					},
				},
			},
		},
		{
			Name:    "success - list model providers with multiple provider type filters (comma separated)",
			Command: []string{"modelprovider", "list", "--provider-type", "anthropic,openai"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderListMock(mockClient, []v1.ModelProviderType{
					v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
					v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
				}, []*v1.ModelProvider{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      true,
					},
					{
						Id:           modelProviderID2,
						Name:         "openai-prod",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
						Enabled:      true,
					},
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelProviderDisplay{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: ModelProviderTypeAnthropic,
						Enabled:      true,
					},
					{
						Id:           modelProviderID2,
						Name:         "openai-prod",
						ProviderType: ModelProviderTypeOpenAI,
						Enabled:      true,
					},
				},
			},
		},
		{
			Name:    "success - list model providers with short flag",
			Command: []string{"modelprovider", "list", "-t", "openai"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderListMock(mockClient, []v1.ModelProviderType{
					v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
				}, []*v1.ModelProvider{
					{
						Id:           modelProviderID2,
						Name:         "openai-prod",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
						Enabled:      true,
					},
				})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelProviderDisplay{
					{
						Id:           modelProviderID2,
						Name:         "openai-prod",
						ProviderType: ModelProviderTypeOpenAI,
						Enabled:      true,
					},
				},
			},
		},
		{
			Name:    "success - list model providers with JSON output",
			Command: []string{"modelprovider", "list", "--output", "json"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderListMock(mockClient, nil, []*v1.ModelProvider{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC,
						Enabled:      true,
					},
				})
			},
			Expected: TestExpectation{
				DisplayFormat: OutputFormatJSON,
				DisplayedObjects: []*ModelProviderDisplay{
					{
						Id:           modelProviderID1,
						Name:         "anthropic-dev",
						ProviderType: ModelProviderTypeAnthropic,
						Enabled:      true,
					},
				},
			},
		},
		{
			Name:    "success - empty list",
			Command: []string{"modelprovider", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelProviderListMock(mockClient, nil, []*v1.ModelProvider{})
			},
			Expected: TestExpectation{
				DisplayedObjects: []*ModelProviderDisplay{},
			},
		},
		{
			Name:    "error - invalid provider type",
			Command: []string{"modelprovider", "list", "--provider-type", "luminal"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				// No mocks needed as validation happens before API call
			},
			Expected: TestExpectation{
				Error: `invalid argument "luminal" for "-t, --provider-type" flag: must be one of "openai" or "anthropic"`,
			},
		},
		{
			Name:    "error - list model providers API failure",
			Command: []string{"modelprovider", "list"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.ModelProvider.EXPECT().ListModelProviders(
					gomock.Any(),
					&connect.Request[v1.ListModelProvidersRequest]{
						Msg: &v1.ListModelProvidersRequest{
							Filter: &v1.ListModelProvidersRequest_Filter{},
						},
					},
				).Return(nil, connect.NewError(connect.CodeInternal, nil))
			},
			Expected: TestExpectation{
				Error: "failed to list model providers: internal",
			},
		},
	})
}

func setupModelProviderListMock(mockClient *api_client.MockClient, providerTypes []v1.ModelProviderType, modelProviders []*v1.ModelProvider) {
	filter := &v1.ListModelProvidersRequest_Filter{}
	if len(providerTypes) > 0 {
		filter.ProviderTypes = providerTypes
	}

	mockClient.ModelProvider.EXPECT().ListModelProviders(
		gomock.Any(),
		&connect.Request[v1.ListModelProvidersRequest]{
			Msg: &v1.ListModelProvidersRequest{
				Filter: filter,
			},
		},
	).Return(&connect.Response[v1.ListModelProvidersResponse]{
		Msg: &v1.ListModelProvidersResponse{
			ModelProviders: modelProviders,
		},
	}, nil)
}
