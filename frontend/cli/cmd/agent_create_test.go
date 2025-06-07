package cmd

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestAgentCreateCLI(t *testing.T) {
	setup := &TestSetup{}

	agentID := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success with inline prompt",
			Command: []string{"agent", "create", "test-agent", "--prompt", "You are a helpful assistant", "--model", "gpt-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("gpt-4"),
							},
						},
					},
				).Return(&connect.Response[v1.ListModelsResponse]{
					Msg: &v1.ListModelsResponse{
						Models: []*v1.Model{
							{
								Id:   uuid.New().String(),
								Name: "gpt-4",
							},
						},
					},
				}, nil)

				mockClient.Agent.EXPECT().CreateAgent(
					gomock.Any(),
					gomock.Any(),
				).Return(&connect.Response[v1.CreateAgentResponse]{
					Msg: &v1.CreateAgentResponse{
						Agent: &v1.Agent{
							Id: agentID,
							Metadata: &v1.AgentMetadata{
								Name: "test-agent",
							},
						},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(agentID),
			},
		},
		{
			Name:    "success with description",
			Command: []string{"agent", "create", "coding-assistant", "--description", "A helpful coding assistant", "--prompt", "You help with coding tasks", "--model", "claude-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				// Setup model lookup
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("claude-4"),
							},
						},
					},
				).Return(&connect.Response[v1.ListModelsResponse]{
					Msg: &v1.ListModelsResponse{
						Models: []*v1.Model{
							{
								Id:   uuid.New().String(),
								Name: "claude-4",
							},
						},
					},
				}, nil)

				// Setup agent creation
				mockClient.Agent.EXPECT().CreateAgent(
					gomock.Any(),
					gomock.Any(),
				).Return(&connect.Response[v1.CreateAgentResponse]{
					Msg: &v1.CreateAgentResponse{
						Agent: &v1.Agent{
							Id: uuid.New().String(),
							Metadata: &v1.AgentMetadata{
								Name:        "coding-assistant",
								Description: "A helpful coding assistant",
							},
						},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:    "success with model ID",
			Command: []string{"agent", "create", "test-agent", "--prompt", "You are helpful", "--model", uuid.New().String()},
			SetupMocks: func(mockClient *api_client.MockClient) {
				// Setup agent creation (no model lookup needed for UUID)
				mockClient.Agent.EXPECT().CreateAgent(
					gomock.Any(),
					gomock.Any(),
				).Return(&connect.Response[v1.CreateAgentResponse]{
					Msg: &v1.CreateAgentResponse{
						Agent: &v1.Agent{
							Id: uuid.New().String(),
							Metadata: &v1.AgentMetadata{
								Name: "test-agent",
							},
						},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:    "success with prompt from stdin",
			Command: []string{"agent", "create", "stdin-agent", "--prompt-stdin", "--model", "gpt-4"},
			Stdin:   "You are a helpful assistant from stdin",
			SetupMocks: func(mockClient *api_client.MockClient) {
				// Setup model lookup
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("gpt-4"),
							},
						},
					},
				).Return(&connect.Response[v1.ListModelsResponse]{
					Msg: &v1.ListModelsResponse{
						Models: []*v1.Model{
							{
								Id:   uuid.New().String(),
								Name: "gpt-4",
							},
						},
					},
				}, nil)

				// Setup agent creation
				mockClient.Agent.EXPECT().CreateAgent(
					gomock.Any(),
					gomock.Any(),
				).Return(&connect.Response[v1.CreateAgentResponse]{
					Msg: &v1.CreateAgentResponse{
						Agent: &v1.Agent{
							Id: uuid.New().String(),
							Metadata: &v1.AgentMetadata{
								Name: "stdin-agent",
							},
						},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:    "error - no prompt provided",
			Command: []string{"agent", "create", "test-agent", "--model", "gpt-4"},
			Expected: TestExpectation{
				Error: "system prompt is required (use --prompt, --prompt-file, or --prompt-stdin)",
			},
		},
		{
			Name:    "error - multiple prompt sources",
			Command: []string{"agent", "create", "test-agent", "--prompt", "inline prompt", "--prompt-stdin", "--model", "gpt-4"},
			Stdin:   "stdin prompt",
			Expected: TestExpectation{
				Error: "only one prompt source can be specified (--prompt, --prompt-file, or --prompt-stdin)",
			},
		},
		{
			Name:    "error - model not found",
			Command: []string{"agent", "create", "test-agent", "--prompt", "You are helpful", "--model", "nonexistent-model"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					gomock.Any(),
				).Return(&connect.Response[v1.ListModelsResponse]{
					Msg: &v1.ListModelsResponse{
						Models: []*v1.Model{},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Error: "model nonexistent-model not found",
			},
		},
		{
			Name:    "error - invalid model ID format",
			Command: []string{"agent", "create", "test-agent", "--prompt", "You are helpful", "--model", "not-a-valid-uuid"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Agent.EXPECT().CreateAgent(
					gomock.Any(),
					gomock.Any(),
				).Return(nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid model ID format: invalid UUID length: 16")))
			},
			Expected: TestExpectation{
				Error: "failed to create agent: invalid_argument: invalid model ID format: invalid UUID length: 16",
			},
		},
		{
			Name:    "error - empty stdin",
			Command: []string{"agent", "create", "test-agent", "--prompt-stdin", "--model", "gpt-4"},
			Stdin:   "",
			Expected: TestExpectation{
				Error: "no prompt content received from stdin",
			},
		},
	})
}
