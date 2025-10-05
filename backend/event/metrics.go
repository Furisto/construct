package event

import "github.com/prometheus/client_golang/prometheus"

type eventBusMetricsProvider struct {
	published *prometheus.CounterVec
	delivered *prometheus.CounterVec
	dropped   *prometheus.CounterVec
}

func newEventBusMetricsProvider(registry *prometheus.Registry) *eventBusMetricsProvider {
	if registry == nil {
		return nil
	}

	provider := &eventBusMetricsProvider{
		published: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "eventbus_events_published_total",
				Help: "Total number of events published by event type",
			},
			[]string{"event_type"},
		),
		delivered: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "eventbus_events_delivered_total",
				Help: "Total number of events delivered by event type",
			},
			[]string{"event_type"},
		),
		dropped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "eventbus_events_dropped_total",
				Help: "Total number of events dropped due to full channel buffers",
			},
			[]string{"event_type"},
		),
	}

	registry.MustRegister(
		provider.published,
		provider.delivered,
		provider.dropped,
	)

	return provider
}

func (p *eventBusMetricsProvider) IncrementPublished(eventType string) {
	if p != nil && p.published != nil {
		p.published.WithLabelValues(eventType).Inc()
	}
}

func (p *eventBusMetricsProvider) IncrementDelivered(eventType string) {
	if p != nil && p.delivered != nil {
		p.delivered.WithLabelValues(eventType).Inc()
	}
}

func (p *eventBusMetricsProvider) IncrementDropped(eventType string) {
	if p != nil && p.dropped != nil {
		p.dropped.WithLabelValues(eventType).Inc()
	}
}
