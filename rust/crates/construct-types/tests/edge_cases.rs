use construct_types::{memory::*, provider, to_go_json, tools::*, Timestamp};

#[test]
fn nil_empty_and_omitted_values_are_distinct() {
    assert_eq!(
        to_go_json(&MessageContent::default()).unwrap(),
        br#"{"blocks":null}"#
    );
    assert_eq!(
        to_go_json(&MessageContent {
            blocks: Some(vec![])
        })
        .unwrap(),
        br#"{"blocks":[]}"#
    );
    assert_eq!(
        to_go_json(&TaskStatus {
            turns: 0,
            tool_uses: Some(Default::default())
        })
        .unwrap(),
        b"{}"
    );
    assert_eq!(
        to_go_json(&AskUserInput {
            question: String::new(),
            options: Some(vec![])
        })
        .unwrap(),
        br#"{"question":""}"#
    );
    assert_eq!(
        to_go_json(&ReadFileInput {
            path: String::new(),
            start_line: Some(Box::new(0)),
            end_line: None
        })
        .unwrap(),
        br#"{"path":"","start_line":0}"#
    );
    let result = to_go_json(&EditFileResult::default()).unwrap();
    assert!(String::from_utf8(result)
        .unwrap()
        .contains(r#""patch_info":{"patch":"","lines_added":0,"lines_removed":0}"#));
    assert_eq!(to_go_json(&ToolInput::default()).unwrap(), b"{}");
    assert_eq!(to_go_json(&ToolOutput::default()).unwrap(), b"{}");
}

#[test]
fn unknown_strings_and_invalid_input() {
    let phase: TaskPhase = serde_json::from_str(r#""future_phase""#).unwrap();
    assert_eq!(to_go_json(&phase).unwrap(), br#""future_phase""#);
    assert!(serde_json::from_str::<MessageContent>("{").is_err());
    assert!(serde_json::from_str::<MessageUsage>(r#"{"cost":"NaN"}"#).is_err());
    assert!(serde_json::from_str::<Timestamp>(r#""not a time""#).is_err());
    assert!(serde_json::from_str::<Timestamp>(r#""2016-12-31T23:59:60Z""#).is_err());
    assert!(serde_json::from_str::<provider::ContentBlock>(r#"{"unexpected":1}"#).is_err());
    assert!(serde_json::value::RawValue::from_string("{".into()).is_err());
    for cost in [f64::NAN, f64::INFINITY, f64::NEG_INFINITY] {
        assert!(to_go_json(&MessageUsage {
            cost,
            ..Default::default()
        })
        .is_err());
    }
    let outside = Timestamp(
        chrono::NaiveDate::from_ymd_opt(10000, 1, 1)
            .unwrap()
            .and_hms_opt(0, 0, 0)
            .unwrap()
            .and_utc(),
    );
    assert!(to_go_json(&outside).is_err());
}

#[test]
fn uuid_and_utc_record_fields() {
    let id = uuid::Uuid::parse_str("12345678-1234-5678-9abc-123456789abc").unwrap();
    let message = construct_types::entity::Message {
        id,
        task_id: id,
        content: Some(MessageContent::default()),
        ..Default::default()
    };
    assert_eq!(message.id, id);
    assert_eq!(message.task_id, id);
    assert_eq!(
        to_go_json(&message.create_time).unwrap(),
        br#""0001-01-01T00:00:00Z""#
    );
    assert!(message.agent_id.is_none());
    let text = provider::ContentBlock::Text(provider::TextBlock::default());
    assert_eq!(text.block_type().unwrap().as_ref(), "text");
    assert!(provider::ContentBlock::Null.block_type().is_none());
}

#[test]
fn raw_tool_arguments_keep_order_and_number_spelling() {
    let raw = r#"{"id":"c","tool":"t","args":{ "z":1.0, "a":1e999 }}"#;
    let block: provider::ContentBlock = serde_json::from_str(raw).unwrap();
    let provider::ContentBlock::ToolCall(call) = &block else {
        panic!("expected tool call")
    };
    assert_eq!(
        call.args.as_ref().unwrap().get(),
        r#"{ "z":1.0, "a":1e999 }"#
    );
    assert_eq!(
        to_go_json(&block).unwrap(),
        br#"{"id":"c","tool":"t","args":{"z":1.0,"a":1e999}}"#
    );
    let nil: provider::ContentBlock = serde_json::from_str("null").unwrap();
    assert_eq!(to_go_json(&nil).unwrap(), b"null");
}
