package client

import (
	"net/http"

	"github.com/furisto/construct/api/go/client/mocks"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"go.uber.org/mock/gomock"
)

type Client struct {
	modelProvider v1connect.ModelProviderServiceClient
	model         v1connect.ModelServiceClient
	agent         v1connect.AgentServiceClient
	task          v1connect.TaskServiceClient
	message       v1connect.MessageServiceClient
}

func NewClient(url string) *Client {
	return &Client{
		modelProvider: v1connect.NewModelProviderServiceClient(http.DefaultClient, url),
		model:         v1connect.NewModelServiceClient(http.DefaultClient, url),
		agent:         v1connect.NewAgentServiceClient(http.DefaultClient, url),
		task:          v1connect.NewTaskServiceClient(http.DefaultClient, url),
		message:       v1connect.NewMessageServiceClient(http.DefaultClient, url),
	}
}

func (c *Client) ModelProvider() v1connect.ModelProviderServiceClient {
	return c.modelProvider
}

func (c *Client) Model() v1connect.ModelServiceClient {
	return c.model
}

func (c *Client) Agent() v1connect.AgentServiceClient {
	return c.agent
}

func (c *Client) Task() v1connect.TaskServiceClient {
	return c.task
}

func (c *Client) Message() v1connect.MessageServiceClient {
	return c.message
}

type MockClient struct {
	ModelProvider *mocks.MockModelProviderServiceClient
	Model         *mocks.MockModelServiceClient
	Agent         *mocks.MockAgentServiceClient
	Task          *mocks.MockTaskServiceClient
	Message       *mocks.MockMessageServiceClient
}

func NewMockClient(ctrl *gomock.Controller) *MockClient {
	return &MockClient{
		ModelProvider: mocks.NewMockModelProviderServiceClient(ctrl),
		Model:         mocks.NewMockModelServiceClient(ctrl),
		Agent:         mocks.NewMockAgentServiceClient(ctrl),
		Task:          mocks.NewMockTaskServiceClient(ctrl),
		Message:       mocks.NewMockMessageServiceClient(ctrl),
	}
}

func (c *MockClient) Client() *Client {
	return &Client{
		modelProvider: c.ModelProvider,
		model:         c.Model,
		agent:         c.Agent,
		task:          c.Task,
		message:       c.Message,
	}
}

func Ptr[T any](v T) *T {
	return &v
}
