package client

import (
	"context"
	"net/http"

	"github.com/furisto/construct/api/go/v1/v1connect"
)

type Client struct {
	modelProvider v1connect.ModelProviderServiceClient
}

func NewClient(ctx context.Context, url string) (*Client, error) {
	return &Client{
		modelProvider: v1connect.NewModelProviderServiceClient(http.DefaultClient, url),
	}, nil
}

func (c *Client) ModelProvider() v1connect.ModelProviderServiceClient {
	return c.modelProvider
}
