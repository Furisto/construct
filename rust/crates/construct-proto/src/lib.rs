//! Construct API messages and proto3 JSON implementations generated from `api/def`.
//!
//! ```no_run
//! use construct_connect::{async_trait, Channel, ClientConfig, ConnectError, Request, Response};
//! use construct_proto::construct::v1::{TaskService, TaskServiceServer, TaskServiceClient,
//!     GetTaskRequest, GetTaskResponse};
//!
//! struct Tasks;
//! #[async_trait]
//! impl TaskService for Tasks {
//!     async fn get_task(&self, request: Request<GetTaskRequest>)
//!         -> Result<Response<GetTaskResponse>, ConnectError>
//!     {
//!         Ok(Response::new(GetTaskResponse::default()))
//!     }
//! }
//!
//! # async fn example() -> Result<(), Box<dyn std::error::Error>> {
//! let router = construct_connect::Router::new()
//!     .nest("/api", TaskServiceServer::new(Tasks).router());
//! let listener = tokio::net::TcpListener::bind("127.0.0.1:8080").await?;
//! tokio::spawn(construct_connect::serve_tcp(listener, router, std::future::pending()));
//! let channel = Channel::new(ClientConfig::new("http://127.0.0.1:8080/api"))?;
//! let client = TaskServiceClient::new(channel);
//! let reply = client.get_task(Request::new(GetTaskRequest { id: "task-id".into() })).await?;
//! # Ok(()) }
//! ```

mod timestamp;

/// Static metadata for a Connect RPC method. Type names are fully qualified proto names.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MethodDescriptor {
    pub name: &'static str,
    pub path: &'static str,
    pub input_type: &'static str,
    pub output_type: &'static str,
    pub server_streaming: bool,
}

#[allow(unknown_lints, clippy::useless_borrows_in_formatting)]
pub mod google {
    pub mod r#type {
        include!(concat!(env!("OUT_DIR"), "/google.r#type.rs"));
        include!(concat!(env!("OUT_DIR"), "/google.r#type.serde.rs"));
    }
}

#[allow(unknown_lints, clippy::useless_borrows_in_formatting)]
pub mod construct {
    pub mod v1 {
        include!(concat!(env!("OUT_DIR"), "/construct.v1.rs"));
        include!(concat!(env!("OUT_DIR"), "/construct.v1.serde.rs"));
    }
}
