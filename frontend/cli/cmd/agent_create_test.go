package cmd

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"go.uber.org/mock/gomock"
)

func TestAgentCreate(t *testing.T) {
	setup := &TestSetup{}

	agentID := uuid.New().String()
	modelID := uuid.New().String()

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success with inline prompt",
			Command: []string{"agent", "create", "coder", "--prompt", "A helpful coding assistant", "--model", "gpt-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupMock(mockClient, "gpt-4", modelID)
				setupAgentCreationMock(mockClient, "coder", "A helpful coding assistant", "", modelID, agentID)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(agentID),
			},
		},
		{
			Name:    "success with description",
			Command: []string{"agent", "create", "coder", "--description", "An agent that helps with coding tasks", "--prompt", "A helpful coding assistant", "--model", "claude-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupMock(mockClient, "claude-4", modelID)
				setupAgentCreationMock(mockClient, "coder", "A helpful coding assistant", "An agent that helps with coding tasks", modelID, agentID)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(agentID),
			},
		},
		{
			Name:    "success with model ID",
			Command: []string{"agent", "create", "coder", "--prompt", "A helpful coding assistant", "--model", modelID},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupAgentCreationMock(mockClient, "coder", "A helpful coding assistant", "", modelID, agentID)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(agentID),
			},
		},
		{
			Name:    "success with prompt from stdin",
			Command: []string{"agent", "create", "coder", "--prompt-stdin", "--model", "gpt-4"},
			Stdin:   "A helpful coding assistant",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupMock(mockClient, "gpt-4", modelID)
				setupAgentCreationMock(mockClient, "coder", "A helpful coding assistant", "", modelID, agentID)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(agentID),
			},
		},
		{
			Name:    "success with prompt from file",
			Command: []string{"agent", "create", "coder", "--prompt-file", "test-prompt.txt", "--model", "gpt-4"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupModelLookupMock(mockClient, "gpt-4", modelID)
				setupAgentCreationMock(mockClient, "coder", "A helpful coding assistant", "", modelID, agentID)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("test-prompt.txt", []byte("A helpful coding assistant"), 0644)
			},
			Expected: TestExpectation{
				Stdout: fmt.Sprintln(agentID),
			},
		},
		{
			Name:    "error - no prompt provided",
			Command: []string{"agent", "create", "coder", "--model", "gpt-4"},
			Expected: TestExpectation{
				Error: "system prompt is required (use --prompt, --prompt-file, or --prompt-stdin)",
			},
		},
		{
			Name:    "error - multiple prompt sources",
			Command: []string{"agent", "create", "coder", "--prompt", "A helpful coding assistant", "--prompt-stdin", "--model", "gpt-4"},
			Stdin:   "A helpful coding assistant",
			Expected: TestExpectation{
				Error: "only one prompt source can be specified (--prompt, --prompt-file, or --prompt-stdin)",
			},
		},
		{
			Name:    "error - model not found",
			Command: []string{"agent", "create", "coder", "--prompt", "A helpful coding assistant", "--model", "nonexistent-model"},
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
			Name:    "error - empty stdin",
			Command: []string{"agent", "create", "coder", "--prompt-stdin", "--model", "gpt-4"},
			Stdin:   "",
			Expected: TestExpectation{
				Error: "no prompt content received from stdin",
			},
		},
		{
			Name:    "error - multiple models found",
			Command: []string{"agent", "create", "coder", "--prompt", "A helpful coding assistant", "--model", "gpt"},
			SetupMocks: func(mockClient *api_client.MockClient) {
				mockClient.Model.EXPECT().ListModels(
					gomock.Any(),
					&connect.Request[v1.ListModelsRequest]{
						Msg: &v1.ListModelsRequest{
							Filter: &v1.ListModelsRequest_Filter{
								Name: api_client.Ptr("gpt"),
							},
						},
					},
				).Return(&connect.Response[v1.ListModelsResponse]{
					Msg: &v1.ListModelsResponse{
						Models: []*v1.Model{
							{
								Id:   uuid.New().String(),
								Name: "gpt-3.5-turbo",
							},
							{
								Id:   uuid.New().String(),
								Name: "gpt-4",
							},
						},
					},
				}, nil)
			},
			Expected: TestExpectation{
				Error: "multiple models found for gpt",
			},
		},
		{
			Name:    "error - prompt file doesn't exist",
			Command: []string{"agent", "create", "coder", "--prompt-file", "nonexistent.txt", "--model", "gpt-4"},
			Expected: TestExpectation{
				Error: "failed to read prompt file nonexistent.txt: open nonexistent.txt: file does not exist",
			},
		},
		{
			Name:    "error - prompt file is empty",
			Command: []string{"agent", "create", "coder", "--prompt-file", "empty-prompt.txt", "--model", "gpt-4"},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("empty-prompt.txt", []byte(""), 0644)
			},
			Expected: TestExpectation{
				Error: "prompt file empty-prompt.txt is empty",
			},
		},
	})
}

func setupModelLookupMock(mockClient *api_client.MockClient, modelName, modelID string) {
	mockClient.Model.EXPECT().ListModels(
		gomock.Any(),
		&connect.Request[v1.ListModelsRequest]{
			Msg: &v1.ListModelsRequest{
				Filter: &v1.ListModelsRequest_Filter{
					Name: api_client.Ptr(modelName),
				},
			},
		},
	).Return(&connect.Response[v1.ListModelsResponse]{
		Msg: &v1.ListModelsResponse{
			Models: []*v1.Model{
				{
					Id:   modelID,
					Name: modelName,
				},
			},
		},
	}, nil)
}

func setupAgentCreationMock(mockClient *api_client.MockClient, agentName, instructions, description, modelID, agentID string) {
	req := &v1.CreateAgentRequest{
		Name:         agentName,
		Instructions: instructions,
		ModelId:      modelID,
	}

	metadata := &v1.AgentMetadata{
		Name: agentName,
	}

	if description != "" {
		req.Description = description
		metadata.Description = description
	}

	mockClient.Agent.EXPECT().CreateAgent(
		gomock.Any(),
		connect.NewRequest(req),
	).Return(&connect.Response[v1.CreateAgentResponse]{
		Msg: &v1.CreateAgentResponse{
			Agent: &v1.Agent{
				Id:       agentID,
				Metadata: metadata,
			},
		},
	}, nil)
}
