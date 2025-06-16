package client

import (
	"context"
	"net"
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

type ClientOptions struct {
	HTTPClient *http.Client
}

func NewClient(endpointContext EndpointContext) *Client {
	httpClient := &http.Client{}

	var baseURL string
	if endpointContext.Type == "unix" {
		httpClient.Transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", endpointContext.Address)
			},
		}

		baseURL = "http://localhost/api" // Include /api prefix for unix sockets
	} else {
		// For HTTP connections, ensure proper scheme and /api prefix
		baseURL = "http://" + endpointContext.Address + "/api"
	}
	return &Client{
		modelProvider: v1connect.NewModelProviderServiceClient(httpClient, baseURL),
		model:         v1connect.NewModelServiceClient(httpClient, baseURL),
		agent:         v1connect.NewAgentServiceClient(httpClient, baseURL),
		task:          v1connect.NewTaskServiceClient(httpClient, baseURL),
		message:       v1connect.NewMessageServiceClient(httpClient, baseURL),
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

type EndpointContexts struct {
	Current  string                     `yaml:"current"`
	Contexts map[string]EndpointContext `yaml:"contexts"`
}

type EndpointContext struct {
	Address string `yaml:"address"`
	Type    string `yaml:"type"`
}

func Ptr[T any](v T) *T {
	return &v
}
