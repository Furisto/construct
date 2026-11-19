use super::*;
use construct_types::memory::TaskPhase;

#[async_trait]
impl pb::TaskService for Services {
    async fn create_task(
        &self,
        req: Request<pb::CreateTaskRequest>,
    ) -> Result<Response<pb::CreateTaskResponse>, ConnectError> {
        let r = req.message;
        let agent_id = parse_id(&r.agent_id, "agent ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        tx.get_agent(agent_id).await.api("agent")?;
        let time = now();
        let t = tx
            .create_task(db::Task {
                id: Uuid::new_v4(),
                agent_id: Some(agent_id),
                description: r.description,
                project_directory: r.project_directory,
                desired_phase: TaskPhase::from(TaskPhase::RUNNING),
                phase: TaskPhase::from(TaskPhase::AWAITING),
                tool_uses: Some(Default::default()),
                create_time: time.clone(),
                update_time: time,
                ..Default::default()
            })
            .await
            .api("task")?;
        tx.commit().await.api("transaction")?;
        self.analytics
            .emit_task_created(&t.id.to_string(), &agent_id.to_string());
        Ok(Response::new(pb::CreateTaskResponse {
            task: Some(conv::convert_task_to_proto(&t)),
        }))
    }
    async fn get_task(
        &self,
        req: Request<pb::GetTaskRequest>,
    ) -> Result<Response<pb::GetTaskResponse>, ConnectError> {
        let t = self
            .store
            .get_task(parse_id(&req.message.id, "task ID")?)
            .await
            .api("task")?;
        Ok(Response::new(pb::GetTaskResponse {
            task: Some(conv::convert_task_to_proto(&t)),
        }))
    }
    async fn list_tasks(
        &self,
        req: Request<pb::ListTasksRequest>,
    ) -> Result<Response<pb::ListTasksResponse>, ConnectError> {
        let r = req.message;
        let mut filter = TaskFilter::default();
        if let Some(f) = r.filter {
            filter.agent_id = f
                .agent_id
                .as_deref()
                .map(|id| parse_id(id, "agent ID"))
                .transpose()?;
            filter.id_prefix = f.task_id_prefix;
            filter.has_messages = f.has_messages;
        }
        let options = ListOptions {
            limit: r
                .page_size
                // Go's ent query treats Limit(0) as no limit.
                .filter(|&size| size > 0)
                .map(|size| size as usize),
            sort_field: if r.sort_field == Some(pb::SortField::UpdatedAt as i32) {
                SortField::UpdatedAt
            } else {
                SortField::CreatedAt
            },
            sort_order: if r.sort_order == Some(pb::SortOrder::Asc as i32) {
                SortOrder::Ascending
            } else {
                SortOrder::Descending
            },
            ..Default::default()
        };
        let tasks = self.store.list_tasks(filter, options).await.api("task")?;
        let mut results = Vec::with_capacity(tasks.len());
        for t in tasks {
            let mut p = conv::convert_task_to_proto(&t);
            let count = self
                .store
                .list_messages(
                    MessageFilter {
                        task_id: Some(t.id),
                        ..Default::default()
                    },
                    ListOptions::default(),
                )
                .await
                .api("message")?
                .len();
            if let Some(status) = p.status.as_mut() {
                status.message_count = count as i64;
            }
            results.push(p);
        }
        Ok(Response::new(pb::ListTasksResponse {
            tasks: results,
            ..Default::default()
        }))
    }
    async fn update_task(
        &self,
        req: Request<pb::UpdateTaskRequest>,
    ) -> Result<Response<pb::UpdateTaskResponse>, ConnectError> {
        let r = req.message;
        let id = parse_id(&r.id, "task ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        let mut task = tx.get_task(id).await.api("task")?;
        let mut fields = vec![];
        if let Some(id) = r.agent_id {
            let id = parse_id(&id, "agent ID")?;
            tx.get_agent(id).await.api("agent")?;
            task.agent_id = Some(id);
            fields.push("agent_id".into());
        }
        task.update_time = now();
        let task = tx.update_task(task).await.api("task")?;
        tx.commit().await.api("transaction")?;
        self.analytics
            .emit_task_updated(&task.id.to_string(), &fields);
        Ok(Response::new(pb::UpdateTaskResponse {
            task: Some(conv::convert_task_to_proto(&task)),
        }))
    }
    async fn delete_task(
        &self,
        req: Request<pb::DeleteTaskRequest>,
    ) -> Result<Response<pb::DeleteTaskResponse>, ConnectError> {
        self.store
            .delete_task(parse_id(&req.message.id, "task ID")?)
            .await
            .api("task")?;
        Ok(Response::new(pb::DeleteTaskResponse {}))
    }
    async fn suspend_task(
        &self,
        req: Request<pb::SuspendTaskRequest>,
    ) -> Result<Response<pb::SuspendTaskResponse>, ConnectError> {
        let id = parse_id(&req.message.task_id, "task ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        let mut t = tx.get_task(id).await.api("task")?;
        t.phase = TaskPhase::from(TaskPhase::SUSPENDED);
        t.update_time = now();
        tx.update_task(t).await.api("task")?;
        tx.commit().await.api("transaction")?;
        self.events
            .publish(construct_event::new_internal_task_suspend_event(id));
        Ok(Response::new(pb::SuspendTaskResponse {}))
    }
}
