use super::*;

#[async_trait]
impl pb::AgentService for Services {
    async fn create_agent(
        &self,
        req: Request<pb::CreateAgentRequest>,
    ) -> Result<Response<pb::CreateAgentResponse>, ConnectError> {
        let r = req.message;
        let model_id = parse_id(&r.model_id, "model ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        let model = tx.get_model(model_id).await.api("model")?;
        if !model.enabled {
            return Err(invalid("default model is disabled"));
        }
        let time = now();
        let a = tx
            .create_agent(db::Agent {
                id: Uuid::new_v4(),
                name: r.name,
                description: r.description,
                instructions: r.instructions,
                model_id: Some(model_id),
                create_time: time.clone(),
                update_time: time,
                ..Default::default()
            })
            .await
            .api("agent")?;
        tx.commit().await.api("transaction")?;
        self.analytics
            .emit_agent_created(&a.id.to_string(), &a.name, &model.name);
        Ok(Response::new(pb::CreateAgentResponse {
            agent: Some(conv::convert_agent_to_proto(&a)),
        }))
    }
    async fn get_agent(
        &self,
        req: Request<pb::GetAgentRequest>,
    ) -> Result<Response<pb::GetAgentResponse>, ConnectError> {
        let a = self
            .store
            .get_agent(parse_id(&req.message.id, "ID")?)
            .await
            .api("agent")?;
        Ok(Response::new(pb::GetAgentResponse {
            agent: Some(conv::convert_agent_to_proto(&a)),
        }))
    }
    async fn list_agents(
        &self,
        req: Request<pb::ListAgentsRequest>,
    ) -> Result<Response<pb::ListAgentsResponse>, ConnectError> {
        let names = req.message.filter.map(|f| f.names).unwrap_or_default();
        let agents = self
            .store
            .list_agents(
                AgentFilter {
                    names,
                    ..Default::default()
                },
                ListOptions::default(),
            )
            .await
            .api("agent")?;
        Ok(Response::new(pb::ListAgentsResponse {
            agents: agents.iter().map(conv::convert_agent_to_proto).collect(),
            ..Default::default()
        }))
    }
    async fn update_agent(
        &self,
        req: Request<pb::UpdateAgentRequest>,
    ) -> Result<Response<pb::UpdateAgentResponse>, ConnectError> {
        let r = req.message;
        let id = parse_id(&r.id, "agent ID")?;
        let mut fields = vec![];
        // Repositories replace complete records; serialize the read/modify/write pair.
        let tx = self.store.begin().await.api("transaction")?;
        let mut a = tx.get_agent(id).await.api("agent")?;
        if let Some(value) = r.name {
            a.name = value;
            fields.push("name".into());
        }
        if let Some(value) = r.description {
            a.description = value;
            fields.push("description".into());
        }
        if let Some(value) = r.instructions {
            a.instructions = value;
            fields.push("instructions".into());
        }
        if let Some(value) = r.model_id {
            let id = parse_id(&value, "model ID")?;
            tx.get_model(id).await.api("model")?;
            a.model_id = Some(id);
            fields.push("model_id".into());
        }
        a.update_time = now();
        let a = tx.update_agent(a).await.api("agent")?;
        tx.commit().await.api("transaction")?;
        self.analytics
            .emit_agent_updated(&a.id.to_string(), &a.name, &fields);
        Ok(Response::new(pb::UpdateAgentResponse {
            agent: Some(conv::convert_agent_to_proto(&a)),
        }))
    }
    async fn delete_agent(
        &self,
        req: Request<pb::DeleteAgentRequest>,
    ) -> Result<Response<pb::DeleteAgentResponse>, ConnectError> {
        let id = parse_id(&req.message.id, "agent ID")?;
        let a = self.store.get_agent(id).await.api("agent")?;
        self.store.delete_agent(id).await.api("agent")?;
        self.analytics
            .emit_agent_deleted(&a.id.to_string(), &a.name);
        Ok(Response::new(pb::DeleteAgentResponse {}))
    }
}
