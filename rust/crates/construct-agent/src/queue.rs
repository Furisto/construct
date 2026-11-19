//! Kubernetes-style dirty/processing deduplication with earliest-deadline delays.
use crate::metrics::QueueMetrics;
use std::{
    collections::{HashMap, VecDeque},
    sync::{Arc, Mutex},
    time::Duration,
};
use tokio::{sync::Notify, time::Instant};
use uuid::Uuid;

const NAME: &[&str] = &["construct"];

#[derive(Default)]
struct State {
    ready: VecDeque<Uuid>,
    dirty: HashMap<Uuid, Instant>,
    processing: HashMap<Uuid, Instant>,
    delayed: HashMap<Uuid, Instant>,
    closed: bool,
}

/// An ID is never processed concurrently. Adding during processing marks it dirty
/// for exactly one further pass. Delayed adds retain the earliest deadline.
pub struct WorkQueue {
    state: Mutex<State>,
    changed: Notify,
    metrics: Arc<QueueMetrics>,
}

impl WorkQueue {
    pub fn new(metrics: Arc<QueueMetrics>) -> Self {
        metrics.depth.with_label_values(NAME).set(0.0);
        metrics.retries.with_label_values(NAME);
        Self {
            state: Mutex::new(State::default()),
            changed: Notify::new(),
            metrics,
        }
    }

    fn add_locked(&self, state: &mut State, id: Uuid) {
        if state.closed || state.dirty.contains_key(&id) {
            return;
        }
        state.dirty.insert(id, Instant::now());
        self.metrics.adds.with_label_values(NAME).inc();
        self.metrics
            .depth
            .with_label_values(NAME)
            .set(state.dirty.len() as f64);
        if !state.processing.contains_key(&id) {
            state.ready.push_back(id);
        }
    }

    pub fn add(&self, id: Uuid) {
        self.add_locked(&mut self.state.lock().unwrap(), id);
        self.changed.notify_waiters();
    }

    pub fn add_after(&self, id: Uuid, delay: Duration) {
        if delay.is_zero() {
            self.add(id);
            return;
        }
        let mut state = self.state.lock().unwrap();
        if state.closed {
            return;
        }
        let due = Instant::now() + delay;
        state
            .delayed
            .entry(id)
            .and_modify(|old| *old = (*old).min(due))
            .or_insert(due);
        drop(state);
        self.changed.notify_waiters();
    }

    pub fn retry(&self, id: Uuid, delay: Duration) {
        self.metrics.retries.with_label_values(NAME).inc();
        self.add_after(id, delay);
    }

    pub async fn get(&self) -> Option<Uuid> {
        loop {
            let notified = self.changed.notified();
            tokio::pin!(notified);
            notified.as_mut().enable();
            let next_deadline = {
                let mut state = self.state.lock().unwrap();
                if state.closed {
                    return None;
                }
                let now = Instant::now();
                let due: Vec<_> = state
                    .delayed
                    .iter()
                    .filter(|(_, time)| **time <= now)
                    .map(|(id, _)| *id)
                    .collect();
                for id in due {
                    state.delayed.remove(&id);
                    self.add_locked(&mut state, id);
                }
                if let Some(id) = state.ready.pop_front() {
                    let added = state.dirty.remove(&id).unwrap();
                    state.processing.insert(id, now);
                    self.metrics
                        .depth
                        .with_label_values(NAME)
                        .set(state.dirty.len() as f64);
                    self.metrics
                        .latency
                        .with_label_values(NAME)
                        .observe(added.elapsed().as_secs_f64());
                    return Some(id);
                }
                state.delayed.values().min().copied()
            };
            match next_deadline {
                Some(due) => {
                    tokio::select! { _ = &mut notified => {}, _ = tokio::time::sleep_until(due) => {} }
                }
                None => notified.await,
            }
        }
    }

    pub fn done(&self, id: Uuid) {
        let mut state = self.state.lock().unwrap();
        if let Some(start) = state.processing.remove(&id) {
            self.metrics
                .work_duration
                .with_label_values(NAME)
                .observe(start.elapsed().as_secs_f64());
        }
        if !state.closed && state.dirty.contains_key(&id) {
            state.ready.push_back(id);
        }
        self.refresh_locked(&state);
        drop(state);
        self.changed.notify_waiters();
    }

    fn refresh_locked(&self, state: &State) {
        let ages: Vec<_> = state
            .processing
            .values()
            .map(|start| start.elapsed().as_secs_f64())
            .collect();
        self.metrics
            .unfinished
            .with_label_values(NAME)
            .set(ages.iter().sum());
        self.metrics
            .longest_running_processor
            .with_label_values(NAME)
            .set(ages.into_iter().fold(0.0, f64::max));
    }

    pub fn refresh_metrics(&self) {
        self.refresh_locked(&self.state.lock().unwrap());
    }

    /// Stop accepting work; workers finish their current items before exiting.
    pub fn shutdown(&self) {
        let mut state = self.state.lock().unwrap();
        state.closed = true;
        state.ready.clear();
        state.dirty.clear();
        state.delayed.clear();
        self.metrics.depth.with_label_values(NAME).set(0.0);
        drop(state);
        self.changed.notify_waiters();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn deduplicates_dirty_and_processing_and_closes_empty_queue() {
        let metrics = Arc::new(QueueMetrics::new().unwrap());
        let q = Arc::new(WorkQueue::new(metrics.clone()));
        let id = Uuid::new_v4();
        let mut adds = tokio::task::JoinSet::new();
        for _ in 0..100 {
            let q = q.clone();
            adds.spawn(async move {
                q.add(id);
            });
        }
        while adds.join_next().await.is_some() {}
        assert_eq!(q.get().await, Some(id));
        for _ in 0..100 {
            q.add(id);
        }
        q.done(id);
        assert_eq!(q.get().await, Some(id));
        q.done(id);
        assert!(tokio::time::timeout(Duration::from_millis(10), q.get())
            .await
            .is_err());
        q.shutdown();
        q.add(id);
        assert_eq!(q.get().await, None);
        assert_eq!(metrics.adds.with_label_values(NAME).get(), 2);
        assert_eq!(
            metrics
                .work_duration
                .with_label_values(NAME)
                .get_sample_count(),
            2
        );
        assert_eq!(metrics.depth.with_label_values(NAME).get(), 0.0);
    }

    #[tokio::test]
    async fn delayed_add_uses_earliest_deadline_and_does_not_block_ready_work() {
        let q = WorkQueue::new(Arc::new(QueueMetrics::new().unwrap()));
        let delayed = Uuid::new_v4();
        let ready = Uuid::new_v4();
        q.add_after(delayed, Duration::from_secs(60));
        q.add_after(delayed, Duration::from_millis(20));
        q.add_after(delayed, Duration::from_secs(60));
        q.add(ready);
        assert_eq!(q.get().await, Some(ready));
        q.done(ready);
        assert!(tokio::time::timeout(Duration::from_millis(5), q.get())
            .await
            .is_err());
        assert_eq!(
            tokio::time::timeout(Duration::from_secs(1), q.get())
                .await
                .unwrap(),
            Some(delayed)
        );
        q.done(delayed);
        assert!(tokio::time::timeout(Duration::from_millis(5), q.get())
            .await
            .is_err());
        q.shutdown();
    }
}
