//! Embeddable agent runtime. Transport/API servers are intentionally separate.
use crate::{
    analytics,
    metrics::QueueMetrics,
    provider::{ModelProviderFactory, ProviderFactory},
    queue::WorkQueue,
    reconciler::TaskReconciler,
    Result,
};
use construct_codeact::CodeActTool;
use construct_core::{
    event::{EventSubscriber, SubscribeOptions},
    store::Store,
    CancellationToken,
};
use construct_event::{
    EventHook, EventRouter, InternalTaskSuspendPayload, InternalTaskTriggerPayload,
    DEFAULT_CHANNEL_BUFFER_SIZE, EVENT_TYPE_INTERNAL_TASK_SUSPEND,
    EVENT_TYPE_INTERNAL_TASK_TRIGGER,
};
use construct_secret::Encryption;
use futures_util::StreamExt;
use std::{
    path::Path,
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc,
    },
};
use tokio::{sync::watch, task::JoinSet};

/// Go's default worker concurrency.
pub const DEFAULT_CONCURRENCY: usize = 50;

pub struct RuntimeOptions {
    pub tools: Vec<Arc<dyn CodeActTool>>,
    pub concurrency: usize,
    pub analytics: Arc<dyn analytics::Client>,
    pub skills: Option<construct_skill::Discoverer>,
    /// Override only the provider construction boundary, not reconciliation.
    pub provider_factory: Option<Arc<dyn ProviderFactory>>,
}

impl Default for RuntimeOptions {
    fn default() -> Self {
        Self {
            tools: Vec::new(),
            concurrency: DEFAULT_CONCURRENCY,
            analytics: Arc::new(analytics::Noop),
            skills: construct_skill::UserDirs::from_env().map(construct_skill::Discoverer::new),
            provider_factory: None,
        }
    }
}

/// A single-use runtime with explicit ownership of worker and title shutdown.
pub struct Runtime {
    store: Arc<dyn Store>,
    encryption: Arc<Encryption>,
    router: Arc<EventRouter>,
    reconciler: Arc<TaskReconciler>,
    analytics: Arc<dyn analytics::Client>,
    metrics: Arc<QueueMetrics>,
    concurrency: usize,
    cancellation: CancellationToken,
    invoked: AtomicBool,
    lifecycle: watch::Sender<Lifecycle>,
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum Lifecycle {
    Created,
    Running,
    Finished,
}

impl Runtime {
    /// Use an existing store. The Store trait cannot install hooks after open;
    /// use `open_sqlite` to register the Go-compatible mutation hooks automatically.
    pub fn new(
        store: Arc<dyn Store>,
        encryption: Arc<Encryption>,
        options: RuntimeOptions,
    ) -> Result<Self> {
        Self::with_router(
            store,
            encryption,
            Arc::new(EventRouter::new(DEFAULT_CHANNEL_BUFFER_SIZE)),
            options,
        )
    }

    /// Open SQLite with committed entity mutation events on this runtime's router.
    pub async fn open_sqlite(
        path: impl AsRef<Path>,
        encryption: Arc<Encryption>,
        options: RuntimeOptions,
    ) -> Result<Self> {
        let router = Arc::new(EventRouter::new(DEFAULT_CHANNEL_BUFFER_SIZE));
        let store = Arc::new(
            construct_store::SqliteStore::open_with_hooks(
                path,
                vec![Arc::new(EventHook::new(router.clone()))],
            )
            .await?,
        );
        Self::with_router(store, encryption, router, options)
    }

    fn with_router(
        store: Arc<dyn Store>,
        encryption: Arc<Encryption>,
        router: Arc<EventRouter>,
        options: RuntimeOptions,
    ) -> Result<Self> {
        if options.concurrency == 0 {
            return Err("runtime concurrency must be positive".into());
        }
        let metrics = Arc::new(QueueMetrics::new()?);
        let queue = Arc::new(WorkQueue::new(metrics.clone()));
        let providers = options.provider_factory.unwrap_or_else(|| {
            Arc::new(ModelProviderFactory::new(store.clone(), encryption.clone()))
        });
        let cancellation = CancellationToken::new();
        let reconciler = Arc::new(TaskReconciler::new(
            store.clone(),
            router.clone(),
            options.tools,
            options.skills,
            providers,
            queue,
            cancellation.clone(),
        ));
        let (lifecycle, _) = watch::channel(Lifecycle::Created);
        Ok(Self {
            store,
            encryption,
            router,
            reconciler,
            analytics: options.analytics,
            metrics,
            concurrency: options.concurrency,
            cancellation,
            invoked: AtomicBool::new(false),
            lifecycle,
        })
    }

    pub fn store(&self) -> Arc<dyn Store> {
        self.store.clone()
    }
    pub fn encryption(&self) -> Arc<Encryption> {
        self.encryption.clone()
    }
    pub fn event_router(&self) -> Arc<EventRouter> {
        self.router.clone()
    }
    pub fn metrics(&self) -> Arc<QueueMetrics> {
        self.metrics.clone()
    }
    pub fn reconciler(&self) -> Arc<TaskReconciler> {
        self.reconciler.clone()
    }

    /// Resolves after the internal subscriptions are active (or shutdown completes).
    pub async fn wait_until_running(&self) {
        let mut state = self.lifecycle.subscribe();
        let _ = state.wait_for(|value| *value != Lifecycle::Created).await;
    }

    pub async fn run(self: Arc<Self>, cancellation: CancellationToken) -> Result<()> {
        if self.invoked.swap(true, Ordering::SeqCst) {
            return Err("runtime already run".into());
        }
        let mut events = self.router.subscribe(SubscribeOptions {
            event_types: vec![
                EVENT_TYPE_INTERNAL_TASK_TRIGGER.into(),
                EVENT_TYPE_INTERNAL_TASK_SUSPEND.into(),
            ],
            internal: true,
            ..Default::default()
        });
        let mut workers = JoinSet::new();
        for _ in 0..self.concurrency {
            workers.spawn(self.reconciler.clone().worker());
        }
        self.lifecycle.send_replace(Lifecycle::Running);
        let mut metrics_tick = tokio::time::interval(std::time::Duration::from_millis(500));
        let mut failure = None;
        loop {
            tokio::select! {
                biased;
                _ = self.cancellation.cancelled() => break,
                _ = cancellation.cancelled() => break,
                event = events.next() => {
                    let Some(event) = event else { break; };
                    if let Some(trigger) = event.payload.downcast_ref::<InternalTaskTriggerPayload>() { self.reconciler.queue.add(trigger.task_id); }
                    if let Some(suspend) = event.payload.downcast_ref::<InternalTaskSuspendPayload>() { self.reconciler.suspend(suspend.task_id); }
                }
                _ = metrics_tick.tick() => self.reconciler.queue.refresh_metrics(),
                result = workers.join_next() => {
                    failure = Some(format!("runtime worker exited unexpectedly: {result:?}")); break;
                }
            }
        }
        events.cancel();
        self.cancellation.cancel();
        self.reconciler.queue.shutdown();
        while let Some(result) = workers.join_next().await {
            if let Err(error) = result {
                failure = Some(error.to_string());
            }
        }
        self.reconciler.join_titles().await;
        self.analytics.close().await;
        self.lifecycle.send_replace(Lifecycle::Finished);
        if let Some(error) = failure {
            Err(error.into())
        } else {
            Ok(())
        }
    }

    /// Signal cancellation and wait until workers, titles and analytics finish.
    /// Before `run`, cancellation is remembered and shutdown returns immediately.
    pub async fn shutdown(&self) {
        self.cancellation.cancel();
        if !self.invoked.load(Ordering::SeqCst) {
            return;
        }
        let mut state = self.lifecycle.subscribe();
        let _ = state.wait_for(|value| *value == Lifecycle::Finished).await;
    }
}
