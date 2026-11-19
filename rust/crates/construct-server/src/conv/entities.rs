use super::{convert_time_to_timestamp as ts, convert_uuid_to_string, ConversionError};
use construct_proto::{construct::v1 as pb, google::r#type::Decimal};
use construct_types::{entity as db, memory as mem, provider};

/// Convert a stored agent to its public API representation.
pub fn convert_agent_to_proto(a: &db::Agent) -> pb::Agent {
    pb::Agent {
        metadata: Some(convert_agent_metadata_to_proto(a)),
        spec: Some(convert_agent_spec_to_proto(a)),
    }
}
/// Convert an agent's immutable metadata.
pub fn convert_agent_metadata_to_proto(a: &db::Agent) -> pb::AgentMetadata {
    pb::AgentMetadata {
        id: a.id.to_string(),
        created_at: Some(ts(&a.create_time)),
        updated_at: Some(ts(&a.update_time)),
    }
}
/// Convert an agent's public specification.
pub fn convert_agent_spec_to_proto(a: &db::Agent) -> pb::AgentSpec {
    pb::AgentSpec {
        name: a.name.clone(),
        description: a.description.clone(),
        instructions: a.instructions.clone(),
        model_id: convert_uuid_to_string(a.model_id.unwrap_or_default()),
    }
}
/// Convert a stored task, including usage and phase.
pub fn convert_task_to_proto(t: &db::Task) -> pb::Task {
    pb::Task {
        metadata: Some(convert_task_metadata_to_proto(t)),
        spec: Some(convert_task_spec_to_proto(t)),
        status: Some(convert_task_status_to_proto(t)),
    }
}
/// Convert a task's immutable metadata.
pub fn convert_task_metadata_to_proto(t: &db::Task) -> pb::TaskMetadata {
    pb::TaskMetadata {
        id: t.id.to_string(),
        created_at: Some(ts(&t.create_time)),
        updated_at: Some(ts(&t.update_time)),
    }
}
/// Convert a task's public specification, preserving the Go agent pointer presence.
pub fn convert_task_spec_to_proto(t: &db::Task) -> pb::TaskSpec {
    pb::TaskSpec {
        agent_id: Some(t.agent_id.unwrap_or_default().to_string()),
        workspace: t.project_directory.clone(),
        desired_phase: convert_task_phase_to_proto(&t.desired_phase) as i32,
        description: t.description.clone(),
    }
}
/// Convert a task's observed phase and aggregate usage.
pub fn convert_task_status_to_proto(t: &db::Task) -> pb::TaskStatus {
    pb::TaskStatus {
        phase: convert_task_phase_to_proto(&t.phase) as i32,
        usage: Some(pb::TaskUsage {
            input_tokens: t.input_tokens,
            output_tokens: t.output_tokens,
            cache_write_tokens: t.cache_write_tokens,
            cache_read_tokens: t.cache_read_tokens,
            cost: t.cost,
            tool_uses: t
                .tool_uses
                .clone()
                .unwrap_or_default()
                .into_iter()
                .collect(),
        }),
        ..Default::default()
    }
}
/// Convert known lifecycle phases; unknown values become unspecified.
pub fn convert_task_phase_to_proto(phase: &mem::TaskPhase) -> pb::TaskPhase {
    match phase.as_ref() {
        "awaiting" => pb::TaskPhase::Awaiting,
        "running" => pb::TaskPhase::Running,
        "suspended" => pb::TaskPhase::Suspended,
        _ => pb::TaskPhase::Unspecified,
    }
}
/// Convert a stored model, using Go adapter's six-decimal price formatting.
pub fn memory_model_to_proto(m: &db::Model) -> pb::Model {
    let decimal = |v: f64| Some(super::float64_to_proto_decimal(v));
    pb::Model {
        metadata: Some(pb::ModelMetadata {
            id: m.id.to_string(),
            created_at: Some(ts(&m.create_time)),
            updated_at: Some(ts(&m.update_time)),
            model_provider_id: m.model_provider_id.to_string(),
        }),
        spec: Some(pb::ModelSpec {
            name: m.name.clone(),
            capabilities: m
                .capabilities
                .iter()
                .flatten()
                .map(|c| memory_model_capability_to_proto(c) as i32)
                .collect(),
            pricing: Some(pb::ModelPricing {
                input_cost: decimal(m.input_cost),
                output_cost: decimal(m.output_cost),
                cache_write_cost: decimal(m.cache_write_cost),
                cache_read_cost: decimal(m.cache_read_cost),
            }),
            context_window: m.context_window,
            enabled: m.enabled,
            alias: m.alias.clone(),
        }),
    }
}
/// Map a stored model capability, with unknown values becoming unspecified.
pub fn memory_model_capability_to_proto(cap: &mem::ModelCapability) -> pb::ModelCapability {
    match cap.as_ref() {
        "image" => pb::ModelCapability::Image,
        "computer_use" => pb::ModelCapability::ComputerUse,
        "prompt_cache" => pb::ModelCapability::PromptCache,
        "extended_thinking" => pb::ModelCapability::Thinking,
        "audio" => pb::ModelCapability::Audio,
        _ => pb::ModelCapability::Unspecified,
    }
}
/// Decode a wire capability, rejecting unknown and unspecified values.
pub fn proto_model_capability_to_memory(cap: i32) -> Result<mem::ModelCapability, ConversionError> {
    let value = match pb::ModelCapability::try_from(cap) {
        Ok(pb::ModelCapability::Image) => "image",
        Ok(pb::ModelCapability::ComputerUse) => "computer_use",
        Ok(pb::ModelCapability::PromptCache) => "prompt_cache",
        Ok(pb::ModelCapability::Thinking) => "extended_thinking",
        Ok(pb::ModelCapability::Audio) => "audio",
        _ => {
            let name = pb::ModelCapability::try_from(cap)
                .map(|c| c.as_str_name().to_owned())
                .unwrap_or_else(|_| cap.to_string());
            return Err(ConversionError(format!("unknown model capability: {name}")));
        }
    };
    Ok(value.into())
}
/// Convert provider capability declarations to stored model capabilities.
pub fn llm_model_capabilities_to_memory(
    caps: &[provider::Capability],
) -> Result<Vec<mem::ModelCapability>, ConversionError> {
    caps.iter()
        .map(|cap| {
            let value = mem::ModelCapability::from(cap.as_ref());
            if memory_model_capability_to_proto(&value) == pb::ModelCapability::Unspecified {
                Err(ConversionError(format!("unknown model capability: {cap}")))
            } else {
                Ok(value)
            }
        })
        .collect()
}
/// Decode API decimal model prices in input/output/cache-write/cache-read order.
pub fn proto_model_pricing_to_memory(
    pricing: &pb::ModelPricing,
) -> Result<(f64, f64, f64, f64), ConversionError> {
    fn number(value: Option<&Decimal>) -> Result<f64, ConversionError> {
        let value = value.ok_or_else(|| ConversionError("decimal is nil".into()))?;
        value
            .value
            .parse::<f64>()
            .ok()
            .filter(|v| v.is_finite())
            .ok_or_else(|| ConversionError(format!("invalid decimal: {}", value.value)))
    }
    Ok((
        number(pricing.input_cost.as_ref())?,
        number(pricing.output_cost.as_ref())?,
        number(pricing.cache_write_cost.as_ref())?,
        number(pricing.cache_read_cost.as_ref())?,
    ))
}
/// Convert a stored provider without exposing URL or encrypted secret bytes.
pub fn convert_model_provider_into_proto(
    mp: Option<&db::ModelProvider>,
) -> Result<Option<pb::ModelProvider>, ConversionError> {
    mp.map(|mp| {
        Ok(pb::ModelProvider {
            metadata: Some(pb::ModelProviderMetadata {
                id: mp.id.to_string(),
                created_at: Some(ts(&mp.create_time)),
                updated_at: Some(ts(&mp.update_time)),
                provider_type: convert_model_provider_type_to_proto(&mp.provider_type)? as i32,
            }),
            spec: Some(pb::ModelProviderSpec {
                name: mp.name.clone(),
                enabled: mp.enabled,
            }),
        })
    })
    .transpose()
}
/// Map supported stored provider types to their wire enum.
pub fn convert_model_provider_type_to_proto(
    kind: &mem::ModelProviderType,
) -> Result<pb::ModelProviderType, ConversionError> {
    match kind.as_ref() {
        "anthropic" => Ok(pb::ModelProviderType::Anthropic),
        "openai" => Ok(pb::ModelProviderType::Openai),
        "gemini" => Ok(pb::ModelProviderType::Gemini),
        "xai" => Ok(pb::ModelProviderType::Xai),
        _ => Err(ConversionError(format!(
            "unsupported provider type: {kind}"
        ))),
    }
}
/// Map supported wire provider types to stored values.
pub fn convert_model_provider_type_to_memory(
    kind: i32,
) -> Result<mem::ModelProviderType, ConversionError> {
    match pb::ModelProviderType::try_from(kind) {
        Ok(pb::ModelProviderType::Anthropic) => Ok("anthropic".into()),
        Ok(pb::ModelProviderType::Openai) => Ok("openai".into()),
        Ok(pb::ModelProviderType::Gemini) => Ok("gemini".into()),
        Ok(pb::ModelProviderType::Xai) => Ok("xai".into()),
        _ => {
            let name = pb::ModelProviderType::try_from(kind)
                .map(|k| k.as_str_name().to_owned())
                .unwrap_or_else(|_| kind.to_string());
            Err(ConversionError(format!(
                "unsupported provider type: {name}"
            )))
        }
    }
}
