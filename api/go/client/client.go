package client

import (
	"context"
	"net/http"

	"github.com/furisto/construct/api/go/v1/v1connect"
)

type Client struct {
	modelProvider v1connect.ModelProviderServiceClient
	agent         v1connect.AgentServiceClient
	task          v1connect.TaskServiceClient
	message       v1connect.MessageServiceClient
}

func NewClient(ctx context.Context, url string) (*Client, error) {
	return &Client{
		modelProvider: v1connect.NewModelProviderServiceClient(http.DefaultClient, url),
		agent:         v1connect.NewAgentServiceClient(http.DefaultClient, url),
		task:          v1connect.NewTaskServiceClient(http.DefaultClient, url),
		message:       v1connect.NewMessageServiceClient(http.DefaultClient, url),
	}, nil
}

func (c *Client) ModelProvider() v1connect.ModelProviderServiceClient {
	return c.modelProvider
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
