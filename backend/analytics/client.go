package analytics

import (
	"github.com/posthog/posthog-go"
)

type Event struct {
	DistinctId string
	Event      string
	Properties map[string]interface{}
}

type Client interface {
	Enqueue(event Event)
	Close()
}

type PostHogClient struct {
	client posthog.Client
}

func NewPostHogClient() (*PostHogClient, error) {
	client, err := posthog.NewWithConfig("phc_YcYtQoT1FASKJ6PZBF61NeGibEcKkw1v6aEd4udQfND", posthog.Config{Endpoint: "https://eu.i.posthog.com"})
	if err != nil {
		return nil, err
	}
	return &PostHogClient{client: client}, nil
}

func (c *PostHogClient) Enqueue(event Event) {
	c.client.Enqueue(posthog.Capture{
		DistinctId: event.DistinctId,
		Event:      event.Event,
		Properties: event.Properties,
	})
}

func (c *PostHogClient) Close() {
	c.client.Close()
}

var _ Client = (*PostHogClient)(nil)
