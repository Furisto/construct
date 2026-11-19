use super::*;
use construct_types::memory::{ModelCapability, ModelProviderType};

type SupportedModel = (
    &'static str,
    i64,
    f64,
    f64,
    f64,
    f64,
    &'static [&'static str],
);
const MODELS: &[SupportedModel] = &[
    (
        "claude-opus-4-5-20251101",
        200000,
        5.0,
        25.0,
        6.25,
        0.5,
        &["image", "computer_use", "prompt_cache", "extended_thinking"],
    ),
    (
        "claude-haiku-4-5-20251001",
        200000,
        1.0,
        5.0,
        1.25,
        0.1,
        &["image", "computer_use", "prompt_cache", "extended_thinking"],
    ),
    (
        "claude-sonnet-4-5-20250929",
        200000,
        3.0,
        15.0,
        3.75,
        0.3,
        &["image", "computer_use", "prompt_cache", "extended_thinking"],
    ),
    (
        "claude-sonnet-4-20250514",
        200000,
        3.0,
        15.0,
        3.75,
        0.3,
        &["image", "computer_use", "prompt_cache", "extended_thinking"],
    ),
    (
        "claude-opus-4-20250514",
        200000,
        15.0,
        75.0,
        18.75,
        1.5,
        &["image", "computer_use", "prompt_cache", "extended_thinking"],
    ),
    (
        "claude-3-7-sonnet-20250219",
        200000,
        3.0,
        15.0,
        3.75,
        0.3,
        &["image", "computer_use", "prompt_cache", "extended_thinking"],
    ),
    (
        "claude-3-5-sonnet-20241022",
        200000,
        3.0,
        15.0,
        3.75,
        0.3,
        &["image", "computer_use", "prompt_cache"],
    ),
    (
        "claude-3-5-sonnet-20240620",
        100000,
        3.0,
        15.0,
        3.75,
        0.3,
        &["image", "computer_use", "prompt_cache"],
    ),
    (
        "claude-3-5-haiku-20241022",
        200000,
        0.8,
        4.0,
        1.0,
        0.08,
        &["prompt_cache"],
    ),
    (
        "claude-3-opus-20240229",
        200000,
        15.0,
        75.0,
        18.75,
        1.5,
        &["image", "prompt_cache"],
    ),
    (
        "claude-3-haiku-20240307",
        200000,
        0.25,
        1.25,
        0.3,
        0.03,
        &["image", "prompt_cache"],
    ),
];

impl Services {
    fn encrypted_auth(&self, id: Uuid, key: String) -> Result<Vec<u8>, ConnectError> {
        let json = serde_json::to_vec(&serde_json::json!({"apiKey": key})).map_err(internal)?;
        self.encryption
            .encrypt(&json, &construct_secret::model_provider_associated(id))
            .map_err(internal)
    }
    async fn builtin_agents(&self, provider: Uuid) -> Result<(), ConnectError> {
        let tx = self.store.begin().await.api("transaction")?;
        let agents = tx
            .list_agents(
                AgentFilter {
                    builtin: Some(true),
                    ..Default::default()
                },
                ListOptions::default(),
            )
            .await
            .api("agent")?;
        if agents.is_empty() {
            let models = tx
                .list_models(
                    ModelFilter {
                        model_provider_id: Some(provider),
                        ..Default::default()
                    },
                    ListOptions::default(),
                )
                .await
                .api("model")?;
            let configs = [
                (
                    1,
                    "edit",
                    "claude-sonnet-4-5-20250929",
                    include_str!("../../../../../backend/prompt/edit.md"),
                    "Implements code changes from plans or direct requests",
                ),
                (
                    2,
                    "quick",
                    "claude-haiku-4-5-20251001",
                    include_str!("../../../../../backend/prompt/edit.md"),
                    "Fast, targeted edits for simple code changes",
                ),
                (
                    3,
                    "plan",
                    "claude-opus-4-5-20251101",
                    include_str!("../../../../../backend/prompt/plan.md"),
                    "Analyzes requirements and creates detailed implementation plans",
                ),
            ];
            for (n, name, model_name, instructions, description) in configs {
                let model = models
                    .iter()
                    .find(|m| m.name == model_name)
                    .ok_or_else(|| ConnectError::new(Code::NotFound, "default model not found"))?;
                let time = now();
                tx.create_agent(db::Agent {
                    id: Uuid::from_u128((1 << 96) | n),
                    name: name.into(),
                    description: description.into(),
                    instructions: instructions.into(),
                    builtin: true,
                    model_id: Some(model.id),
                    create_time: time.clone(),
                    update_time: time,
                })
                .await
                .api("agent")?;
            }
        }
        tx.commit().await.api("transaction")
    }
}

#[async_trait]
impl pb::ModelProviderService for Services {
    async fn create_model_provider(
        &self,
        req: Request<pb::CreateModelProviderRequest>,
    ) -> Result<Response<pb::CreateModelProviderResponse>, ConnectError> {
        let r = req.message;
        let kind = conv::convert_model_provider_type_to_memory(r.provider_type)
            .map_err(|e| invalid(e.to_string()))?;
        if kind.as_ref() != ModelProviderType::ANTHROPIC {
            return Err(invalid("only Anthropic is supported for now"));
        }
        let key = match r.authentication {
            Some(pb::create_model_provider_request::Authentication::ApiKey(key)) => key,
            None => return Err(ConnectError::new(Code::Internal, "failed to marshal authentication config: unsupported authentication config type: <nil>")),
        };
        let id = Uuid::new_v4();
        let secret = self.encrypted_auth(id, key)?;
        let tx = self.store.begin().await.api("transaction")?;
        let time = now();
        let provider = tx
            .create_model_provider(db::ModelProvider {
                id,
                name: r.name,
                provider_type: kind,
                url: r.url.unwrap_or_default(),
                secret,
                enabled: true,
                create_time: time.clone(),
                update_time: time.clone(),
            })
            .await
            .api("model_provider")?;
        for &(name, window, input_cost, output_cost, cache_write_cost, cache_read_cost, caps) in
            MODELS
        {
            tx.create_model(db::Model {
                id: Uuid::new_v4(),
                name: name.into(),
                context_window: window,
                model_provider_id: id,
                enabled: true,
                capabilities: Some(caps.iter().map(|&c| ModelCapability::from(c)).collect()),
                input_cost,
                output_cost,
                cache_write_cost,
                cache_read_cost,
                create_time: time.clone(),
                update_time: time.clone(),
                ..Default::default()
            })
            .await
            .api("model")?;
        }
        tx.commit().await.api("transaction")?;
        self.builtin_agents(id).await?;
        Ok(Response::new(pb::CreateModelProviderResponse {
            model_provider: conv::convert_model_provider_into_proto(Some(&provider))
                .map_err(internal)?,
        }))
    }
    async fn get_model_provider(
        &self,
        req: Request<pb::GetModelProviderRequest>,
    ) -> Result<Response<pb::GetModelProviderResponse>, ConnectError> {
        let p = self
            .store
            .get_model_provider(parse_id(&req.message.id, "ID")?)
            .await
            .api("model_provider")?;
        Ok(Response::new(pb::GetModelProviderResponse {
            model_provider: conv::convert_model_provider_into_proto(Some(&p)).map_err(internal)?,
        }))
    }
    async fn list_model_providers(
        &self,
        req: Request<pb::ListModelProvidersRequest>,
    ) -> Result<Response<pb::ListModelProvidersResponse>, ConnectError> {
        let mut filter = ModelProviderFilter::default();
        if let Some(f) = req.message.filter {
            filter.enabled = f.enabled;
            filter.provider_types = f
                .provider_types
                .into_iter()
                .map(conv::convert_model_provider_type_to_memory)
                .collect::<Result<_, _>>()
                .map_err(internal)?;
        }
        let providers = self
            .store
            .list_model_providers(filter, ListOptions::default())
            .await
            .api("model_provider")?;
        let model_providers = providers
            .iter()
            .map(|p| conv::convert_model_provider_into_proto(Some(p)))
            .collect::<Result<Vec<_>, _>>()
            .map_err(internal)?
            .into_iter()
            .flatten()
            .collect();
        Ok(Response::new(pb::ListModelProvidersResponse {
            model_providers,
            ..Default::default()
        }))
    }
    async fn update_model_provider(
        &self,
        req: Request<pb::UpdateModelProviderRequest>,
    ) -> Result<Response<pb::UpdateModelProviderResponse>, ConnectError> {
        let r = req.message;
        let id = parse_id(&r.id, "ID")?;
        let tx = self.store.begin().await.api("transaction")?;
        let mut p = tx.get_model_provider(id).await.api("model_provider")?;
        if let Some(name) = r.name {
            p.name = name;
        }
        if let Some(enabled) = r.enabled {
            p.enabled = enabled;
        }
        if let Some(pb::update_model_provider_request::Authentication::ApiKey(key)) =
            r.authentication
        {
            p.secret = self.encrypted_auth(id, key)?;
        }
        p.update_time = now();
        let p = tx.update_model_provider(p).await.api("model_provider")?;
        tx.commit().await.api("transaction")?;
        Ok(Response::new(pb::UpdateModelProviderResponse {
            model_provider: conv::convert_model_provider_into_proto(Some(&p)).map_err(internal)?,
        }))
    }
    async fn delete_model_provider(
        &self,
        req: Request<pb::DeleteModelProviderRequest>,
    ) -> Result<Response<pb::DeleteModelProviderResponse>, ConnectError> {
        let id = parse_id(&req.message.id, "ID")?;
        self.store
            .delete_model_provider(id)
            .await
            .api("model_provider")?;
        Ok(Response::new(pb::DeleteModelProviderResponse {}))
    }
}
