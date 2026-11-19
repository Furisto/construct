#![allow(dead_code)]
use construct::{app::App, context::EndpointContext, paths::UserInfo};
use construct_connect::{ConnectError, Request, Response, Router};
use serde::{de::DeserializeOwned, Serialize};
use std::{
    collections::VecDeque,
    io::{Cursor, Write},
    sync::{Arc, Mutex},
};

type Reply = Result<serde_json::Value, ConnectError>;
struct Expectation {
    method: &'static str,
    request: serde_json::Value,
    reply: Reply,
}
#[derive(Clone, Default)]
pub struct Fake {
    expectations: Arc<Mutex<VecDeque<Expectation>>>,
    requests: Arc<Mutex<Vec<(String, serde_json::Value)>>>,
}
impl Fake {
    pub fn expect<I: Serialize, O: Serialize>(&self, method: &'static str, request: I, reply: O) {
        self.push(method, request, Ok(serde_json::to_value(reply).unwrap()));
    }
    pub fn fail<I: Serialize>(&self, method: &'static str, request: I) {
        self.fail_with(
            method,
            request,
            ConnectError::new(construct_connect::Code::Internal, ""),
        );
    }
    pub fn fail_with<I: Serialize>(&self, method: &'static str, request: I, error: ConnectError) {
        self.push(method, request, Err(error));
    }
    fn push<I: Serialize>(&self, method: &'static str, request: I, reply: Reply) {
        self.expectations.lock().unwrap().push_back(Expectation {
            method,
            request: serde_json::to_value(request).unwrap(),
            reply,
        });
    }
    fn call<I: Serialize, O: DeserializeOwned>(
        &self,
        method: &str,
        request: Request<I>,
    ) -> Result<Response<O>, ConnectError> {
        let request = serde_json::to_value(request.message).unwrap();
        self.requests
            .lock()
            .unwrap()
            .push((method.into(), request.clone()));
        let expected = self
            .expectations
            .lock()
            .unwrap()
            .pop_front()
            .expect("unexpected RPC");
        assert_eq!(method, expected.method);
        assert_eq!(request, expected.request, "{method}");
        expected
            .reply
            .map(|value| Response::new(serde_json::from_value(value).unwrap()))
    }
    pub fn done(&self) {
        assert!(
            self.expectations.lock().unwrap().is_empty(),
            "unconsumed RPC expectations"
        );
    }
    pub fn requests(&self) -> Vec<(String, serde_json::Value)> {
        self.requests.lock().unwrap().clone()
    }
}
macro_rules! service {
    ($trait:ident, $($method:ident($input:ident) -> $output:ident),* $(,)?) => {
        #[construct_connect::async_trait]
        impl construct_proto::construct::v1::$trait for Fake {
            $(async fn $method(&self, request: Request<construct_proto::construct::v1::$input>) -> Result<Response<construct_proto::construct::v1::$output>, ConnectError> { self.call(stringify!($method),request) })*
        }
    };
}
service!(AgentService, create_agent(CreateAgentRequest)->CreateAgentResponse, get_agent(GetAgentRequest)->GetAgentResponse, list_agents(ListAgentsRequest)->ListAgentsResponse, update_agent(UpdateAgentRequest)->UpdateAgentResponse, delete_agent(DeleteAgentRequest)->DeleteAgentResponse);
service!(ModelService, create_model(CreateModelRequest)->CreateModelResponse, get_model(GetModelRequest)->GetModelResponse, list_models(ListModelsRequest)->ListModelsResponse, delete_model(DeleteModelRequest)->DeleteModelResponse);
service!(ModelProviderService, create_model_provider(CreateModelProviderRequest)->CreateModelProviderResponse, get_model_provider(GetModelProviderRequest)->GetModelProviderResponse, list_model_providers(ListModelProvidersRequest)->ListModelProvidersResponse, delete_model_provider(DeleteModelProviderRequest)->DeleteModelProviderResponse);
service!(TaskService, create_task(CreateTaskRequest)->CreateTaskResponse, get_task(GetTaskRequest)->GetTaskResponse, list_tasks(ListTasksRequest)->ListTasksResponse, delete_task(DeleteTaskRequest)->DeleteTaskResponse);
service!(MessageService, create_message(CreateMessageRequest)->CreateMessageResponse, get_message(GetMessageRequest)->GetMessageResponse, list_messages(ListMessagesRequest)->ListMessagesResponse, delete_message(DeleteMessageRequest)->DeleteMessageResponse);

#[derive(Clone, Default)]
struct Output(Arc<Mutex<Vec<u8>>>);
impl Write for Output {
    fn write(&mut self, bytes: &[u8]) -> std::io::Result<usize> {
        self.0.lock().unwrap().extend_from_slice(bytes);
        Ok(bytes.len())
    }
    fn flush(&mut self) -> std::io::Result<()> {
        Ok(())
    }
}
pub struct Harness {
    pub dir: tempfile::TempDir,
    pub app: App,
    pub fake: Fake,
    output: Output,
    server: tokio::task::JoinHandle<()>,
}
impl Harness {
    pub async fn new() -> Self {
        use construct_proto::construct::v1::{
            AgentServiceServer, MessageServiceServer, ModelProviderServiceServer,
            ModelServiceServer, TaskServiceServer,
        };
        let fake = Fake::default();
        let router = Router::new().nest(
            "/api",
            AgentServiceServer::new(fake.clone())
                .router()
                .merge(ModelServiceServer::new(fake.clone()).router())
                .merge(ModelProviderServiceServer::new(fake.clone()).router())
                .merge(TaskServiceServer::new(fake.clone()).router())
                .merge(MessageServiceServer::new(fake.clone()).router()),
        );
        Self::with_router(fake, router).await
    }
    pub async fn with_router(fake: Fake, router: Router) -> Self {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = format!("http://{}", listener.local_addr().unwrap());
        let server = tokio::spawn(async move {
            construct_connect::serve_tcp(listener, router, std::future::pending::<()>())
                .await
                .unwrap();
        });
        let dir = tempfile::tempdir().unwrap();
        let user = UserInfo {
            home: dir.path().into(),
            config: dir.path().join("config"),
            data: dir.path().join("data"),
            log: dir.path().join("log"),
            runtime: dir.path().join("runtime"),
        };
        let mut app = App::new(user, "test".into()).unwrap();
        app.contexts
            .upsert(
                "test",
                EndpointContext {
                    address,
                    kind: "http".into(),
                    auth: None,
                },
                true,
            )
            .unwrap();
        let output = Output::default();
        app.stdout = Box::new(output.clone());
        app.stdin = Box::new(Cursor::new(Vec::<u8>::new()));
        Self {
            dir,
            app,
            fake,
            output,
            server,
        }
    }
    pub fn stdin(&mut self, input: &str) {
        self.app.stdin = Box::new(Cursor::new(input.as_bytes().to_vec()));
    }
    pub fn stdout(&self) -> String {
        String::from_utf8(self.output.0.lock().unwrap().clone()).unwrap()
    }
    pub async fn agent(&mut self, args: &[&str]) -> construct::Result<()> {
        let matches = construct::commands::agent::command()
            .try_get_matches_from(std::iter::once("agent").chain(args.iter().copied()))?;
        construct::commands::agent::run(&mut self.app, &matches).await
    }
    pub async fn run(&mut self, group: &str, args: &[&str]) -> construct::Result<()> {
        match group {
            "agent" => self.agent(args).await,
            "model" => {
                let matches = construct::commands::model::command()
                    .try_get_matches_from(std::iter::once("model").chain(args.iter().copied()))?;
                construct::commands::model::run(&mut self.app, &matches).await
            }
            "modelprovider" => {
                let matches = construct::commands::modelprovider::command().try_get_matches_from(
                    std::iter::once("modelprovider").chain(args.iter().copied()),
                )?;
                construct::commands::modelprovider::run(&mut self.app, &matches).await
            }
            "task" => {
                let matches = construct::commands::task::command()
                    .try_get_matches_from(std::iter::once("task").chain(args.iter().copied()))?;
                construct::commands::task::run(&mut self.app, &matches).await
            }
            "message" => {
                let matches = construct::commands::message::command()
                    .try_get_matches_from(std::iter::once("message").chain(args.iter().copied()))?;
                construct::commands::message::run(&mut self.app, &matches).await
            }
            _ => panic!("unsupported command group: {group}"),
        }
    }
}
impl Drop for Harness {
    fn drop(&mut self) {
        self.server.abort();
    }
}
