use chrono::{Duration as ChronoDuration, Utc};
#[cfg(unix)]
use construct_connect::serve_unix;
use construct_connect::{serve_tcp, Channel, ClientConfig, Code, ConnectError, Endpoint, Request};
use construct_core::store::*;
use construct_event::{EventHook, EventPublisher, EventRouter, EventSubscriber, SubscribeOptions};
use construct_proto::{construct::v1 as pb, google::r#type::Decimal};
use construct_secret::{Encryption, Keyset};
use construct_server::{api_router, Analytics, HandlerOptions, StaticRuntime};
use construct_skill::{SkillManager, UserDirs};
use construct_store::SqliteStore;
use construct_types::{entity as db, memory as mem, Timestamp};
use futures_util::StreamExt;
use std::{
    sync::{Arc, Mutex},
    time::Duration,
};
use tempfile::TempDir;
#[cfg(unix)]
use tokio::net::UnixListener;
use tokio::{net::TcpListener, task::JoinHandle};
use uuid::Uuid;

#[derive(Default)]
struct AnalyticsLog(Mutex<Vec<Vec<String>>>);
impl Analytics for AnalyticsLog {
    fn emit_agent_created(&self, id: &str, name: &str, model: &str) {
        self.0.lock().unwrap().push(vec![
            "agent_created".into(),
            id.into(),
            name.into(),
            model.into(),
        ]);
    }
    fn emit_agent_updated(&self, id: &str, name: &str, fields: &[String]) {
        let mut e = vec!["agent_updated".into(), id.into(), name.into()];
        e.extend_from_slice(fields);
        self.0.lock().unwrap().push(e);
    }
    fn emit_agent_deleted(&self, id: &str, name: &str) {
        self.0
            .lock()
            .unwrap()
            .push(vec!["agent_deleted".into(), id.into(), name.into()]);
    }
    fn emit_task_created(&self, id: &str, agent: &str) {
        self.0
            .lock()
            .unwrap()
            .push(vec!["task_created".into(), id.into(), agent.into()]);
    }
    fn emit_task_updated(&self, id: &str, fields: &[String]) {
        let mut e = vec!["task_updated".into(), id.into()];
        e.extend_from_slice(fields);
        self.0.lock().unwrap().push(e);
    }
}
struct Fixture {
    local_admin: Channel,
    tcp: Channel,
    store: Arc<SqliteStore>,
    events: Arc<EventRouter>,
    encryption: Arc<Encryption>,
    analytics: Arc<AnalyticsLog>,
    tasks: Vec<JoinHandle<std::io::Result<()>>>,
    _dir: TempDir,
}
impl Drop for Fixture {
    fn drop(&mut self) {
        for task in &self.tasks {
            task.abort();
        }
    }
}
impl Fixture {
    async fn new() -> Self {
        let dir = tempfile::tempdir().unwrap();
        let events = Arc::new(EventRouter::default());
        let store = Arc::new(
            SqliteStore::open_with_hooks(
                dir.path().join("db.sqlite"),
                vec![Arc::new(EventHook::new(events.clone()))],
            )
            .await
            .unwrap(),
        );
        let encryption = Arc::new(Encryption::new(Keyset::generate()).unwrap());
        let runtime = Arc::new(StaticRuntime::new(
            store.clone(),
            encryption.clone(),
            events.clone(),
        ));
        let skills = Arc::new(SkillManager::new(UserDirs {
            home: None,
            config_dir: dir.path().join("config"),
        }));
        let analytics = Arc::new(AnalyticsLog::default());
        let mut opts = HandlerOptions::new(runtime, skills);
        opts.analytics = analytics.clone();
        let router = api_router(opts);
        #[cfg(unix)]
        let (admin_endpoint, admin_task) = {
            let socket = dir.path().join("api.sock");
            let listener = UnixListener::bind(&socket).unwrap();
            let endpoint = Endpoint::Unix {
                path: socket,
                prefix: String::new(),
            };
            let task = tokio::spawn(serve_unix(listener, router.clone(), std::future::pending()));
            (endpoint, task)
        };
        #[cfg(not(unix))]
        let (admin_endpoint, admin_task) = {
            // Only this fixture router simulates Unix transport. The regular TCP router still enforces bearer auth.
            let router = router.clone().layer(construct_connect::axum::Extension(
                construct_connect::Transport::Unix,
            ));
            let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
            let endpoint = Endpoint::Http(format!("http://{}", listener.local_addr().unwrap()));
            let task = tokio::spawn(serve_tcp(listener, router, std::future::pending()));
            (endpoint, task)
        };
        let tcp_listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = tcp_listener.local_addr().unwrap();
        let tasks = vec![
            admin_task,
            tokio::spawn(serve_tcp(tcp_listener, router, std::future::pending())),
        ];
        let channel = |endpoint| {
            let mut cfg = ClientConfig::new(endpoint);
            cfg.timeout = Some(Duration::from_secs(5));
            Channel::new(cfg).unwrap()
        };
        Self {
            local_admin: channel(admin_endpoint),
            tcp: channel(Endpoint::Http(format!("http://{address}"))),
            store,
            events,
            encryption,
            analytics,
            tasks,
            _dir: dir,
        }
    }
    async fn seed(&self) -> (db::ModelProvider, db::Model, db::Agent, db::Task) {
        let t = Timestamp(Utc::now());
        let p = self
            .store
            .create_model_provider(db::ModelProvider {
                id: Uuid::new_v4(),
                name: "seed-provider".into(),
                provider_type: mem::ModelProviderType::from("anthropic"),
                secret: vec![1],
                enabled: true,
                create_time: t.clone(),
                update_time: t.clone(),
                ..Default::default()
            })
            .await
            .unwrap();
        let m = self
            .store
            .create_model(db::Model {
                id: Uuid::new_v4(),
                name: "seed-model".into(),
                model_provider_id: p.id,
                enabled: true,
                context_window: 200000,
                create_time: t.clone(),
                update_time: t.clone(),
                ..Default::default()
            })
            .await
            .unwrap();
        let a = self
            .store
            .create_agent(db::Agent {
                id: Uuid::new_v4(),
                name: "seed-agent".into(),
                model_id: Some(m.id),
                create_time: t.clone(),
                update_time: t.clone(),
                ..Default::default()
            })
            .await
            .unwrap();
        let task = self
            .store
            .create_task(db::Task {
                id: Uuid::new_v4(),
                agent_id: Some(a.id),
                desired_phase: mem::TaskPhase::from("running"),
                phase: mem::TaskPhase::from("awaiting"),
                create_time: t.clone(),
                update_time: t,
                ..Default::default()
            })
            .await
            .unwrap();
        (p, m, a, task)
    }
    async fn message(
        &self,
        task: Uuid,
        agent: Option<Uuid>,
        source: &str,
        delta: i64,
    ) -> db::Message {
        let time = Timestamp(Utc::now() + ChronoDuration::seconds(delta));
        let model_id = match agent {
            Some(id) => self.store.get_agent(id).await.unwrap().model_id,
            None => None,
        };
        self.store
            .create_message(db::Message {
                id: Uuid::new_v4(),
                task_id: task,
                agent_id: agent,
                model_id,
                source: mem::MessageSource::from(source),
                content: Some(mem::MessageContent {
                    blocks: Some(vec![mem::MessageBlock {
                        kind: mem::MessageBlockKind::from("text"),
                        payload: "hello".into(),
                    }]),
                }),
                create_time: time.clone(),
                update_time: time,
                ..Default::default()
            })
            .await
            .unwrap()
    }
}
fn req<T>(value: T) -> Request<T> {
    Request::new(value)
}
fn text(value: &str) -> Vec<pb::MessagePart> {
    vec![pb::MessagePart {
        data: Some(pb::message_part::Data::Text(pb::message_part::Text {
            content: value.into(),
        })),
    }]
}
fn error<T: std::fmt::Debug>(result: Result<T, ConnectError>, code: Code, message: &str) {
    let e = result.unwrap_err();
    assert_eq!(e.code, code);
    assert_eq!(e.message, message);
}
fn event_message(response: pb::EventSubscribeResponse) -> pb::Message {
    let event = response.event.unwrap();
    assert_eq!(event.r#type, "message.created");
    match event.payload.unwrap() {
        pb::event::Payload::Message(m) => m.message.unwrap(),
        other => panic!("unexpected {other:?}"),
    }
}
macro_rules! id_errors {
    ($client:expr, $method:ident, $request:ident, $field:literal, $entity:literal) => {{
        for (id, code, message) in [
            (
                "invalid-id-value".to_owned(),
                Code::InvalidArgument,
                format!("invalid {} format: invalid UUID length: 16", $field),
            ),
            (
                Uuid::new_v4().to_string(),
                Code::NotFound,
                format!("{} not found", $entity),
            ),
        ] {
            #[allow(clippy::needless_update)]
            let request = pb::$request {
                id,
                ..Default::default()
            };
            error($client.$method(req(request)).await, code, &message);
        }
    }};
}

#[tokio::test]
async fn auth_go_cases_and_admin_guards() {
    let f = Fixture::new().await;
    let c = pb::AuthServiceClient::new(f.local_admin.clone());
    error(
        c.create_token(req(pb::CreateTokenRequest::default())).await,
        Code::InvalidArgument,
        "name is required",
    );
    error(
        c.create_setup_code(req(pb::CreateSetupCodeRequest::default()))
            .await,
        Code::InvalidArgument,
        "token_name is required",
    );
    assert!(c
        .list_tokens(req(pb::ListTokensRequest::default()))
        .await
        .unwrap()
        .message
        .tokens
        .is_empty());
    for (name, description) in [
        ("token-1", None),
        ("token-2", Some("description".into())),
        ("prod-token", None),
    ] {
        let token = c
            .create_token(req(pb::CreateTokenRequest {
                name: name.into(),
                description,
                ..Default::default()
            }))
            .await
            .unwrap()
            .message;
        assert!(token.token.starts_with("ct_"));
        assert!(token.expires_at.is_some());
    }
    error(
        c.create_token(req(pb::CreateTokenRequest {
            name: "token-1".into(),
            ..Default::default()
        }))
        .await,
        Code::AlreadyExists,
        "token with name \"token-1\" already exists",
    );
    let all = c
        .list_tokens(req(pb::ListTokensRequest::default()))
        .await
        .unwrap()
        .message
        .tokens;
    assert_eq!(
        all.iter().map(|t| t.name.as_str()).collect::<Vec<_>>(),
        ["prod-token", "token-2", "token-1"]
    );
    assert_eq!(all[1].description.as_deref(), Some("description"));
    assert_eq!(
        c.list_tokens(req(pb::ListTokensRequest {
            name_prefix: "prod-".into(),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .tokens
        .len(),
        1
    );
    c.create_token(req(pb::CreateTokenRequest {
        name: "expired".into(),
        expires_in: Some(pbjson_types::Duration {
            seconds: -1,
            nanos: 0,
        }),
        ..Default::default()
    }))
    .await
    .unwrap();
    assert_eq!(
        c.list_tokens(req(pb::ListTokensRequest::default()))
            .await
            .unwrap()
            .message
            .tokens
            .len(),
        3
    );
    let expired = c
        .list_tokens(req(pb::ListTokensRequest {
            include_expired: true,
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .tokens;
    assert_eq!(expired.len(), 4);
    assert!(!expired[0].is_active);
    error(
        c.revoke_token(req(pb::RevokeTokenRequest {
            id: "invalid-id".into(),
        }))
        .await,
        Code::InvalidArgument,
        "invalid ID format: invalid UUID length: 10",
    );
    error(
        c.revoke_token(req(pb::RevokeTokenRequest {
            id: Uuid::new_v4().to_string(),
        }))
        .await,
        Code::NotFound,
        "token not found",
    );
    c.revoke_token(req(pb::RevokeTokenRequest {
        id: all[0].id.clone(),
    }))
    .await
    .unwrap();
    for (setup_code, code, message) in [
        ("", Code::InvalidArgument, "setup_code is required"),
        (
            "invalid",
            Code::PermissionDenied,
            "invalid or expired setup code",
        ),
    ] {
        error(
            c.exchange_setup_code(req(pb::ExchangeSetupCodeRequest {
                setup_code: setup_code.into(),
            }))
            .await,
            code,
            message,
        );
    }
    let code = c
        .create_setup_code(req(pb::CreateSetupCodeRequest {
            token_name: "token-1".into(),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .setup_code;
    error(
        c.exchange_setup_code(req(pb::ExchangeSetupCodeRequest {
            setup_code: code.clone(),
        }))
        .await,
        Code::AlreadyExists,
        "token with name \"token-1\" already exists",
    );
    error(
        c.exchange_setup_code(req(pb::ExchangeSetupCodeRequest { setup_code: code }))
            .await,
        Code::PermissionDenied,
        "invalid or expired setup code",
    );
    let max = 365 * 24 * 3600 + 1;
    error(
        c.create_token(req(pb::CreateTokenRequest {
            name: "long".into(),
            expires_in: Some(pbjson_types::Duration {
                seconds: max,
                nanos: 0,
            }),
            ..Default::default()
        }))
        .await,
        Code::InvalidArgument,
        "expires_in exceeds maximum of 8760h0m0s",
    );
    error(
        c.create_setup_code(req(pb::CreateSetupCodeRequest {
            token_name: "long".into(),
            expires_in: Some(pbjson_types::Duration {
                seconds: 72 * 3600 + 1,
                nanos: 0,
            }),
            ..Default::default()
        }))
        .await,
        Code::InvalidArgument,
        "expires_in exceeds maximum of 72h0m0s",
    );
    error(
        c.create_setup_code(req(pb::CreateSetupCodeRequest {
            token_name: "long".into(),
            token_expires_in: Some(pbjson_types::Duration {
                seconds: max,
                nanos: 0,
            }),
            ..Default::default()
        }))
        .await,
        Code::InvalidArgument,
        "token_expires_in exceeds maximum of 8760h0m0s",
    );
}

#[tokio::test]
async fn providers_go_cases_encryption_and_builtin_agents() {
    let f = Fixture::new().await;
    let c = pb::ModelProviderServiceClient::new(f.local_admin.clone());
    assert!(c
        .list_model_providers(req(pb::ListModelProvidersRequest::default()))
        .await
        .unwrap()
        .message
        .model_providers
        .is_empty());
    error(
        c.create_model_provider(req(pb::CreateModelProviderRequest::default()))
            .await,
        Code::InvalidArgument,
        "unsupported provider type: MODEL_PROVIDER_TYPE_UNSPECIFIED",
    );
    error(
        c.create_model_provider(req(pb::CreateModelProviderRequest {
            provider_type: pb::ModelProviderType::Openai as i32,
            ..Default::default()
        }))
        .await,
        Code::InvalidArgument,
        "only Anthropic is supported for now",
    );
    assert_eq!(
        c.create_model_provider(req(pb::CreateModelProviderRequest {
            provider_type: pb::ModelProviderType::Anthropic as i32,
            ..Default::default()
        }))
        .await
        .unwrap_err()
        .code,
        Code::Internal
    );
    id_errors!(
        c,
        get_model_provider,
        GetModelProviderRequest,
        "ID",
        "model_provider"
    );
    id_errors!(
        c,
        update_model_provider,
        UpdateModelProviderRequest,
        "ID",
        "model_provider"
    );
    id_errors!(
        c,
        delete_model_provider,
        DeleteModelProviderRequest,
        "ID",
        "model_provider"
    );
    let mut ids = vec![];
    for name in ["anthropic", "second"] {
        let p = c
            .create_model_provider(req(pb::CreateModelProviderRequest {
                name: name.into(),
                provider_type: pb::ModelProviderType::Anthropic as i32,
                url: Some("https://api.anthropic.com".into()),
                authentication: Some(pb::create_model_provider_request::Authentication::ApiKey(
                    "sensitive".into(),
                )),
            }))
            .await
            .unwrap()
            .message
            .model_provider
            .unwrap();
        let id = Uuid::parse_str(&p.metadata.unwrap().id).unwrap();
        ids.push(id);
        let stored = f.store.get_model_provider(id).await.unwrap();
        assert!(!stored.secret.windows(9).any(|s| s == b"sensitive"));
        let clear = f
            .encryption
            .decrypt(
                &stored.secret,
                &construct_secret::model_provider_associated(id),
            )
            .unwrap();
        assert_eq!(
            serde_json::from_slice::<serde_json::Value>(&clear).unwrap(),
            serde_json::json!({"apiKey":"sensitive"})
        );
        assert!(f
            .encryption
            .decrypt(&stored.secret, b"wrong associated data")
            .is_err());
        let models = f
            .store
            .list_models(
                ModelFilter {
                    model_provider_id: Some(id),
                    ..Default::default()
                },
                ListOptions::default(),
            )
            .await
            .unwrap();
        assert_eq!(models.len(), 11);
        assert!(models.iter().all(|m| m.enabled));
    }
    let builtin = f
        .store
        .list_agents(
            AgentFilter {
                builtin: Some(true),
                ..Default::default()
            },
            ListOptions::default(),
        )
        .await
        .unwrap();
    assert_eq!(builtin.len(), 3);
    for a in builtin {
        let (n, model_name) = match a.name.as_str() {
            "edit" => (1, "claude-sonnet-4-5-20250929"),
            "quick" => (2, "claude-haiku-4-5-20251001"),
            "plan" => (3, "claude-opus-4-5-20251101"),
            _ => panic!(),
        };
        assert_eq!(a.id, Uuid::from_u128((1 << 96) | n));
        assert!(!a.instructions.is_empty());
        assert_eq!(
            f.store.get_model(a.model_id.unwrap()).await.unwrap().name,
            model_name
        );
    }
    let id = ids[1].to_string();
    let p = c
        .update_model_provider(req(pb::UpdateModelProviderRequest {
            id: id.clone(),
            name: Some("renamed".into()),
            enabled: Some(false),
            authentication: Some(pb::update_model_provider_request::Authentication::ApiKey(
                "updated".into(),
            )),
        }))
        .await
        .unwrap()
        .message
        .model_provider
        .unwrap();
    assert!(!p.spec.unwrap().enabled);
    let stored = f.store.get_model_provider(ids[1]).await.unwrap();
    assert_eq!(
        f.encryption
            .decrypt(
                &stored.secret,
                &construct_secret::model_provider_associated(ids[1])
            )
            .unwrap(),
        b"{\"apiKey\":\"updated\"}"
    );
    let p = c
        .get_model_provider(req(pb::GetModelProviderRequest { id: id.clone() }))
        .await
        .unwrap()
        .message
        .model_provider
        .unwrap();
    assert_eq!(p.spec.unwrap().name, "renamed");
    for (enabled, count) in [(None, 2), (Some(true), 1), (Some(false), 1)] {
        assert_eq!(
            c.list_model_providers(req(pb::ListModelProvidersRequest {
                filter: Some(pb::list_model_providers_request::Filter {
                    enabled,
                    provider_types: vec![pb::ModelProviderType::Anthropic as i32]
                }),
                ..Default::default()
            }))
            .await
            .unwrap()
            .message
            .model_providers
            .len(),
            count
        );
    }
    let unsupported_filter = c
        .list_model_providers(req(pb::ListModelProvidersRequest {
            filter: Some(pb::list_model_providers_request::Filter {
                enabled: None,
                provider_types: vec![0],
            }),
            ..Default::default()
        }))
        .await
        .unwrap_err();
    assert_eq!(unsupported_filter.code, Code::Internal);
    let models = f
        .store
        .list_models(
            ModelFilter {
                model_provider_id: Some(ids[1]),
                ..Default::default()
            },
            ListOptions::default(),
        )
        .await
        .unwrap();
    for m in models {
        f.store.delete_model(m.id).await.unwrap();
    }
    c.delete_model_provider(req(pb::DeleteModelProviderRequest { id }))
        .await
        .unwrap();
}

#[tokio::test]
async fn models_go_cases_crud_filters_and_pricing() {
    let f = Fixture::new().await;
    let c = pb::ModelServiceClient::new(f.local_admin.clone());
    assert!(c
        .list_models(req(pb::ListModelsRequest::default()))
        .await
        .unwrap()
        .message
        .models
        .is_empty());
    id_errors!(c, get_model, GetModelRequest, "ID", "model");
    id_errors!(c, update_model, UpdateModelRequest, "ID", "model");
    id_errors!(c, delete_model, DeleteModelRequest, "ID", "model");
    for (id, code, message) in [
        (
            "invalid-id-value".into(),
            Code::InvalidArgument,
            "invalid model provider ID format: invalid UUID length: 16",
        ),
        (
            Uuid::new_v4().to_string(),
            Code::NotFound,
            "model_provider not found",
        ),
    ] {
        error(
            c.create_model(req(pb::CreateModelRequest {
                name: "model".into(),
                model_provider_id: id,
                ..Default::default()
            }))
            .await,
            code,
            message,
        );
    }
    error(
        c.list_models(req(pb::ListModelsRequest {
            filter: Some(pb::list_models_request::Filter {
                model_provider_id: Some("invalid-id-value".into()),
                ..Default::default()
            }),
            ..Default::default()
        }))
        .await,
        Code::InvalidArgument,
        "invalid model provider ID format: invalid UUID length: 16",
    );
    let (p, _, _, _) = f.seed().await;
    let pricing = pb::ModelPricing {
        input_cost: Some(Decimal {
            value: "1.25".into(),
        }),
        output_cost: Some(Decimal {
            value: "2.5".into(),
        }),
        cache_write_cost: Some(Decimal {
            value: "0.75".into(),
        }),
        cache_read_cost: Some(Decimal {
            value: "0.25".into(),
        }),
    };
    let m = c
        .create_model(req(pb::CreateModelRequest {
            name: "new-model".into(),
            model_provider_id: p.id.to_string(),
            capabilities: vec![
                pb::ModelCapability::Image as i32,
                pb::ModelCapability::Thinking as i32,
            ],
            pricing: Some(pricing.clone()),
            context_window: 50000,
            alias: Some("alias".into()),
        }))
        .await
        .unwrap()
        .message
        .model
        .unwrap();
    let id = m.metadata.unwrap().id;
    let spec = m.spec.unwrap();
    assert!(spec.enabled);
    assert_eq!(spec.alias, "alias");
    assert_eq!(spec.context_window, 50000);
    assert_eq!(spec.capabilities.len(), 2);
    assert_eq!(
        f.store
            .get_model(Uuid::parse_str(&id).unwrap())
            .await
            .unwrap()
            .cache_write_cost,
        0.75
    );
    let m = c
        .update_model(req(pb::UpdateModelRequest {
            id: id.clone(),
            name: Some("updated".into()),
            context_window: Some(100000),
            alias: Some("new-alias".into()),
            capabilities: vec![pb::ModelCapability::PromptCache as i32],
            pricing: Some(pricing),
            enabled: Some(false),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .model
        .unwrap();
    let spec = m.spec.unwrap();
    assert_eq!(spec.name, "updated");
    assert_eq!(spec.context_window, 100000);
    assert_eq!(spec.capabilities, [pb::ModelCapability::PromptCache as i32]);
    assert!(!spec.enabled);
    let unchanged = c
        .update_model(req(pb::UpdateModelRequest {
            id: id.clone(),
            model_provider_id: Some(Uuid::new_v4().to_string()),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .model
        .unwrap();
    assert_eq!(
        unchanged.metadata.unwrap().model_provider_id,
        p.id.to_string()
    );
    assert_eq!(
        c.get_model(req(pb::GetModelRequest { id: id.clone() }))
            .await
            .unwrap()
            .message
            .model
            .unwrap()
            .spec
            .unwrap()
            .alias,
        "new-alias"
    );
    for (filter, count) in [
        (None, 2),
        (
            Some(pb::list_models_request::Filter {
                names: vec!["updated".into()],
                ..Default::default()
            }),
            1,
        ),
        (
            Some(pb::list_models_request::Filter {
                enabled: Some(true),
                ..Default::default()
            }),
            1,
        ),
        (
            Some(pb::list_models_request::Filter {
                model_provider_id: Some(p.id.to_string()),
                ..Default::default()
            }),
            2,
        ),
        (
            Some(pb::list_models_request::Filter {
                enabled: Some(false),
                ..Default::default()
            }),
            1,
        ),
    ] {
        assert_eq!(
            c.list_models(req(pb::ListModelsRequest {
                filter,
                ..Default::default()
            }))
            .await
            .unwrap()
            .message
            .models
            .len(),
            count
        );
    }
    c.delete_model(req(pb::DeleteModelRequest { id }))
        .await
        .unwrap();
    assert_eq!(
        c.create_model(req(pb::CreateModelRequest {
            name: "bad-cap".into(),
            model_provider_id: p.id.to_string(),
            capabilities: vec![999],
            ..Default::default()
        }))
        .await
        .unwrap_err()
        .code,
        Code::Internal
    );
    assert_eq!(
        f.store
            .list_models(ModelFilter::default(), ListOptions::default())
            .await
            .unwrap()
            .len(),
        1
    );
}

#[tokio::test]
async fn agents_go_cases_and_analytics() {
    let f = Fixture::new().await;
    let c = pb::AgentServiceClient::new(f.local_admin.clone());
    assert!(c
        .list_agents(req(pb::ListAgentsRequest::default()))
        .await
        .unwrap()
        .message
        .agents
        .is_empty());
    id_errors!(c, get_agent, GetAgentRequest, "ID", "agent");
    id_errors!(c, update_agent, UpdateAgentRequest, "agent ID", "agent");
    id_errors!(c, delete_agent, DeleteAgentRequest, "agent ID", "agent");
    for (id, code, message) in [
        (
            "invalid-id-value".into(),
            Code::InvalidArgument,
            "invalid model ID format: invalid UUID length: 16",
        ),
        (
            Uuid::new_v4().to_string(),
            Code::NotFound,
            "model not found",
        ),
    ] {
        error(
            c.create_agent(req(pb::CreateAgentRequest {
                name: "agent".into(),
                model_id: id,
                ..Default::default()
            }))
            .await,
            code,
            message,
        );
    }
    let (_, mut m, seed, _) = f.seed().await;
    m.enabled = false;
    f.store.update_model(m.clone()).await.unwrap();
    error(
        c.create_agent(req(pb::CreateAgentRequest {
            name: "agent".into(),
            model_id: m.id.to_string(),
            ..Default::default()
        }))
        .await,
        Code::InvalidArgument,
        "default model is disabled",
    );
    m.enabled = true;
    f.store.update_model(m.clone()).await.unwrap();
    let a = c
        .create_agent(req(pb::CreateAgentRequest {
            name: "architect-agent".into(),
            description: "description".into(),
            instructions: "instructions".into(),
            model_id: m.id.to_string(),
        }))
        .await
        .unwrap()
        .message
        .agent
        .unwrap();
    let id = a.metadata.unwrap().id;
    let spec = a.spec.unwrap();
    assert_eq!(spec.name, "architect-agent");
    assert_eq!(spec.instructions, "instructions");
    for model_id in ["invalid-id-value".into(), Uuid::new_v4().to_string()] {
        assert!(c
            .update_agent(req(pb::UpdateAgentRequest {
                id: id.clone(),
                model_id: Some(model_id),
                ..Default::default()
            }))
            .await
            .is_err());
    }
    assert_eq!(
        f.store
            .get_agent(Uuid::parse_str(&id).unwrap())
            .await
            .unwrap()
            .name,
        "architect-agent"
    );
    let mut replacement = m.clone();
    replacement.id = Uuid::new_v4();
    replacement.name = "replacement-model".into();
    replacement.enabled = false;
    f.store.create_model(replacement.clone()).await.unwrap();
    let a = c
        .update_agent(req(pb::UpdateAgentRequest {
            id: id.clone(),
            name: Some("updated-agent".into()),
            description: Some("updated-description".into()),
            instructions: Some("updated-instructions".into()),
            model_id: Some(replacement.id.to_string()),
        }))
        .await
        .unwrap()
        .message
        .agent
        .unwrap();
    let spec = a.spec.unwrap();
    assert_eq!(spec.description, "updated-description");
    assert_eq!(spec.model_id, replacement.id.to_string());
    assert_eq!(
        c.get_agent(req(pb::GetAgentRequest { id: id.clone() }))
            .await
            .unwrap()
            .message
            .agent
            .unwrap()
            .spec
            .unwrap()
            .instructions,
        "updated-instructions"
    );
    assert_eq!(
        c.list_agents(req(pb::ListAgentsRequest::default()))
            .await
            .unwrap()
            .message
            .agents
            .len(),
        2
    );
    let named = c
        .list_agents(req(pb::ListAgentsRequest {
            filter: Some(pb::list_agents_request::Filter {
                names: vec![seed.name],
            }),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .agents;
    assert_eq!(named.len(), 1);
    c.delete_agent(req(pb::DeleteAgentRequest { id: id.clone() }))
        .await
        .unwrap();
    assert_eq!(
        *f.analytics.0.lock().unwrap(),
        vec![
            vec!["agent_created", &id, "architect-agent", &m.name],
            vec![
                "agent_updated",
                &id,
                "updated-agent",
                "name",
                "description",
                "instructions",
                "model_id"
            ],
            vec!["agent_deleted", &id, "updated-agent"]
        ]
    );
}

#[tokio::test]
async fn tasks_go_cases_count_filter_order_limit_and_events() {
    let f = Fixture::new().await;
    let c = pb::TaskServiceClient::new(f.local_admin.clone());
    assert!(c
        .list_tasks(req(pb::ListTasksRequest::default()))
        .await
        .unwrap()
        .message
        .tasks
        .is_empty());
    id_errors!(c, get_task, GetTaskRequest, "task ID", "task");
    id_errors!(c, update_task, UpdateTaskRequest, "task ID", "task");
    id_errors!(c, delete_task, DeleteTaskRequest, "task ID", "task");
    for (id, code, message) in [
        (
            "invalid-id-value".into(),
            Code::InvalidArgument,
            "invalid agent ID format: invalid UUID length: 16",
        ),
        (
            Uuid::new_v4().to_string(),
            Code::NotFound,
            "agent not found",
        ),
    ] {
        error(
            c.create_task(req(pb::CreateTaskRequest {
                agent_id: id,
                ..Default::default()
            }))
            .await,
            code,
            message,
        );
    }
    let (_, _, a, first) = f.seed().await;
    let t = c
        .create_task(req(pb::CreateTaskRequest {
            agent_id: a.id.to_string(),
            project_directory: "/project".into(),
            description: "description".into(),
        }))
        .await
        .unwrap()
        .message
        .task
        .unwrap();
    let id = t.metadata.unwrap().id;
    assert_eq!(t.spec.unwrap().workspace, "/project");
    assert_eq!(t.status.unwrap().phase, pb::TaskPhase::Awaiting as i32);
    for agent_id in ["invalid-id-value".into(), Uuid::new_v4().to_string()] {
        assert!(c
            .update_task(req(pb::UpdateTaskRequest {
                id: id.clone(),
                agent_id: Some(agent_id)
            }))
            .await
            .is_err());
    }
    let mut external = f.events.subscribe(SubscribeOptions {
        event_types: vec!["task.updated".into()],
        ..Default::default()
    });
    c.update_task(req(pb::UpdateTaskRequest {
        id: id.clone(),
        agent_id: Some(a.id.to_string()),
    }))
    .await
    .unwrap();
    f.events.close();
    assert!(external.next().await.is_none());
    assert_eq!(
        c.get_task(req(pb::GetTaskRequest { id: id.clone() }))
            .await
            .unwrap()
            .message
            .task
            .unwrap()
            .spec
            .unwrap()
            .agent_id,
        Some(a.id.to_string())
    );
    let first_message = f.message(first.id, Some(a.id), "user", 0).await;
    f.message(first.id, Some(a.id), "assistant", 1).await;
    let all = c
        .list_tasks(req(pb::ListTasksRequest::default()))
        .await
        .unwrap()
        .message
        .tasks;
    assert_eq!(all.len(), 2);
    assert_eq!(all[0].metadata.as_ref().unwrap().id, id);
    assert_eq!(all[1].status.as_ref().unwrap().message_count, 2);
    for (has_messages, expected) in [
        (Some(true), first.id.to_string()),
        (Some(false), id.clone()),
    ] {
        let tasks = c
            .list_tasks(req(pb::ListTasksRequest {
                filter: Some(pb::list_tasks_request::Filter {
                    has_messages,
                    ..Default::default()
                }),
                ..Default::default()
            }))
            .await
            .unwrap()
            .message
            .tasks;
        assert_eq!(tasks.len(), 1);
        assert_eq!(tasks[0].metadata.as_ref().unwrap().id, expected);
    }
    for (filter, count) in [
        (
            pb::list_tasks_request::Filter {
                agent_id: Some(a.id.to_string()),
                ..Default::default()
            },
            2,
        ),
        (
            pb::list_tasks_request::Filter {
                task_id_prefix: Some(id[..8].into()),
                ..Default::default()
            },
            1,
        ),
    ] {
        assert_eq!(
            c.list_tasks(req(pb::ListTasksRequest {
                filter: Some(filter),
                ..Default::default()
            }))
            .await
            .unwrap()
            .message
            .tasks
            .len(),
            count
        );
    }
    for field in [pb::SortField::CreatedAt, pb::SortField::UpdatedAt] {
        let tasks = c
            .list_tasks(req(pb::ListTasksRequest {
                sort_field: Some(field as i32),
                sort_order: Some(pb::SortOrder::Asc as i32),
                page_size: Some(1),
                ..Default::default()
            }))
            .await
            .unwrap()
            .message
            .tasks;
        assert_eq!(tasks.len(), 1);
        assert_eq!(tasks[0].metadata.as_ref().unwrap().id, first.id.to_string());
    }
    assert!(c
        .list_tasks(req(pb::ListTasksRequest {
            page_size: Some(0),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message
        .tasks
        .is_empty());
    error(
        c.list_tasks(req(pb::ListTasksRequest {
            filter: Some(pb::list_tasks_request::Filter {
                agent_id: Some("invalid-id-value".into()),
                ..Default::default()
            }),
            ..Default::default()
        }))
        .await,
        Code::InvalidArgument,
        "invalid agent ID format: invalid UUID length: 16",
    );
    c.delete_task(req(pb::DeleteTaskRequest {
        id: first.id.to_string(),
    }))
    .await
    .unwrap();
    assert!(matches!(
        f.store.get_message(first_message.id).await,
        Err(StoreError::NotFound(_))
    ));
    assert_eq!(
        f.analytics
            .0
            .lock()
            .unwrap()
            .iter()
            .map(|e| e[0].as_str())
            .collect::<Vec<_>>(),
        ["task_created", "task_updated"]
    );
}

#[tokio::test]
async fn messages_go_cases_resume_suspend_and_internal_signals() {
    let f = Fixture::new().await;
    let c = pb::MessageServiceClient::new(f.local_admin.clone());
    let tasks = pb::TaskServiceClient::new(f.local_admin.clone());
    assert!(c
        .list_messages(req(pb::ListMessagesRequest::default()))
        .await
        .unwrap()
        .message
        .messages
        .is_empty());
    id_errors!(c, get_message, GetMessageRequest, "ID", "message");
    id_errors!(c, update_message, UpdateMessageRequest, "ID", "message");
    id_errors!(c, delete_message, DeleteMessageRequest, "ID", "message");
    for (id, code, message) in [
        (
            "invalid-id-value".into(),
            Code::InvalidArgument,
            "invalid task ID format: invalid UUID length: 16",
        ),
        (Uuid::new_v4().to_string(), Code::NotFound, "task not found"),
    ] {
        error(
            c.create_message(req(pb::CreateMessageRequest {
                task_id: id,
                content: text("hello"),
            }))
            .await,
            code,
            message,
        );
    }
    let (_, _, a, mut t) = f.seed().await;
    let mut internal = f.events.subscribe(SubscribeOptions {
        event_types: vec!["internal.*".into()],
        internal: true,
        ..Default::default()
    });
    tasks
        .suspend_task(req(pb::SuspendTaskRequest {
            task_id: t.id.to_string(),
        }))
        .await
        .unwrap();
    assert_eq!(
        internal.next().await.unwrap().r#type,
        "internal.task.suspend"
    );
    t = f.store.get_task(t.id).await.unwrap();
    assert_eq!(t.phase.as_ref(), "suspended");
    assert_eq!(t.desired_phase.as_ref(), "running");
    t.desired_phase = mem::TaskPhase::from("suspended");
    f.store.update_task(t.clone()).await.unwrap();
    let m = c
        .create_message(req(pb::CreateMessageRequest {
            task_id: t.id.to_string(),
            content: text("hello"),
        }))
        .await
        .unwrap()
        .message
        .message
        .unwrap();
    let metadata = m.metadata.unwrap();
    let id = metadata.id;
    assert_eq!(metadata.role, pb::MessageRole::User as i32);
    assert_eq!(
        internal.next().await.unwrap().r#type,
        "internal.task.trigger"
    );
    assert_eq!(
        f.store.get_task(t.id).await.unwrap().desired_phase.as_ref(),
        "running"
    );
    assert_eq!(
        c.get_message(req(pb::GetMessageRequest { id: id.clone() }))
            .await
            .unwrap()
            .message
            .message
            .unwrap()
            .spec
            .unwrap()
            .content,
        text("hello")
    );
    let updated = c
        .update_message(req(pb::UpdateMessageRequest {
            id: id.clone(),
            content: text("updated"),
        }))
        .await
        .unwrap()
        .message
        .message
        .unwrap();
    assert_eq!(updated.spec.unwrap().content, text("updated"));
    f.message(t.id, Some(a.id), "assistant", 1).await;
    for (filter, count) in [
        (None, 2),
        (
            Some(pb::list_messages_request::Filter {
                task_ids: Some(t.id.to_string()),
                ..Default::default()
            }),
            2,
        ),
        (
            Some(pb::list_messages_request::Filter {
                agent_ids: Some(a.id.to_string()),
                ..Default::default()
            }),
            1,
        ),
        (
            Some(pb::list_messages_request::Filter {
                roles: Some(pb::MessageRole::User as i32),
                ..Default::default()
            }),
            1,
        ),
        (
            Some(pb::list_messages_request::Filter {
                roles: Some(pb::MessageRole::Assistant as i32),
                ..Default::default()
            }),
            1,
        ),
        (
            Some(pb::list_messages_request::Filter {
                roles: Some(0),
                ..Default::default()
            }),
            0,
        ),
    ] {
        assert_eq!(
            c.list_messages(req(pb::ListMessagesRequest {
                filter,
                ..Default::default()
            }))
            .await
            .unwrap()
            .message
            .messages
            .len(),
            count
        );
    }
    for (filter, message) in [
        (
            pb::list_messages_request::Filter {
                task_ids: Some("invalid-id-value".into()),
                ..Default::default()
            },
            "invalid task ID format: invalid UUID length: 16",
        ),
        (
            pb::list_messages_request::Filter {
                agent_ids: Some("invalid-id-value".into()),
                ..Default::default()
            },
            "invalid agent ID format: invalid UUID length: 16",
        ),
    ] {
        error(
            c.list_messages(req(pb::ListMessagesRequest {
                filter: Some(filter),
                ..Default::default()
            }))
            .await,
            Code::InvalidArgument,
            message,
        );
    }
    c.delete_message(req(pb::DeleteMessageRequest { id }))
        .await
        .unwrap();
    error(
        tasks
            .suspend_task(req(pb::SuspendTaskRequest {
                task_id: "invalid-id-value".into(),
            }))
            .await,
        Code::InvalidArgument,
        "invalid task ID format: invalid UUID length: 16",
    );
    error(
        tasks
            .suspend_task(req(pb::SuspendTaskRequest {
                task_id: Uuid::new_v4().to_string(),
            }))
            .await,
        Code::NotFound,
        "task not found",
    );
}

#[tokio::test]
async fn unix_tcp_auth_exchange_and_replay_live_final_response() {
    let f = Fixture::new().await;
    let unix = pb::AuthServiceClient::new(f.local_admin.clone());
    let tcp = pb::AuthServiceClient::new(f.tcp.clone());
    let agents = pb::AgentServiceClient::new(f.tcp.clone());
    assert_eq!(
        agents
            .list_agents(req(pb::ListAgentsRequest::default()))
            .await
            .unwrap_err()
            .code,
        Code::Unauthenticated
    );
    let setup = unix
        .create_setup_code(req(pb::CreateSetupCodeRequest {
            token_name: "remote".into(),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message;
    let exchange = tcp
        .exchange_setup_code(req(pb::ExchangeSetupCodeRequest {
            setup_code: setup.setup_code.to_lowercase(),
        }))
        .await
        .unwrap()
        .message;
    assert_eq!(exchange.name, "remote");
    let remote = |message| {
        let mut r = req(message);
        r.metadata.insert(
            "authorization",
            format!("Bearer {}", exchange.token).parse().unwrap(),
        );
        r
    };
    let mut r = req(pb::CreateTokenRequest {
        name: "forbidden".into(),
        ..Default::default()
    });
    r.metadata.insert(
        "authorization",
        format!("Bearer {}", exchange.token).parse().unwrap(),
    );
    error(
        tcp.create_token(r).await,
        Code::PermissionDenied,
        "admin privileges required",
    );
    let mut r = req(pb::ListTokensRequest::default());
    r.metadata.insert(
        "authorization",
        format!("Bearer {}", exchange.token).parse().unwrap(),
    );
    error(
        tcp.list_tokens(r).await,
        Code::PermissionDenied,
        "admin privileges required",
    );
    let mut r = req(pb::CreateSetupCodeRequest {
        token_name: "forbidden".into(),
        ..Default::default()
    });
    r.metadata.insert(
        "authorization",
        format!("Bearer {}", exchange.token).parse().unwrap(),
    );
    error(
        tcp.create_setup_code(r).await,
        Code::PermissionDenied,
        "admin privileges required",
    );
    let mut r = req(pb::RevokeTokenRequest {
        id: Uuid::new_v4().to_string(),
    });
    r.metadata.insert(
        "authorization",
        format!("Bearer {}", exchange.token).parse().unwrap(),
    );
    error(
        tcp.revoke_token(r).await,
        Code::PermissionDenied,
        "admin privileges required",
    );
    let list = agents
        .list_agents(remote(pb::ListAgentsRequest::default()))
        .await
        .unwrap();
    assert!(list.message.agents.is_empty());
    let (_, _, a, t) = f.seed().await;
    let replay = f.message(t.id, Some(a.id), "user", 0).await;
    let c = pb::EventServiceClient::new(f.tcp.clone());
    let mut r = req(pb::EventSubscribeRequest {
        task_id: Some(t.id.to_string()),
        event_types: vec!["message.*".into()],
        ..Default::default()
    });
    r.metadata.insert(
        "authorization",
        format!("Bearer {}", exchange.token).parse().unwrap(),
    );
    let mut stream = c.subscribe(r).await.unwrap().message;
    assert_eq!(
        event_message(stream.next().await.unwrap().unwrap())
            .metadata
            .unwrap()
            .id,
        replay.id.to_string()
    );
    let live = f.message(t.id, Some(a.id), "assistant", 1).await;
    f.events
        .publish(construct_event::new_message_created_event(live.clone()));
    let final_message = event_message(stream.next().await.unwrap().unwrap());
    assert_eq!(final_message.metadata.unwrap().id, live.id.to_string());
    assert!(final_message.status.unwrap().is_final_response);
    f.events.close();
    assert!(stream.next().await.is_none());
    let mut r = req(pb::ListAgentsRequest::default());
    r.metadata.insert(
        "authorization",
        format!("Bearer {}", exchange.token).parse().unwrap(),
    );
    let token = unix
        .list_tokens(req(pb::ListTokensRequest::default()))
        .await
        .unwrap()
        .message
        .tokens
        .remove(0);
    unix.revoke_token(req(pb::RevokeTokenRequest { id: token.id }))
        .await
        .unwrap();
    assert_eq!(
        agents.list_agents(r).await.unwrap_err().code,
        Code::Unauthenticated
    );
}

#[tokio::test]
async fn event_replay_after_invalid_replay_conversion_and_cancellation() {
    let f = Fixture::new().await;
    let (_, _, a, t) = f.seed().await;
    let first = f.message(t.id, Some(a.id), "user", -10).await;
    let second = f.message(t.id, Some(a.id), "assistant", -5).await;
    let c = pb::EventServiceClient::new(f.local_admin.clone());
    let mut stream = c
        .subscribe(req(pb::EventSubscribeRequest {
            task_id: Some(t.id.to_string()),
            replay_after_message_id: Some(first.id.to_string()),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message;
    assert_eq!(
        event_message(stream.next().await.unwrap().unwrap())
            .metadata
            .unwrap()
            .id,
        second.id.to_string()
    );
    drop(stream);
    let mut invalid = c
        .subscribe(req(pb::EventSubscribeRequest {
            task_id: Some(t.id.to_string()),
            replay_after_message_id: Some(Uuid::new_v4().to_string()),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message;
    f.events.publish(construct_event::Event {
        r#type: "message.created".into(),
        action: "created".into(),
        timestamp: Timestamp(Utc::now()),
        task_id: Some(t.id),
        payload: Arc::new("unconvertible"),
    });
    f.events
        .publish(construct_event::new_message_created_event(second.clone()));
    assert_eq!(
        event_message(invalid.next().await.unwrap().unwrap())
            .metadata
            .unwrap()
            .id,
        second.id.to_string()
    );
    drop(invalid);
    // Drop the HTTP body; the server must also drop the live subscription.
    tokio::time::timeout(Duration::from_secs(2), async {
        while f.events.subscription_count() != 0 {
            tokio::task::yield_now().await;
        }
    })
    .await
    .unwrap();
}

#[tokio::test]
async fn skills_router_delegates_to_manager() {
    let f = Fixture::new().await;
    let c = pb::SkillServiceClient::new(f.local_admin.clone());
    assert!(c
        .list_skills(req(pb::ListSkillsRequest {}))
        .await
        .unwrap()
        .message
        .skills
        .is_empty());
    assert_eq!(
        c.install_skill(req(pb::InstallSkillRequest {
            source: "".into(),
            ..Default::default()
        }))
        .await
        .unwrap_err()
        .code,
        Code::InvalidArgument
    );
    assert_eq!(
        c.delete_skill(req(pb::DeleteSkillRequest {
            name: "missing".into()
        }))
        .await
        .unwrap_err()
        .code,
        Code::Internal
    );
    assert!(c
        .update_skill(req(pb::UpdateSkillRequest { name: None }))
        .await
        .unwrap()
        .message
        .updated_skills
        .is_empty());
}
