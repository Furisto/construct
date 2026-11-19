use super::*;

#[async_trait]
impl pb::ModelService for Services {
    async fn create_model(
        &self,
        req: Request<pb::CreateModelRequest>,
    ) -> Result<Response<pb::CreateModelResponse>, ConnectError> {
        let r = req.message;
        let provider = parse_id(&r.model_provider_id, "model provider ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        tx.get_model_provider(provider)
            .await
            .api("model_provider")?;
        let capabilities = r
            .capabilities
            .into_iter()
            .map(conv::proto_model_capability_to_memory)
            .collect::<Result<Vec<_>, _>>()
            .map_err(internal)?;
        let (input_cost, output_cost, cache_write_cost, cache_read_cost) = r
            .pricing
            .as_ref()
            .map(conv::proto_model_pricing_to_memory)
            .transpose()
            .map_err(internal)?
            .unwrap_or_default();
        let time = now();
        let model = tx
            .create_model(db::Model {
                id: Uuid::new_v4(),
                name: r.name,
                model_provider_id: provider,
                context_window: r.context_window,
                capabilities: (!capabilities.is_empty()).then_some(capabilities),
                enabled: true,
                alias: r.alias.unwrap_or_default(),
                input_cost,
                output_cost,
                cache_write_cost,
                cache_read_cost,
                create_time: time.clone(),
                update_time: time,
            })
            .await
            .api("model")?;
        tx.commit().await.api("transaction")?;
        Ok(Response::new(pb::CreateModelResponse {
            model: Some(conv::memory_model_to_proto(&model)),
        }))
    }
    async fn get_model(
        &self,
        req: Request<pb::GetModelRequest>,
    ) -> Result<Response<pb::GetModelResponse>, ConnectError> {
        let m = self
            .store
            .get_model(parse_id(&req.message.id, "ID")?)
            .await
            .api("model")?;
        Ok(Response::new(pb::GetModelResponse {
            model: Some(conv::memory_model_to_proto(&m)),
        }))
    }
    async fn list_models(
        &self,
        req: Request<pb::ListModelsRequest>,
    ) -> Result<Response<pb::ListModelsResponse>, ConnectError> {
        let mut filter = ModelFilter::default();
        if let Some(f) = req.message.filter {
            filter.names = f.names;
            filter.enabled = f.enabled;
            filter.model_provider_id = f
                .model_provider_id
                .as_deref()
                .map(|id| parse_id(id, "model provider ID"))
                .transpose()?;
        }
        let models = self
            .store
            .list_models(filter, ListOptions::default())
            .await
            .api("model")?;
        Ok(Response::new(pb::ListModelsResponse {
            models: models.iter().map(conv::memory_model_to_proto).collect(),
            ..Default::default()
        }))
    }
    async fn update_model(
        &self,
        req: Request<pb::UpdateModelRequest>,
    ) -> Result<Response<pb::UpdateModelResponse>, ConnectError> {
        let r = req.message;
        let id = parse_id(&r.id, "ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        let mut model = tx.get_model(id).await.api("model")?;
        if let Some(name) = r.name {
            model.name = name;
        }
        if !r.capabilities.is_empty() {
            model.capabilities = Some(
                r.capabilities
                    .into_iter()
                    .filter(|cap| {
                        matches!(
                            pb::ModelCapability::try_from(*cap),
                            Ok(pb::ModelCapability::Image
                                | pb::ModelCapability::ComputerUse
                                | pb::ModelCapability::PromptCache
                                | pb::ModelCapability::Thinking)
                        )
                    })
                    .filter_map(|cap| conv::proto_model_capability_to_memory(cap).ok())
                    .collect(),
            );
        }
        if let Some(pricing) = r.pricing {
            (
                model.input_cost,
                model.output_cost,
                model.cache_write_cost,
                model.cache_read_cost,
            ) = conv::proto_model_pricing_to_memory(&pricing).map_err(internal)?;
        }
        if let Some(value) = r.context_window {
            model.context_window = value;
        }
        if let Some(value) = r.enabled {
            model.enabled = value;
        }
        if let Some(value) = r.alias {
            model.alias = value;
        }
        model.update_time = now();
        let model = tx.update_model(model).await.api("model")?;
        tx.commit().await.api("transaction")?;
        Ok(Response::new(pb::UpdateModelResponse {
            model: Some(conv::memory_model_to_proto(&model)),
        }))
    }
    async fn delete_model(
        &self,
        req: Request<pb::DeleteModelRequest>,
    ) -> Result<Response<pb::DeleteModelResponse>, ConnectError> {
        self.store
            .delete_model(parse_id(&req.message.id, "ID")?)
            .await
            .api("model")?;
        Ok(Response::new(pb::DeleteModelResponse {}))
    }
}
