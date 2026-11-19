use construct_connect::{
    async_trait, http::HeaderMap, serve_tcp, serve_unix, with_interceptor, Channel, ClientConfig,
    Code, Codec, ConnectError, Endpoint, Interceptor, Request, Response, Router, Streaming,
    Transport,
};
use construct_proto::construct::v1::*;
use futures_util::{stream, StreamExt};
use std::{
    path::PathBuf,
    sync::{
        atomic::{AtomicUsize, Ordering},
        Arc,
    },
    time::Duration,
};
use tokio::{
    net::{TcpListener, UnixListener},
    sync::oneshot,
};

struct Tasks;

#[async_trait]
impl TaskService for Tasks {
    async fn get_task(
        &self,
        req: Request<GetTaskRequest>,
    ) -> Result<Response<GetTaskResponse>, ConnectError> {
        let transport = req.transport.expect("server transport extension");
        if req.message.id == "missing" {
            let mut error = ConnectError::new(Code::NotFound, "task does not exist");
            error
                .details
                .push(serde_json::json!({"type": "test.Detail", "value": "AQ=="}));
            error.metadata.insert("x-error", "present".parse().unwrap());
            return Err(error);
        }
        if req.message.id == "slow" {
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
        if req.message.id == "metadata" {
            assert_eq!(req.metadata["x-request"], "present");
        }
        let mut response = Response::new(GetTaskResponse {
            task: Some(Task {
                metadata: Some(TaskMetadata {
                    id: req.message.id,
                    ..Default::default()
                }),
                spec: Some(TaskSpec {
                    workspace: format!("{transport:?}"),
                    ..Default::default()
                }),
                ..Default::default()
            }),
        });
        response
            .metadata
            .insert("x-response", "present".parse().unwrap());
        Ok(response)
    }
}

struct Events;

#[async_trait]
impl EventService for Events {
    async fn subscribe(
        &self,
        req: Request<EventSubscribeRequest>,
    ) -> Result<Response<Streaming<EventSubscribeResponse>>, ConnectError> {
        assert!(req.transport.is_some());
        let mode = req.message.task_id.unwrap_or_default();
        if mode == "immediate-error" {
            return Err(ConnectError::new(Code::NotFound, "no subscription"));
        }
        let messages = if mode == "empty" { 0 } else { 3 };
        let mut values: Vec<_> = (0..messages)
            .map(|index| {
                Ok(EventSubscribeResponse {
                    event: Some(Event {
                        r#type: format!("event.{index}"),
                        ..Default::default()
                    }),
                })
            })
            .collect();
        if mode == "error" {
            let mut error = ConnectError::new(Code::Aborted, "subscription stopped");
            error.metadata.append("x-trailer", "one".parse().unwrap());
            error.metadata.append("x-trailer", "two".parse().unwrap());
            values.push(Err(error));
        }
        let output = stream::iter(values).then(move |value| {
            let slow = mode == "slow";
            async move {
                if slow {
                    tokio::time::sleep(Duration::from_millis(100)).await;
                }
                value
            }
        });
        let mut response = Response::new(Box::pin(output) as Streaming<_>);
        response
            .metadata
            .insert("x-stream", "present".parse().unwrap());
        Ok(response)
    }
}

struct Auth;

#[async_trait]
impl Interceptor for Auth {
    async fn intercept(
        &self,
        headers: &HeaderMap,
        transport: Option<Transport>,
    ) -> Result<(), ConnectError> {
        assert!(transport.is_some());
        if headers
            .get("authorization")
            .is_some_and(|v| v == "Bearer test")
        {
            Ok(())
        } else {
            Err(ConnectError::new(
                Code::Unauthenticated,
                "missing auth token",
            ))
        }
    }
}

struct Server {
    endpoint: Endpoint,
    transport: Transport,
    shutdown: oneshot::Sender<()>,
    task: tokio::task::JoinHandle<std::io::Result<()>>,
    socket: Option<PathBuf>,
}

impl Server {
    async fn start(unix: bool) -> Self {
        let tasks: Arc<dyn TaskService> = Arc::new(Tasks);
        let services = TaskServiceServer::from_arc(tasks)
            .router()
            .merge(EventServiceServer::new(Events).router());
        let router = with_interceptor(Router::new().nest("/api", services), Arc::new(Auth));
        let (shutdown, receiver) = oneshot::channel();
        let stop = async {
            let _ = receiver.await;
        };
        if unix {
            static NEXT: AtomicUsize = AtomicUsize::new(0);
            let path = std::env::temp_dir().join(format!(
                "construct-{}-{}.sock",
                std::process::id(),
                NEXT.fetch_add(1, Ordering::Relaxed)
            ));
            let listener = UnixListener::bind(&path).unwrap();
            let task = tokio::spawn(serve_unix(listener, router, stop));
            Self {
                endpoint: Endpoint::Unix {
                    path: path.clone(),
                    prefix: "/api".into(),
                },
                transport: Transport::Unix,
                shutdown,
                task,
                socket: Some(path),
            }
        } else {
            let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
            let endpoint = Endpoint::Http(format!("http://{}/api", listener.local_addr().unwrap()));
            let task = tokio::spawn(serve_tcp(listener, router, stop));
            Self {
                endpoint,
                transport: Transport::Tcp,
                shutdown,
                task,
                socket: None,
            }
        }
    }

    fn config(&self, codec: Codec, auth: bool) -> ClientConfig {
        let mut config = ClientConfig::new(self.endpoint.clone());
        config.codec = codec;
        if auth {
            config
                .default_headers
                .insert("authorization", "Bearer test".parse().unwrap());
        }
        config
    }

    async fn stop(self) {
        self.shutdown.send(()).unwrap();
        tokio::time::timeout(Duration::from_secs(2), self.task)
            .await
            .unwrap()
            .unwrap()
            .unwrap();
        if let Some(path) = self.socket {
            std::fs::remove_file(path).unwrap();
        }
    }
}

async fn exercise(unix: bool, codec: Codec) {
    let server = Server::start(unix).await;
    let channel = Channel::new(server.config(codec, true)).unwrap();
    let client = TaskServiceClient::new(channel.clone());
    let events = EventServiceClient::new(channel);

    let mut request = Request::new(GetTaskRequest {
        id: "metadata".into(),
    });
    request
        .metadata
        .insert("x-request", "present".parse().unwrap());
    let response = client.get_task(request).await.unwrap();
    assert_eq!(response.metadata["x-response"], "present");
    let task = response.message.task.unwrap();
    assert_eq!(task.metadata.unwrap().id, "metadata");
    assert_eq!(
        task.spec.unwrap().workspace,
        format!("{:?}", server.transport)
    );

    let error = client
        .get_task(Request::new(GetTaskRequest {
            id: "missing".into(),
        }))
        .await
        .unwrap_err();
    assert_eq!(error.code, Code::NotFound);
    assert_eq!(error.message, "task does not exist");
    assert_eq!(error.details.len(), 1);
    assert_eq!(error.metadata["x-error"], "present");

    // Empty/default payloads are valid even though validation annotations are not enforced.
    let empty = client
        .get_task(Request::new(GetTaskRequest::default()))
        .await
        .unwrap();
    assert_eq!(empty.message.task.unwrap().metadata.unwrap().id, "");
    let error = client
        .create_task(Request::new(CreateTaskRequest::default()))
        .await
        .unwrap_err();
    assert_eq!(error.code, Code::Unimplemented);

    for mode in ["normal", "empty", "error", "immediate-error"] {
        let mut response = events
            .subscribe(Request::new(EventSubscribeRequest {
                task_id: Some(mode.into()),
                ..Default::default()
            }))
            .await
            .unwrap();
        if mode != "immediate-error" {
            assert_eq!(response.metadata["x-stream"], "present");
        }
        let mut count = 0;
        let mut error = None;
        while let Some(item) = response.message.next().await {
            match item {
                Ok(message) => {
                    assert_eq!(message.event.unwrap().r#type, format!("event.{count}"));
                    count += 1;
                }
                Err(e) => {
                    error = Some(e);
                    break;
                }
            }
        }
        assert_eq!(
            count,
            if mode == "empty" || mode == "immediate-error" {
                0
            } else {
                3
            }
        );
        match mode {
            "error" => {
                let error = error.unwrap();
                assert_eq!(error.code, Code::Aborted);
                assert_eq!(error.message, "subscription stopped");
                assert_eq!(error.metadata["x-stream"], "present");
                assert_eq!(error.metadata.get_all("x-trailer").iter().count(), 2);
            }
            "immediate-error" => assert_eq!(error.unwrap().code, Code::NotFound),
            _ => assert!(error.is_none()),
        }
        assert!(response.message.next().await.is_none());
    }

    let unauth_channel = Channel::new(server.config(codec, false)).unwrap();
    let unauth = TaskServiceClient::new(unauth_channel.clone());
    let error = unauth
        .get_task(Request::new(GetTaskRequest::default()))
        .await
        .unwrap_err();
    assert_eq!(error.code, Code::Unauthenticated);
    assert_eq!(error.message, "missing auth token");
    let mut unauth_stream = EventServiceClient::new(unauth_channel)
        .subscribe(Request::new(EventSubscribeRequest::default()))
        .await
        .unwrap()
        .message;
    assert_eq!(
        unauth_stream.next().await.unwrap().unwrap_err().code,
        Code::Unauthenticated
    );

    let mut request = Request::new(GetTaskRequest { id: "slow".into() });
    request.timeout = Some(Duration::from_millis(15));
    assert_eq!(
        client.get_task(request).await.unwrap_err().code,
        Code::DeadlineExceeded
    );
    let mut request = Request::new(EventSubscribeRequest {
        task_id: Some("slow".into()),
        ..Default::default()
    });
    request.timeout = Some(Duration::from_millis(15));
    let mut response = events.subscribe(request).await.unwrap();
    assert_eq!(
        response.message.next().await.unwrap().unwrap_err().code,
        Code::DeadlineExceeded
    );

    let mut config = server.config(codec, true);
    config
        .default_headers
        .insert("content-encoding", "identity".parse().unwrap());
    config
        .default_headers
        .insert("connect-content-encoding", "identity".parse().unwrap());
    let identity = Channel::new(config).unwrap();
    TaskServiceClient::new(identity.clone())
        .get_task(Request::new(GetTaskRequest::default()))
        .await
        .unwrap();
    let mut stream = EventServiceClient::new(identity)
        .subscribe(Request::new(EventSubscribeRequest {
            task_id: Some("empty".into()),
            ..Default::default()
        }))
        .await
        .unwrap()
        .message;
    assert!(stream.next().await.is_none());

    let mut config = server.config(codec, true);
    config
        .default_headers
        .insert("content-encoding", "gzip".parse().unwrap());
    let error = TaskServiceClient::new(Channel::new(config).unwrap())
        .get_task(Request::new(GetTaskRequest::default()))
        .await
        .unwrap_err();
    assert_eq!(error.code, Code::Unimplemented);
    server.stop().await;
}

#[tokio::test]
async fn tcp_proto() {
    exercise(false, Codec::Proto).await;
}
#[tokio::test]
async fn tcp_json() {
    exercise(false, Codec::Json).await;
}
#[tokio::test]
async fn unix_proto() {
    exercise(true, Codec::Proto).await;
}
#[tokio::test]
async fn unix_json() {
    exercise(true, Codec::Json).await;
}
