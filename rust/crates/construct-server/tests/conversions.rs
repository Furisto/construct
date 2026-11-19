use construct_proto::{construct::v1 as pb, google::r#type::Decimal};
use construct_server::conv::*;
use construct_types::{entity as db, memory as mem, provider, tools, Timestamp};
use serde_json::json;
use uuid::Uuid;

fn id() -> Uuid {
    Uuid::from_u128(0x0123456789abcdef0123456789abcdef)
}
fn text(content: &str) -> pb::MessagePart {
    pb::MessagePart {
        data: Some(pb::message_part::Data::Text(pb::message_part::Text {
            content: content.into(),
        })),
    }
}
fn message(source: &str, blocks: Vec<mem::MessageBlock>) -> db::Message {
    db::Message {
        id: id(),
        task_id: id(),
        source: source.into(),
        content: Some(mem::MessageContent {
            blocks: Some(blocks),
        }),
        ..Default::default()
    }
}
fn block(kind: &str, payload: &str) -> mem::MessageBlock {
    mem::MessageBlock {
        kind: kind.into(),
        payload: payload.into(),
    }
}

#[test]
fn uuid_and_timestamp_helpers_preserve_go_zero_and_presence() {
    assert_eq!(convert_uuid_to_string(Uuid::nil()), "");
    assert_eq!(convert_uuid_to_string(id()), id().to_string());
    assert_eq!(convert_uuid_ptr_to_string_ptr(Uuid::nil()), None);
    assert_eq!(convert_uuid_ptr_to_string_ptr(id()), Some(id().to_string()));
    assert_eq!(convert_uuid_ptr_to_string(None), "");
    assert_eq!(
        convert_uuid_ptr_to_string(Some(Uuid::nil())),
        Uuid::nil().to_string()
    );
    assert_eq!(convert_string_to_uuid("").unwrap(), Uuid::nil());
    assert_eq!(convert_string_ptr_to_uuid(None).unwrap(), Uuid::nil());
    assert!(convert_string_ptr_to_uuid(Some("")).is_err());
    assert!(convert_string_to_uuid("invalid").is_err());
    assert_eq!(convert_string_to_uuid(&id().to_string()).unwrap(), id());
    assert_eq!(
        convert_string_ptr_to_uuid(Some(&id().to_string())).unwrap(),
        id()
    );
    for time in [
        Timestamp::default(),
        Timestamp(chrono::DateTime::from_timestamp(-1, 123456789).unwrap()),
    ] {
        assert_eq!(
            convert_timestamp_to_time(Some(&convert_time_to_timestamp(&time))).unwrap(),
            time
        );
    }
    assert_eq!(convert_timestamp_to_time(None).unwrap().0.timestamp(), 0);
    assert!(convert_timestamp_to_time(Some(&pbjson_types::Timestamp {
        seconds: i64::MAX,
        nanos: 0
    }))
    .is_err());
    assert_eq!(float64_to_proto_decimal(0.00005).value, "0.000050");
    assert_eq!(float64_to_proto_decimal(-1.25).value, "-1.250000");
    assert_eq!(
        MissingRelatedEntityError {
            entity: "model".into()
        }
        .to_string(),
        "missing related entity: model"
    );
}

#[test]
fn agent_and_task_match_go_api_conversion_expectations() {
    let agent = db::Agent {
        id: id(),
        name: "architect-agent".into(),
        description: "Architect agent".into(),
        instructions: "Instructions for architect agent".into(),
        model_id: Some(id()),
        ..Default::default()
    };
    let a = convert_agent_to_proto(&agent);
    assert_eq!(a.metadata.unwrap().id, id().to_string());
    assert_eq!(
        a.spec.unwrap(),
        pb::AgentSpec {
            name: agent.name.clone(),
            description: agent.description.clone(),
            instructions: agent.instructions.clone(),
            model_id: id().to_string()
        }
    );
    assert!(convert_agent_spec_to_proto(&db::Agent::default())
        .model_id
        .is_empty());
    let task = db::Task {
        id: id(),
        agent_id: Some(id()),
        project_directory: "/tmp/test".into(),
        desired_phase: "running".into(),
        phase: "awaiting".into(),
        description: "test task".into(),
        input_tokens: 10,
        output_tokens: 20,
        cache_write_tokens: 30,
        cache_read_tokens: 40,
        cost: 0.5,
        turns: 99,
        tool_uses: Some([("read_file".into(), 2)].into()),
        ..Default::default()
    };
    let t = convert_task_to_proto(&task);
    assert_eq!(t.metadata.unwrap(), convert_task_metadata_to_proto(&task));
    assert_eq!(
        t.spec.unwrap(),
        pb::TaskSpec {
            agent_id: Some(id().to_string()),
            workspace: "/tmp/test".into(),
            desired_phase: pb::TaskPhase::Running as i32,
            description: "test task".into()
        }
    );
    assert_eq!(
        t.status.unwrap(),
        pb::TaskStatus {
            phase: pb::TaskPhase::Awaiting as i32,
            usage: Some(pb::TaskUsage {
                input_tokens: 10,
                output_tokens: 20,
                cache_write_tokens: 30,
                cache_read_tokens: 40,
                cost: 0.5,
                tool_uses: [("read_file".into(), 2)].into()
            }),
            ..Default::default()
        }
    );
    assert_eq!(
        convert_task_spec_to_proto(&db::Task::default()).agent_id,
        Some(Uuid::nil().to_string())
    );
    for (value, expected) in [
        ("running", pb::TaskPhase::Running),
        ("awaiting", pb::TaskPhase::Awaiting),
        ("suspended", pb::TaskPhase::Suspended),
        ("", pb::TaskPhase::Unspecified),
        ("unknown", pb::TaskPhase::Unspecified),
    ] {
        assert_eq!(convert_task_phase_to_proto(&value.into()), expected);
    }
}

#[test]
fn model_pricing_capabilities_and_provider_match_go_expectations() {
    let model = db::Model {
        id: id(),
        model_provider_id: id(),
        name: "test-model".into(),
        context_window: 4096,
        enabled: true,
        alias: "alias".into(),
        input_cost: 0.0001,
        output_cost: 0.0002,
        cache_write_cost: 0.00005,
        cache_read_cost: 0.00001,
        capabilities: Some(vec!["image".into()]),
        ..Default::default()
    };
    let proto = memory_model_to_proto(&model);
    assert_eq!(proto.metadata.unwrap().model_provider_id, id().to_string());
    let spec = proto.spec.unwrap();
    assert_eq!(spec.name, "test-model");
    assert_eq!(spec.context_window, 4096);
    assert!(spec.enabled);
    assert_eq!(spec.alias, "alias");
    assert_eq!(spec.capabilities, vec![pb::ModelCapability::Image as i32]);
    let pricing = spec.pricing.unwrap();
    assert_eq!(pricing.input_cost.as_ref().unwrap().value, "0.000100");
    assert_eq!(pricing.cache_write_cost.as_ref().unwrap().value, "0.000050");
    assert_eq!(
        proto_model_pricing_to_memory(&pricing).unwrap(),
        (0.0001, 0.0002, 0.00005, 0.00001)
    );
    assert!(proto_model_pricing_to_memory(&pb::ModelPricing::default()).is_err());
    let invalid = pb::ModelPricing {
        input_cost: Some(Decimal {
            value: "bad".into(),
        }),
        ..pricing.clone()
    };
    assert!(proto_model_pricing_to_memory(&invalid).is_err());
    for value in [
        "image",
        "computer_use",
        "prompt_cache",
        "extended_thinking",
        "audio",
    ] {
        let cap = mem::ModelCapability::from(value);
        assert_eq!(
            proto_model_capability_to_memory(memory_model_capability_to_proto(&cap) as i32)
                .unwrap(),
            cap
        );
        assert_eq!(
            llm_model_capabilities_to_memory(&[provider::Capability::from(value)]).unwrap(),
            vec![cap]
        );
    }
    assert_eq!(
        memory_model_capability_to_proto(&"future".into()),
        pb::ModelCapability::Unspecified
    );
    assert!(proto_model_capability_to_memory(0).is_err());
    assert!(proto_model_capability_to_memory(99).is_err());
    assert!(llm_model_capabilities_to_memory(&["future".into()]).is_err());
    assert!(llm_model_capabilities_to_memory(&[]).unwrap().is_empty());
    assert!(convert_model_provider_into_proto(None).unwrap().is_none());
    for value in ["anthropic", "openai", "gemini", "xai"] {
        let kind = mem::ModelProviderType::from(value);
        assert_eq!(
            convert_model_provider_type_to_memory(
                convert_model_provider_type_to_proto(&kind).unwrap() as i32
            )
            .unwrap(),
            kind
        );
        let p = db::ModelProvider {
            id: id(),
            name: "provider".into(),
            provider_type: kind,
            secret: b"must never leak".to_vec(),
            url: "secret-url".into(),
            enabled: true,
            ..Default::default()
        };
        let wire = convert_model_provider_into_proto(Some(&p))
            .unwrap()
            .unwrap();
        assert_eq!(
            wire.spec.unwrap(),
            pb::ModelProviderSpec {
                name: "provider".into(),
                enabled: true
            }
        );
    }
    assert!(convert_model_provider_type_to_proto(&"unknown".into()).is_err());
    assert!(convert_model_provider_type_to_memory(0).is_err());
    assert!(convert_model_provider_type_to_memory(99).is_err());
    assert_eq!(
        convert_model_provider_type_to_memory(0)
            .unwrap_err()
            .to_string(),
        "unsupported provider type: MODEL_PROVIDER_TYPE_UNSPECIFIED"
    );
    assert_eq!(
        proto_model_capability_to_memory(0).unwrap_err().to_string(),
        "unknown model capability: MODEL_CAPABILITY_UNSPECIFIED"
    );
}

#[test]
fn message_conversion_matches_go_fields_and_usage() {
    let mut m = message("user", vec![block("text", "Test message content")]);
    m.agent_id = Some(id());
    m.model_id = Some(id());
    m.usage = Some(mem::MessageUsage {
        input_tokens: 1,
        output_tokens: 2,
        cache_write_tokens: 3,
        cache_read_tokens: 4,
        cost: 0.125,
    });
    let proto = convert_memory_message_to_proto(Some(&m)).unwrap();
    assert_eq!(
        proto.spec.as_ref().unwrap().content,
        vec![text("Test message content")]
    );
    assert_eq!(
        proto.metadata.as_ref().unwrap().agent_id,
        Some(id().to_string())
    );
    assert_eq!(
        proto.metadata.as_ref().unwrap().role,
        pb::MessageRole::User as i32
    );
    assert_eq!(
        proto
            .status
            .as_ref()
            .unwrap()
            .usage
            .as_ref()
            .unwrap()
            .cache_read_tokens,
        4
    );
    let roundtrip = convert_proto_message_to_memory(Some(&proto)).unwrap();
    assert_eq!(roundtrip.id, m.id);
    assert_eq!(roundtrip.task_id, m.task_id);
    assert_eq!(roundtrip.agent_id, m.agent_id);
    assert_eq!(roundtrip.model_id, m.model_id);
    assert_eq!(roundtrip.create_time, m.create_time);
    assert_eq!(roundtrip.usage.unwrap().cost, 0.125);
    assert_eq!(
        roundtrip.content.unwrap().blocks.unwrap()[0].payload,
        "Test message content"
    );
    assert!(convert_proto_usage_to_memory_usage(None).is_none());
    assert!(convert_memory_message_to_proto(None).is_err());
    assert!(convert_proto_message_to_memory(None).is_err());
    assert!(convert_proto_message_to_memory(Some(&pb::Message::default())).is_err());
    assert!(convert_memory_message_to_proto(Some(&message("unknown", vec![]))).is_err());
    for role in [0, 3, 99] {
        assert!(convert_proto_role_to_memory_role(role).is_err());
    }
    assert_eq!(
        convert_proto_role_to_memory_role(2).unwrap().as_ref(),
        "assistant"
    );
    assert_eq!(
        convert_proto_role_to_memory_role(3)
            .unwrap_err()
            .to_string(),
        "invalid message role: MESSAGE_ROLE_SYSTEM"
    );
}

#[test]
fn proto_content_is_text_only_with_go_nil_and_empty_semantics() {
    assert!(convert_proto_content_to_memory(&[]).is_none());
    let parts = vec![
        pb::MessagePart::default(),
        pb::MessagePart {
            data: Some(pb::message_part::Data::Error(pb::message_part::Error {
                message: "error".into(),
            })),
        },
    ];
    assert!(convert_proto_content_to_memory(&parts)
        .unwrap()
        .blocks
        .unwrap()
        .is_empty());
    let mut parts = parts;
    parts.push(text("hello"));
    let blocks = convert_proto_content_to_memory(&parts)
        .unwrap()
        .blocks
        .unwrap();
    assert_eq!(blocks.len(), 1);
    assert_eq!(blocks[0].kind.as_ref(), "text");
    assert_eq!(blocks[0].payload, "hello");
}

#[test]
fn final_response_requires_assistant_nonempty_blocks_and_no_tool_calls() {
    for (source, blocks, expected) in [
        ("assistant", vec![block("text", "done")], true),
        ("assistant", vec![block("text", "")], true),
        ("assistant", vec![], false),
        ("user", vec![block("text", "done")], false),
        ("system", vec![block("text", "done")], false),
        (
            "assistant",
            vec![block("text", "thinking"), block("tool_call", "{}")],
            false,
        ),
        ("assistant", vec![block("tool_result", "{}")], true),
        ("assistant", vec![block("unknown", "")], true),
    ] {
        assert_eq!(
            convert_memory_message_to_proto(Some(&message(source, blocks)))
                .unwrap()
                .status
                .unwrap()
                .is_final_response,
            expected
        );
    }
    let empty = db::Message {
        source: "assistant".into(),
        ..Default::default()
    };
    assert!(
        !convert_memory_message_to_proto(Some(&empty))
            .unwrap()
            .status
            .unwrap()
            .is_final_response
    );
}

#[test]
fn malformed_tool_blocks_fail_and_unknown_blocks_are_ignored() {
    for kind in ["tool_call", "tool_result"] {
        assert!(convert_memory_message_to_proto(Some(&message(
            "assistant",
            vec![block(kind, "{")]
        )))
        .is_err());
        assert!(convert_memory_message_to_proto(Some(&message(
            "assistant",
            vec![block(kind, "{}")]
        )))
        .unwrap()
        .spec
        .unwrap()
        .content
        .is_empty());
    }
    assert!(convert_memory_message_to_proto(Some(&message(
        "user",
        vec![block("unknown", "arbitrary")]
    )))
    .unwrap()
    .spec
    .unwrap()
    .content
    .is_empty());
}

#[test]
fn message_uuid_error_fields_are_identified() {
    let proto = convert_memory_message_to_proto(Some(&message("user", vec![]))).unwrap();
    for field in ["id", "task", "agent", "model"] {
        let mut p = proto.clone();
        let m = p.metadata.as_mut().unwrap();
        match field {
            "id" => m.id = "bad".into(),
            "task" => m.task_id = "bad".into(),
            "agent" => m.agent_id = Some("bad".into()),
            _ => m.model_id = Some("bad".into()),
        }
        assert!(convert_proto_message_to_memory(Some(&p))
            .unwrap_err()
            .to_string()
            .starts_with("invalid "));
    }
}

#[test]
fn interpreter_call_and_result_expand_function_calls_in_order() {
    let call = json!({"tool":"code_interpreter", "input":{"interpreter":{"script":"run()"}}});
    let result = json!({"tool":"code_interpreter", "output":{"interpreter":{"console_output":"done", "function_calls":[
        {"tool_name":"create_file", "input":{"create_file":{"Path":"file", "Content":"hello"}}, "output":{"create_file":{"overwritten":true}}},
        {"tool_name":"unknown", "input":{}, "output":{}},
        {"tool_name":"read_file", "input":{}, "output":{"read_file":{"content":"skip"}}}
    ]}}});
    let parts = convert_memory_message_to_proto(Some(&message(
        "assistant",
        vec![
            block("tool_call", &call.to_string()),
            block("tool_result", &result.to_string()),
        ],
    )))
    .unwrap()
    .spec
    .unwrap()
    .content;
    assert_eq!(parts.len(), 4);
    assert!(
        matches!(&parts[0].data, Some(pb::message_part::Data::ToolCall(pb::ToolCall { input: Some(pb::tool_call::Input::CodeInterpreter(i)), .. })) if i.code == "run()")
    );
    assert!(
        matches!(&parts[1].data, Some(pb::message_part::Data::ToolResult(pb::ToolResult { result: Some(pb::tool_result::Result::CodeInterpreter(o)), .. })) if o.output == "done")
    );
    assert!(
        matches!(&parts[2].data, Some(pb::message_part::Data::ToolCall(pb::ToolCall { input: Some(pb::tool_call::Input::CreateFile(i)), .. })) if i.path == "file" && i.content == "hello")
    );
    assert!(
        matches!(&parts[3].data, Some(pb::message_part::Data::ToolResult(pb::ToolResult { result: Some(pb::tool_result::Result::CreateFile(o)), .. })) if o.overwritten)
    );
}

#[test]
fn tool_event_conversions_cover_every_input_and_output_variant() {
    let cases = [
        (
            "create_file",
            json!({"Path":"p", "Content":"c"}),
            json!({"overwritten":true}),
            "createFile",
        ),
        (
            "edit_file",
            json!({"path":"p", "diffs":[{"old":"a", "new":"b"}]}),
            json!({"path":"p", "patch_info":{"patch":"patch", "lines_added":2, "lines_removed":1}}),
            "editFile",
        ),
        (
            "execute_command",
            json!({"Command":"pwd"}),
            json!({"stdout":"out", "stderr":"err", "exit_code":1, "command":"pwd"}),
            "executeCommand",
        ),
        (
            "find_file",
            json!({"pattern":"*.rs", "path":"p", "exclude_pattern":"target", "max_results":3}),
            json!({"files":["a"], "total_files":10, "truncated_count":9}),
            "findFile",
        ),
        (
            "grep",
            json!({"query":"q", "path":"p", "include_pattern":"*.rs", "exclude_pattern":"target", "case_sensitive":true, "max_results":5}),
            json!({"matches":[{"file_path":"p", "value":"q"}], "total_matches":8, "truncated_matches":4, "searched_files":2}),
            "grep",
        ),
        (
            "list_files",
            json!({"Path":"p", "Recursive":true}),
            json!({"path":"p", "entries":[{"n":"file", "t":"file", "s":12}]}),
            "listFiles",
        ),
        (
            "read_file",
            json!({"path":"p", "start_line":2, "end_line":5}),
            json!({"path":"p", "content":"c"}),
            "readFile",
        ),
        (
            "submit_report",
            json!({"Summary":"s", "Completed":true, "Deliverables":["d"], "NextSteps":"n"}),
            json!({"summary":"s", "completed":true, "deliverables":["d"], "next_steps":"n"}),
            "submitReport",
        ),
        (
            "ask_user",
            json!({"question":"q", "options":["a"]}),
            json!({"user_response":"a"}),
            "askUser",
        ),
        (
            "handoff",
            json!({"RequestedAgent":"a", "HandoverMessage":"h"}),
            json!({}),
            "handoff",
        ),
        (
            "fetch",
            json!({"url":"https://example.com", "headers":{"accept":"text/plain"}, "timeout":4}),
            json!({"url":"u", "title":"t", "content":"c", "content_type":"text/plain", "byte_size":1, "truncated":true}),
            "fetch",
        ),
    ];
    for (name, input, output, key) in cases {
        let i: tools::ToolInput = serde_json::from_value(json!({name: input.clone()})).unwrap();
        let o: tools::ToolOutput = serde_json::from_value(json!({name: output.clone()})).unwrap();
        let call = convert_tool_input_to_proto("id", name, Some(&i));
        let result = convert_tool_output_to_proto("id", name, Some(&o));
        assert_eq!(call.id, "id");
        assert!(
            serde_json::to_value(&call).unwrap().get(key).is_some(),
            "{name}"
        );
        assert_eq!(result.id, "id");
        if name == "ask_user" || name == "handoff" {
            assert!(result.result.is_none());
        } else {
            assert!(
                serde_json::to_value(&result).unwrap().get(key).is_some(),
                "{name}"
            );
        }
        let persisted = json!({"tool":"code_interpreter", "output":{"interpreter":{"function_calls":[{"tool_name": name, "input": {name: input}, "output":{name:output}}]}}});
        let parts = convert_memory_message_to_proto(Some(&message(
            "assistant",
            vec![block("tool_result", &persisted.to_string())],
        )))
        .unwrap()
        .spec
        .unwrap()
        .content;
        assert_eq!(
            parts.len(),
            if ["ask_user", "handoff"].contains(&name) {
                2
            } else {
                3
            },
            "{name}"
        );
        assert!(
            serde_json::to_value(&parts[1]).unwrap()["toolCall"]
                .get(key)
                .is_some(),
            "{name}"
        );
    }
    assert!(convert_tool_input_to_proto("id", "unknown", None)
        .input
        .is_none());
    assert!(convert_tool_output_to_proto("id", "unknown", None)
        .result
        .is_none());
    assert!(
        convert_tool_input_to_proto("", "", Some(&tools::ToolInput::default()))
            .input
            .is_none()
    );
    assert!(
        convert_tool_output_to_proto("", "", Some(&tools::ToolOutput::default()))
            .result
            .is_none()
    );
}
