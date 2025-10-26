package agent

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/workqueue"
)

// workqueueMetricsProvider implements workqueue.MetricsProvider using prometheus metrics
type workqueueMetricsProvider struct {
	depth                   *prometheus.GaugeVec
	adds                    *prometheus.CounterVec
	latency                 *prometheus.HistogramVec
	workDuration            *prometheus.HistogramVec
	unfinished              *prometheus.GaugeVec
	longestRunningProcessor *prometheus.GaugeVec
	retries                 *prometheus.CounterVec
}

func newWorkqueueMetricsProvider(registry prometheus.Registerer) *workqueueMetricsProvider {
	p := &workqueueMetricsProvider{
		depth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: "workqueue",
			Name:      "depth",
			Help:      "Current depth of workqueue",
		}, []string{"name"}),
		adds: prometheus.NewCounterVec(prometheus.CounterOpts{
			Subsystem: "workqueue",
			Name:      "adds_total",
			Help:      "Total number of adds handled by workqueue",
		}, []string{"name"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: "workqueue",
			Name:      "queue_duration_seconds",
			Help:      "How long in seconds an item stays in workqueue before being requested",
			Buckets:   prometheus.ExponentialBuckets(10e-9, 10, 10),
		}, []string{"name"}),
		workDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: "workqueue",
			Name:      "work_duration_seconds",
			Help:      "How long in seconds processing an item from workqueue takes",
			Buckets:   prometheus.ExponentialBuckets(10e-9, 10, 10),
		}, []string{"name"}),
		unfinished: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: "workqueue",
			Name:      "unfinished_work_seconds",
			Help:      "How many seconds of work has done that is in progress and hasn't been observed by work_duration",
		}, []string{"name"}),
		longestRunningProcessor: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: "workqueue",
			Name:      "longest_running_processor_seconds",
			Help:      "How many seconds has the longest running processor for workqueue been running",
		}, []string{"name"}),
		retries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Subsystem: "workqueue",
			Name:      "retries_total",
			Help:      "Total number of retries handled by workqueue",
		}, []string{"name"}),
	}

	registry.MustRegister(p.depth, p.adds, p.latency, p.workDuration, p.unfinished, p.longestRunningProcessor, p.retries)
	return p
}

func (p *workqueueMetricsProvider) NewDepthMetric(name string) workqueue.GaugeMetric {
	return p.depth.WithLabelValues(name)
}

func (p *workqueueMetricsProvider) NewAddsMetric(name string) workqueue.CounterMetric {
	return p.adds.WithLabelValues(name)
}

func (p *workqueueMetricsProvider) NewLatencyMetric(name string) workqueue.HistogramMetric {
	return p.latency.WithLabelValues(name)
}

func (p *workqueueMetricsProvider) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	return p.workDuration.WithLabelValues(name)
}

func (p *workqueueMetricsProvider) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return p.unfinished.WithLabelValues(name)
}

func (p *workqueueMetricsProvider) NewLongestRunningProcessorSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return p.longestRunningProcessor.WithLabelValues(name)
}

func (p *workqueueMetricsProvider) NewRetriesMetric(name string) workqueue.CounterMetric {
	return p.retries.WithLabelValues(name)
}
