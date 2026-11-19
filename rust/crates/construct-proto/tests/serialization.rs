use construct_proto::construct::v1::{
    event_service, task_service, DeleteTaskResponse, Task, TaskMetadata, TaskPhase, TaskSpec,
    TaskStatus,
};
use prost::Message;
use serde_json::json;

#[test]
fn binary_and_proto3_json_round_trip() {
    let task = Task {
        metadata: Some(TaskMetadata {
            id: "task-1".into(),
            created_at: Some(pbjson_types::Timestamp {
                seconds: 1_704_067_200,
                nanos: 0,
            }),
            updated_at: None,
        }),
        spec: Some(TaskSpec {
            agent_id: Some("agent-1".into()),
            desired_phase: TaskPhase::Running.into(),
            ..Default::default()
        }),
        status: Some(TaskStatus {
            turn: 9_007_199_254_740_993,
            ..Default::default()
        }),
    };
    assert_eq!(Task::decode(task.encode_to_vec().as_slice()).unwrap(), task);
    let value = serde_json::to_value(&task).unwrap();
    assert_eq!(
        value,
        json!({
            "metadata": {"id": "task-1", "createdAt": "2024-01-01T00:00:00+00:00"},
            "spec": {"agentId": "agent-1", "desiredPhase": "TASK_PHASE_RUNNING"},
            "status": {"turn": "9007199254740993"}
        })
    );
    assert_eq!(serde_json::from_value::<Task>(value).unwrap(), task);
    let metadata: TaskMetadata = serde_json::from_value(json!({
        "id": "task-1", "createdAt": "2024-01-01T00:00:00Z"
    }))
    .unwrap();
    assert_eq!(Some(metadata), task.metadata);
}

#[test]
fn empty_messages_round_trip() {
    let message = DeleteTaskResponse {};
    assert!(message.encode_to_vec().is_empty());
    assert_eq!(DeleteTaskResponse::decode(&[][..]).unwrap(), message);
    assert_eq!(serde_json::to_string(&message).unwrap(), "{}");
    assert_eq!(
        serde_json::from_str::<DeleteTaskResponse>("{}").unwrap(),
        message
    );
    assert_eq!(serde_json::from_str::<Task>("{}").unwrap(), Task::default());
}

#[test]
fn malformed_binary_and_json_are_rejected() {
    assert!(Task::decode(&[0xff][..]).is_err());
    assert!(serde_json::from_value::<TaskSpec>(json!({"desiredPhase": "NOT_A_PHASE"})).is_err());
    assert!(serde_json::from_value::<TaskMetadata>(json!({"createdAt": "not-a-time"})).is_err());
    assert!(serde_json::from_str::<TaskSpec>(r#"{"agentId":"a","agentId":"b"}"#).is_err());
}

#[test]
fn service_metadata_preserves_proto_names_and_streaming() {
    assert_eq!(task_service::SERVICE_NAME, "construct.v1.TaskService");
    assert_eq!(task_service::METHODS.len(), 6);
    let create = &task_service::METHODS[0];
    assert_eq!(create.name, "CreateTask");
    assert_eq!(create.path, "/construct.v1.TaskService/CreateTask");
    assert_eq!(create.input_type, "construct.v1.CreateTaskRequest");
    assert_eq!(create.output_type, "construct.v1.CreateTaskResponse");
    assert!(!create.server_streaming);
    assert_eq!(event_service::SERVICE_NAME, "construct.v1.EventService");
    assert_eq!(event_service::METHODS.len(), 1);
    assert!(event_service::METHODS[0].server_streaming);
}
