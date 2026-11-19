use crate::*;
use construct_store::{EntityKind, HookRecord, MutationHook};
use std::sync::Arc;
use uuid::Uuid;

/// Publish Go-compatible entity events from committed SQLite mutations.
///
/// Pass this hook to `SqliteStore::open_with_hooks`. Task updates and message mutations
/// are published by their producers, not by the store.
pub struct EventHook {
    publisher: Arc<dyn EventPublisher>,
}

impl EventHook {
    /// Create a post-commit hook that publishes through the given router or publisher.
    pub fn new(publisher: Arc<dyn EventPublisher>) -> Self {
        Self { publisher }
    }
}

impl MutationHook for EventHook {
    fn on_create(&self, record: &HookRecord) {
        self.publisher.publish(match record {
            HookRecord::Agent(agent) => new_agent_created_event(agent.clone()),
            HookRecord::Model(model) => new_model_created_event(model.clone()),
            HookRecord::ModelProvider(provider) => {
                new_model_provider_created_event(provider.clone())
            }
            HookRecord::Task(task) => new_task_created_event(task.clone()),
        });
    }

    fn on_update(&self, record: &HookRecord) {
        self.publisher.publish(match record {
            HookRecord::Agent(agent) => new_agent_updated_event(agent.clone()),
            HookRecord::Model(model) => new_model_updated_event(model.clone()),
            HookRecord::ModelProvider(provider) => {
                new_model_provider_updated_event(provider.clone())
            }
            HookRecord::Task(_) => return,
        });
    }

    fn on_delete(&self, kind: EntityKind, id: Uuid) {
        if id.is_nil() {
            return;
        }
        self.publisher.publish(match kind {
            EntityKind::Agent => new_agent_deleted_event(id),
            EntityKind::Model => new_model_deleted_event(id),
            EntityKind::ModelProvider => new_model_provider_deleted_event(id),
            EntityKind::Task => new_task_deleted_event(id),
        });
    }
}
