use super::*;
use construct_connect::Streaming;
use construct_event::{EventSubscriber, SubscribeOptions};
use futures_util::StreamExt;
use std::collections::HashSet;

impl Services {
    async fn replay(&self, task: &str, after: &str) -> Result<Vec<db::Message>, ConnectError> {
        let task_id = parse_id(task, "task ID")?;
        let created_after = if after.is_empty() {
            None
        } else {
            let message_id = parse_id(after, "message ID")?;
            Some(
                self.store
                    .get_message(message_id)
                    .await
                    .api("message")?
                    .create_time,
            )
        };
        self.store
            .list_messages(
                MessageFilter {
                    task_id: Some(task_id),
                    created_after,
                    ..Default::default()
                },
                ListOptions {
                    sort_order: SortOrder::Ascending,
                    ..Default::default()
                },
            )
            .await
            .api("message")
    }
}
#[async_trait]
impl pb::EventService for Services {
    async fn subscribe(
        &self,
        req: Request<pb::EventSubscribeRequest>,
    ) -> Result<Response<Streaming<pb::EventSubscribeResponse>>, ConnectError> {
        let r = req.message;
        // Register before replay reads, then deduplicate the replay/live overlap by message ID.
        let task_id = r.task_id.unwrap_or_default();
        let after = r.replay_after_message_id.unwrap_or_default();
        let mut live = self.events.subscribe(SubscribeOptions {
            event_types: r.event_types,
            task_id: task_id.clone(),
            ..Default::default()
        });
        let service = self.clone();
        let stream = async_stream::stream! {
            let mut replayed = HashSet::new();
            if !task_id.is_empty() {
                match service.replay(&task_id, &after).await {
                    Ok(messages) => for message in messages {
                        let id = message.id;
                        let event = construct_event::new_message_created_event(message);
                        match conv::convert_stream_event_to_proto(Some(&event)) {
                            Ok(event) => {
                                replayed.insert(id);
                                yield Ok(pb::EventSubscribeResponse { event: Some(event) });
                            },
                            Err(error) => tracing::warn!(%error, "failed to convert replay event"),
                        }
                    },
                    Err(error) => tracing::warn!(%error, "failed to replay events"),
                }
            }
            while let Some(event) = live.next().await {
                if event.r#type == "message.created" {
                    if let Some(payload) = event.payload.downcast_ref::<construct_event::MessageEventPayload>() {
                        if replayed.contains(&payload.message.id) { continue; }
                    }
                }
                match conv::convert_stream_event_to_proto(Some(&event)) {
                    Ok(event) => yield Ok(pb::EventSubscribeResponse { event: Some(event) }),
                    Err(error) => tracing::warn!(%error, "failed to convert live event"),
                }
            }
        };
        Ok(Response::new(Box::pin(stream)))
    }
}
