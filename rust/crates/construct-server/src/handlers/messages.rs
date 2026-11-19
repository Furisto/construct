use super::*;
use construct_types::memory::{MessageSource, TaskPhase};

#[async_trait]
impl pb::MessageService for Services {
    async fn create_message(
        &self,
        req: Request<pb::CreateMessageRequest>,
    ) -> Result<Response<pb::CreateMessageResponse>, ConnectError> {
        let r = req.message;
        let task_id = parse_id(&r.task_id, "task ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        let mut task = tx.get_task(task_id).await.api("task")?;
        if task.desired_phase.as_ref() == TaskPhase::SUSPENDED {
            task.desired_phase = TaskPhase::from(TaskPhase::RUNNING);
            task.update_time = now();
            tx.update_task(task).await.api("task")?;
        }
        let time = now();
        let message = tx
            .create_message(db::Message {
                id: Uuid::new_v4(),
                task_id,
                source: MessageSource::from(MessageSource::USER),
                content: conv::convert_proto_content_to_memory(&r.content),
                create_time: time.clone(),
                update_time: time,
                ..Default::default()
            })
            .await
            .api("message")?;
        tx.commit().await.api("transaction")?;
        let message = conv::convert_memory_message_to_proto(Some(&message)).map_err(internal)?;
        self.events
            .publish(construct_event::new_internal_task_trigger_event(task_id));
        Ok(Response::new(pb::CreateMessageResponse {
            message: Some(message),
        }))
    }
    async fn get_message(
        &self,
        req: Request<pb::GetMessageRequest>,
    ) -> Result<Response<pb::GetMessageResponse>, ConnectError> {
        let m = self
            .store
            .get_message(parse_id(&req.message.id, "ID")?)
            .await
            .api("message")?;
        Ok(Response::new(pb::GetMessageResponse {
            message: Some(conv::convert_memory_message_to_proto(Some(&m)).map_err(internal)?),
        }))
    }
    async fn list_messages(
        &self,
        req: Request<pb::ListMessagesRequest>,
    ) -> Result<Response<pb::ListMessagesResponse>, ConnectError> {
        let mut filter = MessageFilter::default();
        if let Some(f) = req.message.filter {
            filter.task_id = f
                .task_ids
                .as_deref()
                .map(|id| parse_id(id, "task ID"))
                .transpose()?;
            filter.agent_id = f
                .agent_ids
                .as_deref()
                .map(|id| parse_id(id, "agent ID"))
                .transpose()?;
            if let Some(role) = f.roles {
                filter.sources = vec![match pb::MessageRole::try_from(role) {
                    Ok(pb::MessageRole::User) => MessageSource::from(MessageSource::USER),
                    Ok(pb::MessageRole::Assistant) => MessageSource::from(MessageSource::ASSISTANT),
                    _ => MessageSource::default(),
                }];
            }
        }
        let messages = self
            .store
            .list_messages(
                filter,
                // Go's ListMessages applies no ORDER BY, so SQLite returns insertion order.
                ListOptions {
                    sort_order: SortOrder::Ascending,
                    ..Default::default()
                },
            )
            .await
            .api("message")?;
        let messages = messages
            .iter()
            .map(|m| conv::convert_memory_message_to_proto(Some(m)))
            .collect::<Result<_, _>>()
            .map_err(internal)?;
        Ok(Response::new(pb::ListMessagesResponse {
            messages,
            ..Default::default()
        }))
    }
    async fn update_message(
        &self,
        req: Request<pb::UpdateMessageRequest>,
    ) -> Result<Response<pb::UpdateMessageResponse>, ConnectError> {
        let r = req.message;
        let id = parse_id(&r.id, "ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        let mut message = tx.get_message(id).await.api("message")?;
        message.content = conv::convert_proto_content_to_memory(&r.content);
        message.update_time = now();
        let m = tx.update_message(message).await.api("message")?;
        tx.commit().await.api("transaction")?;
        Ok(Response::new(pb::UpdateMessageResponse {
            message: Some(conv::convert_memory_message_to_proto(Some(&m)).map_err(internal)?),
        }))
    }
    async fn delete_message(
        &self,
        req: Request<pb::DeleteMessageRequest>,
    ) -> Result<Response<pb::DeleteMessageResponse>, ConnectError> {
        self.store
            .delete_message(parse_id(&req.message.id, "ID")?)
            .await
            .api("message")?;
        Ok(Response::new(pb::DeleteMessageResponse {}))
    }
}
