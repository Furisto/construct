package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
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
	HTTPClient     *http.Client
	ConnectOptions []connect.ClientOption
}

type ClientOption func(*ClientOptions)

func WithHTTPClient(client *http.Client) ClientOption {
	return func(o *ClientOptions) {
		o.HTTPClient = client
	}
}

func WithConnectOptions(options ...connect.ClientOption) ClientOption {
	return func(o *ClientOptions) {
		o.ConnectOptions = append(o.ConnectOptions, options...)
	}
}

func WithAuthToken(token string) ClientOption {
	return func(o *ClientOptions) {
		interceptor := newAuthInterceptor(token)
		o.ConnectOptions = append(o.ConnectOptions, connect.WithInterceptors(interceptor))
	}
}

type authInterceptor struct {
	token string
}

func newAuthInterceptor(token string) *authInterceptor {
	return &authInterceptor{token: token}
}

func (i *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("Authorization", "Bearer "+i.token)
		return next(ctx, req)
	}
}

func (i *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("Authorization", "Bearer "+i.token)
		return conn
	}
}

func (i *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func NewClient(endpointContext EndpointContext, options ...ClientOption) (*Client, error) {
	opts := ClientOptions{
		HTTPClient: &http.Client{},
	}
	for _, option := range options {
		option(&opts)
	}

	baseURL := endpointContext.Address
	if endpointContext.Kind == "unix" {
		opts.HTTPClient.Transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", endpointContext.Address)
			},
		}
		baseURL = "http://unix"
	}
	baseURL, err := url.JoinPath(baseURL, "api")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL %s: %w", baseURL, err)
	}

	return &Client{
		modelProvider: v1connect.NewModelProviderServiceClient(opts.HTTPClient, baseURL, opts.ConnectOptions...),
		model:         v1connect.NewModelServiceClient(opts.HTTPClient, baseURL, opts.ConnectOptions...),
		agent:         v1connect.NewAgentServiceClient(opts.HTTPClient, baseURL, opts.ConnectOptions...),
		task:          v1connect.NewTaskServiceClient(opts.HTTPClient, baseURL, opts.ConnectOptions...),
		message:       v1connect.NewMessageServiceClient(opts.HTTPClient, baseURL, opts.ConnectOptions...),
	}, nil
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
	CurrentContext  string                     `yaml:"current"`
	PreviousContext string                     `yaml:"previous"`
	Contexts        map[string]EndpointContext `yaml:"contexts"`
}

func (c *EndpointContexts) Validate() error {
	for name, context := range c.Contexts {
		if err := context.Validate(); err != nil {
			return fmt.Errorf("invalid context '%s': %w", name, err)
		}
	}

	if c.CurrentContext != "" {
		if _, ok := c.Contexts[c.CurrentContext]; !ok {
			return fmt.Errorf("current context %s not found", c.CurrentContext)
		}
	}

	return nil
}

func (c *EndpointContexts) Current() (EndpointContext, bool) {
	context, ok := c.Contexts[c.CurrentContext]
	return context, ok
}

func (c *EndpointContexts) SetCurrent(contextName string) error {
	if contextName == "" {
		return fmt.Errorf("context name is required")
	}

	if _, ok := c.Contexts[contextName]; !ok {
		return fmt.Errorf("context %s not found", contextName)
	}

	c.CurrentContext = contextName
	return nil
}

func (c *EndpointContexts) SetPrevious(contextName string) error {
	if contextName == "" {
		return fmt.Errorf("context name is required")
	}

	c.PreviousContext = contextName
	return nil
}

const (
	AuthTypeToken    = "token"
	KeyringRefPrefix = "keyring://construct/"
)

type AuthConfig struct {
	Type     string `yaml:"type,omitempty"`
	Token    string `yaml:"token,omitempty"`
	TokenRef string `yaml:"token-ref,omitempty"`
}

func (a *AuthConfig) Validate() error {
	if a == nil {
		return nil
	}

	if a.Type != "" && a.Type != AuthTypeToken {
		return fmt.Errorf("invalid auth type: %s (supported: %s)", a.Type, AuthTypeToken)
	}

	if a.Token != "" && a.TokenRef != "" {
		return fmt.Errorf("cannot specify both token and token-ref")
	}

	if a.TokenRef != "" && !strings.HasPrefix(a.TokenRef, KeyringRefPrefix) {
		return fmt.Errorf("invalid token-ref format: must start with %s", KeyringRefPrefix)
	}

	if a.Type == AuthTypeToken && a.Token == "" && a.TokenRef == "" {
		return fmt.Errorf("auth type 'token' requires either token or token-ref")
	}

	return nil
}

func (a *AuthConfig) IsConfigured() bool {
	return a != nil && (a.Token != "" || a.TokenRef != "")
}

func (a *AuthConfig) KeyringKey() string {
	if a == nil || a.TokenRef == "" {
		return ""
	}
	return strings.TrimPrefix(a.TokenRef, KeyringRefPrefix)
}

type EndpointContext struct {
	Address string      `yaml:"address"`
	Kind    string      `yaml:"kind"`
	Auth    *AuthConfig `yaml:"auth,omitempty"`
}

func (c *EndpointContext) Validate() error {
	if c.Kind != "unix" && c.Kind != "http" {
		return fmt.Errorf("invalid kind: %s", c.Kind)
	}

	if c.Kind == "unix" {
		if !filepath.IsAbs(c.Address) {
			return fmt.Errorf("unix address must be an absolute path: %s", c.Address)
		}
	}

	if c.Kind == "http" {
		if _, err := url.Parse(c.Address); err != nil {
			return fmt.Errorf("invalid http address: %s", c.Address)
		}
	}

	if err := c.Auth.Validate(); err != nil {
		return fmt.Errorf("invalid auth config: %w", err)
	}

	return nil
}

func Ptr[T any](v T) *T {
	return &v
}
