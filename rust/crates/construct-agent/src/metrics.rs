//! Per-runtime workqueue and process metrics (no global registration).
use prometheus::{
    Encoder, GaugeVec, HistogramOpts, HistogramVec, IntCounterVec, Opts, Registry, TextEncoder,
};

pub struct QueueMetrics {
    pub registry: Registry,
    pub depth: GaugeVec,
    pub adds: IntCounterVec,
    pub latency: HistogramVec,
    pub work_duration: HistogramVec,
    pub unfinished: GaugeVec,
    pub longest_running_processor: GaugeVec,
    pub retries: IntCounterVec,
}

impl QueueMetrics {
    pub fn new() -> prometheus::Result<Self> {
        let registry = Registry::new();
        let depth = GaugeVec::new(
            Opts::new("workqueue_depth", "Current depth of workqueue"),
            &["name"],
        )?;
        let adds = IntCounterVec::new(
            Opts::new(
                "workqueue_adds_total",
                "Total number of adds handled by workqueue",
            ),
            &["name"],
        )?;
        let buckets = prometheus::exponential_buckets(10e-9, 10.0, 10)?;
        let latency = HistogramVec::new(
            HistogramOpts::new(
                "workqueue_queue_duration_seconds",
                "How long in seconds an item stays in workqueue before being requested",
            )
            .buckets(buckets.clone()),
            &["name"],
        )?;
        let work_duration = HistogramVec::new(
            HistogramOpts::new(
                "workqueue_work_duration_seconds",
                "How long in seconds processing an item from workqueue takes",
            )
            .buckets(buckets),
            &["name"],
        )?;
        let unfinished = GaugeVec::new(Opts::new("workqueue_unfinished_work_seconds", "How many seconds of work has done that is in progress and hasn't been observed by work_duration"), &["name"])?;
        let longest_running_processor = GaugeVec::new(
            Opts::new(
                "workqueue_longest_running_processor_seconds",
                "How many seconds has the longest running processor for workqueue been running",
            ),
            &["name"],
        )?;
        let retries = IntCounterVec::new(
            Opts::new(
                "workqueue_retries_total",
                "Total number of retries handled by workqueue",
            ),
            &["name"],
        )?;
        registry.register(Box::new(depth.clone()))?;
        registry.register(Box::new(adds.clone()))?;
        registry.register(Box::new(latency.clone()))?;
        registry.register(Box::new(work_duration.clone()))?;
        registry.register(Box::new(unfinished.clone()))?;
        registry.register(Box::new(longest_running_processor.clone()))?;
        registry.register(Box::new(retries.clone()))?;
        #[cfg(target_os = "linux")]
        registry.register(Box::new(
            prometheus::process_collector::ProcessCollector::for_self(),
        ))?;
        Ok(Self {
            registry,
            depth,
            adds,
            latency,
            work_duration,
            unfinished,
            longest_running_processor,
            retries,
        })
    }

    pub fn encode_text(&self) -> prometheus::Result<String> {
        let mut bytes = Vec::new();
        TextEncoder::new().encode(&self.registry.gather(), &mut bytes)?;
        String::from_utf8(bytes).map_err(|error| prometheus::Error::Msg(error.to_string()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn metric_names_labels_help_and_buckets() {
        let metrics = QueueMetrics::new().unwrap();
        assert!(!metrics.encode_text().unwrap().contains("workqueue_depth{"));
        metrics.depth.with_label_values(&["tasks"]).set(2.0);
        metrics.adds.with_label_values(&["tasks"]).inc();
        metrics.latency.with_label_values(&["tasks"]).observe(0.1);
        metrics
            .work_duration
            .with_label_values(&["tasks"])
            .observe(0.2);
        metrics.unfinished.with_label_values(&["tasks"]).set(0.3);
        metrics
            .longest_running_processor
            .with_label_values(&["tasks"])
            .set(0.4);
        metrics.retries.with_label_values(&["tasks"]).inc();
        let text = metrics.encode_text().unwrap();
        for name in [
            "depth",
            "adds_total",
            "queue_duration_seconds",
            "work_duration_seconds",
            "unfinished_work_seconds",
            "longest_running_processor_seconds",
            "retries_total",
        ] {
            assert!(
                text.contains(&format!("# HELP workqueue_{name} ")),
                "{name}"
            );
        }
        assert!(text.contains("workqueue_depth{name=\"tasks\"} 2"));
        assert!(text.contains("# HELP workqueue_depth Current depth of workqueue"));
        let families = metrics.registry.gather();
        let latency = families
            .iter()
            .find(|m| m.name() == "workqueue_queue_duration_seconds")
            .unwrap();
        assert_eq!(
            latency.get_metric()[0].get_histogram().get_bucket().len(),
            10
        );
        #[cfg(target_os = "linux")]
        assert!(text.contains("process_cpu_seconds_total"));
    }
}
