use construct_types::{memory, provider, skill, to_go_json, tools, Timestamp};
use serde::{Deserialize, Serialize};
use serde_json::value::RawValue;
use std::{collections::BTreeMap, fs, path::Path};
use uuid::Uuid;

const SPECIAL: &str = "<>&\u{2028}\u{2029}\"\\\n\t é 🚀";

#[test]
fn floats_times_and_raw_content_go_goldens() {
    fixture(
        "floats",
        &vec![
            0.0_f64,
            -0.0,
            1.0,
            -1.0,
            1.25,
            1e-7,
            1e-6,
            1e20,
            1e21,
            1.2345678901234567e25,
            f64::from_bits(1),
            f64::MAX,
        ],
    );
    let times = [
        "0001-01-01T00:00:00Z",
        "2025-01-01T00:00:00.1Z",
        "2025-01-01T00:00:00.1234Z",
        "2025-01-01T00:00:00.123456789Z",
    ]
    .map(|s| Timestamp(chrono::DateTime::parse_from_rfc3339(s).unwrap().to_utc()));
    fixture("times", &times);
    fixture(
        "content-blocks",
        &vec![
            provider::ContentBlock::Text(provider::TextBlock {
                text: SPECIAL.into(),
            }),
            provider::ContentBlock::ToolCall(provider::ToolCallBlock {
                id: "call".into(),
                tool: "read".into(),
                args: Some(RawValue::from_string(r#"{ "z": "<", "a": 1.0 }"#.into()).unwrap()),
            }),
            provider::ContentBlock::ToolResult(provider::ToolResultBlock {
                id: "call".into(),
                name: "read".into(),
                result: SPECIAL.into(),
                succeeded: true,
            }),
        ],
    );
}

#[test]
fn provider_error_go_policy() {
    #[derive(Serialize, Deserialize)]
    #[serde(rename_all = "PascalCase")]
    struct Case {
        kind: provider::ProviderErrorKind,
        retry_after: i64,
        message: String,
        retryable: bool,
        delay: i64,
        error: String,
    }
    let cases: Vec<Case> =
        serde_json::from_str(include_str!("fixtures/provider-errors.json")).unwrap();
    let mut constructed = Vec::new();
    for case in cases {
        let error = provider::ProviderError {
            provider: "test".into(),
            kind: case.kind,
            retry_after: case.retry_after,
            err: Some("underlying".into()),
        };
        assert_eq!(error.retryable(), (case.retryable, case.delay));
        assert_eq!(error.message(), case.message);
        assert_eq!(error.to_string(), case.error);
        constructed.push(Case {
            kind: error.kind.clone(),
            retry_after: error.retry_after,
            message: error.message(),
            retryable: error.retryable().0,
            delay: error.retryable().1,
            error: error.to_string(),
        });
    }
    fixture("provider-errors", &constructed);
}
trait Sample: Default {
    fn sample(depth: usize, special: bool) -> Self;
    fn empty() -> Self {
        Self::default()
    }
}
impl Sample for String {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            Self::default()
        } else {
            if s { SPECIAL } else { "value" }.into()
        }
    }
}
impl Sample for i64 {
    fn sample(d: usize, _: bool) -> Self {
        if d > 5 {
            0
        } else {
            7
        }
    }
}
impl Sample for f64 {
    fn sample(d: usize, _: bool) -> Self {
        if d > 5 {
            0.0
        } else {
            1.25
        }
    }
}
impl Sample for bool {
    fn sample(d: usize, _: bool) -> Self {
        d <= 5
    }
}
impl Sample for Timestamp {
    fn sample(d: usize, _: bool) -> Self {
        if d > 5 {
            Self::default()
        } else {
            Self(
                chrono::DateTime::parse_from_rfc3339("2025-02-03T04:05:06.1234Z")
                    .unwrap()
                    .to_utc(),
            )
        }
    }
}
impl Sample for Uuid {
    fn sample(d: usize, _: bool) -> Self {
        if d > 5 {
            Self::default()
        } else {
            Self::parse_str("12345678-1234-5678-9abc-123456789abc").unwrap()
        }
    }
}
impl<T: Sample> Sample for Option<Box<T>> {
    fn sample(d: usize, s: bool) -> Self {
        (d <= 5).then(|| Box::new(T::sample(d + 1, s)))
    }
}
impl<T: Sample> Sample for Option<Vec<T>> {
    fn sample(d: usize, s: bool) -> Self {
        (d <= 5).then(|| vec![T::sample(d + 1, s)])
    }
    fn empty() -> Self {
        Some(Vec::new())
    }
}
impl<T: Sample> Sample for Option<BTreeMap<String, T>> {
    fn sample(d: usize, s: bool) -> Self {
        (d <= 5).then(|| {
            let keys = if s { ["z&", "a<"] } else { ["z", "a"] };
            keys.into_iter()
                .map(|k| (k.into(), T::sample(d + 1, s)))
                .collect()
        })
    }
    fn empty() -> Self {
        Some(BTreeMap::new())
    }
}
impl Sample for Option<Box<RawValue>> {
    fn sample(d: usize, _: bool) -> Self {
        (d <= 5)
            .then(|| RawValue::from_string(r#"{ "z" : "<>&", "a" : [1.0, true] }"#.into()).unwrap())
    }
}

fn fixture<T: Serialize + for<'de> Deserialize<'de>>(name: &str, constructed: &T) {
    let path = Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("tests/fixtures")
        .join(format!("{name}.json"));
    let bytes = fs::read(path).unwrap();
    let decoded: T = serde_json::from_slice(&bytes).unwrap();
    assert_eq!(
        String::from_utf8(to_go_json(&decoded).unwrap()).unwrap(),
        String::from_utf8(bytes.clone()).unwrap(),
        "{name} round trip"
    );
    assert_eq!(
        String::from_utf8(to_go_json(constructed).unwrap()).unwrap(),
        String::from_utf8(bytes).unwrap(),
        "{name} Rust construction"
    );
}

#[derive(Debug, Default, Serialize, Deserialize)]
struct MemoryFixture {
    #[serde(rename = "TaskSpec")]
    task_spec: memory::TaskSpec,
    #[serde(rename = "TaskStatus")]
    task_status: memory::TaskStatus,
    #[serde(rename = "TaskUsage")]
    task_usage: memory::TaskUsage,
    #[serde(rename = "MessageContent")]
    message_content: memory::MessageContent,
    #[serde(rename = "MessageBlock")]
    message_block: memory::MessageBlock,
    #[serde(rename = "MessageUsage")]
    message_usage: memory::MessageUsage,
    #[serde(rename = "Decimal")]
    decimal: memory::Decimal,
    #[serde(rename = "MessageBlockKind")]
    message_block_kind: memory::MessageBlockKind,
    #[serde(rename = "MessageSource")]
    message_source: memory::MessageSource,
    #[serde(rename = "ModelCapability")]
    model_capability: memory::ModelCapability,
    #[serde(rename = "ModelProviderType")]
    model_provider_type: memory::ModelProviderType,
    #[serde(rename = "TaskPhase")]
    task_phase: memory::TaskPhase,
    #[serde(rename = "TokenType")]
    token_type: memory::TokenType,
}
impl Sample for memory::TaskSpec {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            workspace: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            workspace: <String>::empty(),
        }
    }
}
impl Sample for memory::TaskStatus {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            turns: <i64>::sample(d + 1, s),
            tool_uses: <Option<BTreeMap<String, i64>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            turns: <i64>::empty(),
            tool_uses: <Option<BTreeMap<String, i64>>>::empty(),
        }
    }
}
impl Sample for memory::TaskUsage {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            input_tokens: <i64>::sample(d + 1, s),
            output_tokens: <i64>::sample(d + 1, s),
            cache_write_tokens: <i64>::sample(d + 1, s),
            cache_read_tokens: <i64>::sample(d + 1, s),
            cost: <f64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            input_tokens: <i64>::empty(),
            output_tokens: <i64>::empty(),
            cache_write_tokens: <i64>::empty(),
            cache_read_tokens: <i64>::empty(),
            cost: <f64>::empty(),
        }
    }
}
impl Sample for memory::MessageContent {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            blocks: <Option<Vec<memory::MessageBlock>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            blocks: <Option<Vec<memory::MessageBlock>>>::empty(),
        }
    }
}
impl Sample for memory::MessageBlock {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            kind: <memory::MessageBlockKind>::sample(d + 1, s),
            payload: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            kind: <memory::MessageBlockKind>::empty(),
            payload: <String>::empty(),
        }
    }
}
impl Sample for memory::MessageUsage {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            input_tokens: <i64>::sample(d + 1, s),
            output_tokens: <i64>::sample(d + 1, s),
            cache_write_tokens: <i64>::sample(d + 1, s),
            cache_read_tokens: <i64>::sample(d + 1, s),
            cost: <f64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            input_tokens: <i64>::empty(),
            output_tokens: <i64>::empty(),
            cache_write_tokens: <i64>::empty(),
            cache_read_tokens: <i64>::empty(),
            cost: <f64>::empty(),
        }
    }
}
impl Sample for memory::Decimal {
    fn sample(d: usize, _: bool) -> Self {
        if d > 5 {
            Self::default()
        } else {
            Self("-123456789012345678901234567890.1234".into())
        }
    }
}
impl Sample for memory::MessageBlockKind {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for memory::MessageSource {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for memory::ModelCapability {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for memory::ModelProviderType {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for memory::TaskPhase {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for memory::TokenType {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for MemoryFixture {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            task_spec: memory::TaskSpec::sample(d + 1, s),
            task_status: memory::TaskStatus::sample(d + 1, s),
            task_usage: memory::TaskUsage::sample(d + 1, s),
            message_content: memory::MessageContent::sample(d + 1, s),
            message_block: memory::MessageBlock::sample(d + 1, s),
            message_usage: memory::MessageUsage::sample(d + 1, s),
            decimal: memory::Decimal::sample(d + 1, s),
            message_block_kind: memory::MessageBlockKind::sample(d + 1, s),
            message_source: memory::MessageSource::sample(d + 1, s),
            model_capability: memory::ModelCapability::sample(d + 1, s),
            model_provider_type: memory::ModelProviderType::sample(d + 1, s),
            task_phase: memory::TaskPhase::sample(d + 1, s),
            token_type: memory::TokenType::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            task_spec: memory::TaskSpec::empty(),
            task_status: memory::TaskStatus::empty(),
            task_usage: memory::TaskUsage::empty(),
            message_content: memory::MessageContent::empty(),
            message_block: memory::MessageBlock::empty(),
            message_usage: memory::MessageUsage::empty(),
            decimal: memory::Decimal::empty(),
            message_block_kind: memory::MessageBlockKind::empty(),
            message_source: memory::MessageSource::empty(),
            model_capability: memory::ModelCapability::empty(),
            model_provider_type: memory::ModelProviderType::empty(),
            task_phase: memory::TaskPhase::empty(),
            token_type: memory::TokenType::empty(),
        }
    }
}
#[test]
fn memory_go_goldens() {
    for mode in ["zero", "empty", "full", "special"] {
        let v = if mode == "zero" {
            MemoryFixture::default()
        } else if mode == "empty" {
            MemoryFixture::empty()
        } else {
            MemoryFixture::sample(0, mode == "special")
        };
        fixture(&format!("memory-{mode}"), &v);
    }
}

#[derive(Debug, Default, Serialize, Deserialize)]
struct ToolsFixture {
    #[serde(rename = "CreateFileInput")]
    create_file_input: tools::CreateFileInput,
    #[serde(rename = "CreateFileResult")]
    create_file_result: tools::CreateFileResult,
    #[serde(rename = "EditFileInput")]
    edit_file_input: tools::EditFileInput,
    #[serde(rename = "DiffPair")]
    diff_pair: tools::DiffPair,
    #[serde(rename = "DiffValidationError")]
    diff_validation_error: tools::DiffValidationError,
    #[serde(rename = "ConflictWarning")]
    conflict_warning: tools::ConflictWarning,
    #[serde(rename = "PatchInfo")]
    patch_info: tools::PatchInfo,
    #[serde(rename = "EditFileResult")]
    edit_file_result: tools::EditFileResult,
    #[serde(rename = "FindFileInput")]
    find_file_input: tools::FindFileInput,
    #[serde(rename = "FindFileResult")]
    find_file_result: tools::FindFileResult,
    #[serde(rename = "GrepInput")]
    grep_input: tools::GrepInput,
    #[serde(rename = "GrepMatch")]
    grep_match: tools::GrepMatch,
    #[serde(rename = "GrepResult")]
    grep_result: tools::GrepResult,
    #[serde(rename = "ListFilesInput")]
    list_files_input: tools::ListFilesInput,
    #[serde(rename = "ListFilesResult")]
    list_files_result: tools::ListFilesResult,
    #[serde(rename = "DirectoryEntry")]
    directory_entry: tools::DirectoryEntry,
    #[serde(rename = "ReadFileInput")]
    read_file_input: tools::ReadFileInput,
    #[serde(rename = "ReadFileResult")]
    read_file_result: tools::ReadFileResult,
    #[serde(rename = "ExecuteCommandInput")]
    execute_command_input: tools::ExecuteCommandInput,
    #[serde(rename = "ExecuteCommandResult")]
    execute_command_result: tools::ExecuteCommandResult,
    #[serde(rename = "AskUserInput")]
    ask_user_input: tools::AskUserInput,
    #[serde(rename = "AskUserResult")]
    ask_user_result: tools::AskUserResult,
    #[serde(rename = "SubmitReportInput")]
    submit_report_input: tools::SubmitReportInput,
    #[serde(rename = "SubmitReportResult")]
    submit_report_result: tools::SubmitReportResult,
    #[serde(rename = "HandoffInput")]
    handoff_input: tools::HandoffInput,
    #[serde(rename = "FetchInput")]
    fetch_input: tools::FetchInput,
    #[serde(rename = "FetchResult")]
    fetch_result: tools::FetchResult,
    #[serde(rename = "InterpreterInput")]
    interpreter_input: tools::InterpreterInput,
    #[serde(rename = "InterpreterOutput")]
    interpreter_output: tools::InterpreterOutput,
    #[serde(rename = "FunctionCall")]
    function_call: tools::FunctionCall,
    #[serde(rename = "ToolInput")]
    tool_input: tools::ToolInput,
    #[serde(rename = "ToolOutput")]
    tool_output: tools::ToolOutput,
    #[serde(rename = "ToolCallEvent")]
    tool_call_event: tools::ToolCallEvent,
    #[serde(rename = "ToolResultEvent")]
    tool_result_event: tools::ToolResultEvent,
    #[serde(rename = "ProviderData")]
    provider_data: tools::ProviderData,
    #[serde(rename = "ToolCall")]
    tool_call: tools::ToolCall,
    #[serde(rename = "ToolResult")]
    tool_result: tools::ToolResult,
    #[serde(rename = "InterpreterToolResult")]
    interpreter_tool_result: tools::InterpreterToolResult,
}
impl Sample for tools::CreateFileInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            path: <String>::sample(d + 1, s),
            content: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            path: <String>::empty(),
            content: <String>::empty(),
        }
    }
}
impl Sample for tools::CreateFileResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            overwritten: <bool>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            overwritten: <bool>::empty(),
        }
    }
}
impl Sample for tools::EditFileInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            path: <String>::sample(d + 1, s),
            diffs: <Option<Vec<tools::DiffPair>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            path: <String>::empty(),
            diffs: <Option<Vec<tools::DiffPair>>>::empty(),
        }
    }
}
impl Sample for tools::DiffPair {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            old: <String>::sample(d + 1, s),
            new: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            old: <String>::empty(),
            new: <String>::empty(),
        }
    }
}
impl Sample for tools::DiffValidationError {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            diff_index: <i64>::sample(d + 1, s),
            error_type: <String>::sample(d + 1, s),
            error_message: <String>::sample(d + 1, s),
            suggested_fix: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            diff_index: <i64>::empty(),
            error_type: <String>::empty(),
            error_message: <String>::empty(),
            suggested_fix: <String>::empty(),
        }
    }
}
impl Sample for tools::ConflictWarning {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            edit1_index: <i64>::sample(d + 1, s),
            edit2_index: <i64>::sample(d + 1, s),
            conflict_type: <String>::sample(d + 1, s),
            message: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            edit1_index: <i64>::empty(),
            edit2_index: <i64>::empty(),
            conflict_type: <String>::empty(),
            message: <String>::empty(),
        }
    }
}
impl Sample for tools::PatchInfo {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            patch: <String>::sample(d + 1, s),
            lines_added: <i64>::sample(d + 1, s),
            lines_removed: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            patch: <String>::empty(),
            lines_added: <i64>::empty(),
            lines_removed: <i64>::empty(),
        }
    }
}
impl Sample for tools::EditFileResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            success: <bool>::sample(d + 1, s),
            path: <String>::sample(d + 1, s),
            replacements_made: <i64>::sample(d + 1, s),
            expected_replacements: <i64>::sample(d + 1, s),
            failure_reason: <String>::sample(d + 1, s),
            validation_errors: <Option<Vec<tools::DiffValidationError>>>::sample(d + 1, s),
            conflict_warnings: <Option<Vec<tools::ConflictWarning>>>::sample(d + 1, s),
            patch_info: <tools::PatchInfo>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            success: <bool>::empty(),
            path: <String>::empty(),
            replacements_made: <i64>::empty(),
            expected_replacements: <i64>::empty(),
            failure_reason: <String>::empty(),
            validation_errors: <Option<Vec<tools::DiffValidationError>>>::empty(),
            conflict_warnings: <Option<Vec<tools::ConflictWarning>>>::empty(),
            patch_info: <tools::PatchInfo>::empty(),
        }
    }
}
impl Sample for tools::FindFileInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            pattern: <String>::sample(d + 1, s),
            path: <String>::sample(d + 1, s),
            exclude_pattern: <String>::sample(d + 1, s),
            max_results: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            pattern: <String>::empty(),
            path: <String>::empty(),
            exclude_pattern: <String>::empty(),
            max_results: <i64>::empty(),
        }
    }
}
impl Sample for tools::FindFileResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            files: <Option<Vec<String>>>::sample(d + 1, s),
            total_files: <i64>::sample(d + 1, s),
            truncated_count: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            files: <Option<Vec<String>>>::empty(),
            total_files: <i64>::empty(),
            truncated_count: <i64>::empty(),
        }
    }
}
impl Sample for tools::GrepInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            query: <String>::sample(d + 1, s),
            path: <String>::sample(d + 1, s),
            include_pattern: <String>::sample(d + 1, s),
            exclude_pattern: <String>::sample(d + 1, s),
            case_sensitive: <bool>::sample(d + 1, s),
            max_results: <i64>::sample(d + 1, s),
            context: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            query: <String>::empty(),
            path: <String>::empty(),
            include_pattern: <String>::empty(),
            exclude_pattern: <String>::empty(),
            case_sensitive: <bool>::empty(),
            max_results: <i64>::empty(),
            context: <i64>::empty(),
        }
    }
}
impl Sample for tools::GrepMatch {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            file_path: <String>::sample(d + 1, s),
            value: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            file_path: <String>::empty(),
            value: <String>::empty(),
        }
    }
}
impl Sample for tools::GrepResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            matches: <Option<Vec<tools::GrepMatch>>>::sample(d + 1, s),
            total_matches: <i64>::sample(d + 1, s),
            truncated_matches: <i64>::sample(d + 1, s),
            searched_files: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            matches: <Option<Vec<tools::GrepMatch>>>::empty(),
            total_matches: <i64>::empty(),
            truncated_matches: <i64>::empty(),
            searched_files: <i64>::empty(),
        }
    }
}
impl Sample for tools::ListFilesInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            path: <String>::sample(d + 1, s),
            recursive: <bool>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            path: <String>::empty(),
            recursive: <bool>::empty(),
        }
    }
}
impl Sample for tools::ListFilesResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            path: <String>::sample(d + 1, s),
            entries: <Option<Vec<tools::DirectoryEntry>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            path: <String>::empty(),
            entries: <Option<Vec<tools::DirectoryEntry>>>::empty(),
        }
    }
}
impl Sample for tools::DirectoryEntry {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            name: <String>::sample(d + 1, s),
            r#type: <String>::sample(d + 1, s),
            size: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            name: <String>::empty(),
            r#type: <String>::empty(),
            size: <i64>::empty(),
        }
    }
}
impl Sample for tools::ReadFileInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            path: <String>::sample(d + 1, s),
            start_line: <Option<Box<i64>>>::sample(d + 1, s),
            end_line: <Option<Box<i64>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            path: <String>::empty(),
            start_line: <Option<Box<i64>>>::empty(),
            end_line: <Option<Box<i64>>>::empty(),
        }
    }
}
impl Sample for tools::ReadFileResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            path: <String>::sample(d + 1, s),
            content: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            path: <String>::empty(),
            content: <String>::empty(),
        }
    }
}
impl Sample for tools::ExecuteCommandInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            command: <String>::sample(d + 1, s),
            working_directory: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            command: <String>::empty(),
            working_directory: <String>::empty(),
        }
    }
}
impl Sample for tools::ExecuteCommandResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            stdout: <String>::sample(d + 1, s),
            stderr: <String>::sample(d + 1, s),
            exit_code: <i64>::sample(d + 1, s),
            command: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            stdout: <String>::empty(),
            stderr: <String>::empty(),
            exit_code: <i64>::empty(),
            command: <String>::empty(),
        }
    }
}
impl Sample for tools::AskUserInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            question: <String>::sample(d + 1, s),
            options: <Option<Vec<String>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            question: <String>::empty(),
            options: <Option<Vec<String>>>::empty(),
        }
    }
}
impl Sample for tools::AskUserResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            user_response: <String>::sample(d + 1, s),
            selected_option: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            user_response: <String>::empty(),
            selected_option: <String>::empty(),
        }
    }
}
impl Sample for tools::SubmitReportInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            summary: <String>::sample(d + 1, s),
            completed: <bool>::sample(d + 1, s),
            deliverables: <Option<Vec<String>>>::sample(d + 1, s),
            next_steps: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            summary: <String>::empty(),
            completed: <bool>::empty(),
            deliverables: <Option<Vec<String>>>::empty(),
            next_steps: <String>::empty(),
        }
    }
}
impl Sample for tools::SubmitReportResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            summary: <String>::sample(d + 1, s),
            completed: <bool>::sample(d + 1, s),
            deliverables: <Option<Vec<String>>>::sample(d + 1, s),
            next_steps: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            summary: <String>::empty(),
            completed: <bool>::empty(),
            deliverables: <Option<Vec<String>>>::empty(),
            next_steps: <String>::empty(),
        }
    }
}
impl Sample for tools::HandoffInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            task_id: <Uuid>::sample(d + 1, s),
            current_agent_id: <Uuid>::sample(d + 1, s),
            requested_agent: <String>::sample(d + 1, s),
            handover_message: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            task_id: <Uuid>::empty(),
            current_agent_id: <Uuid>::empty(),
            requested_agent: <String>::empty(),
            handover_message: <String>::empty(),
        }
    }
}
impl Sample for tools::FetchInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            url: <String>::sample(d + 1, s),
            headers: <Option<BTreeMap<String, String>>>::sample(d + 1, s),
            timeout: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            url: <String>::empty(),
            headers: <Option<BTreeMap<String, String>>>::empty(),
            timeout: <i64>::empty(),
        }
    }
}
impl Sample for tools::FetchResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            url: <String>::sample(d + 1, s),
            title: <String>::sample(d + 1, s),
            content: <String>::sample(d + 1, s),
            content_type: <String>::sample(d + 1, s),
            byte_size: <i64>::sample(d + 1, s),
            truncated: <bool>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            url: <String>::empty(),
            title: <String>::empty(),
            content: <String>::empty(),
            content_type: <String>::empty(),
            byte_size: <i64>::empty(),
            truncated: <bool>::empty(),
        }
    }
}
impl Sample for tools::InterpreterInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            script: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            script: <String>::empty(),
        }
    }
}
impl Sample for tools::InterpreterOutput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            console_output: <String>::sample(d + 1, s),
            function_calls: <Option<Vec<tools::FunctionCall>>>::sample(d + 1, s),
            tool_stats: <Option<BTreeMap<String, i64>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            console_output: <String>::empty(),
            function_calls: <Option<Vec<tools::FunctionCall>>>::empty(),
            tool_stats: <Option<BTreeMap<String, i64>>>::empty(),
        }
    }
}
impl Sample for tools::FunctionCall {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            tool_name: <String>::sample(d + 1, s),
            input: <tools::ToolInput>::sample(d + 1, s),
            output: <tools::ToolOutput>::sample(d + 1, s),
            index: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            tool_name: <String>::empty(),
            input: <tools::ToolInput>::empty(),
            output: <tools::ToolOutput>::empty(),
            index: <i64>::empty(),
        }
    }
}
impl Sample for tools::ToolInput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            create_file: <Option<Box<tools::CreateFileInput>>>::sample(d + 1, s),
            edit_file: <Option<Box<tools::EditFileInput>>>::sample(d + 1, s),
            execute_command: <Option<Box<tools::ExecuteCommandInput>>>::sample(d + 1, s),
            find_file: <Option<Box<tools::FindFileInput>>>::sample(d + 1, s),
            grep: <Option<Box<tools::GrepInput>>>::sample(d + 1, s),
            list_files: <Option<Box<tools::ListFilesInput>>>::sample(d + 1, s),
            read_file: <Option<Box<tools::ReadFileInput>>>::sample(d + 1, s),
            submit_report: <Option<Box<tools::SubmitReportInput>>>::sample(d + 1, s),
            ask_user: <Option<Box<tools::AskUserInput>>>::sample(d + 1, s),
            handoff: <Option<Box<tools::HandoffInput>>>::sample(d + 1, s),
            fetch: <Option<Box<tools::FetchInput>>>::sample(d + 1, s),
            interpreter: <Option<Box<tools::InterpreterInput>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            create_file: <Option<Box<tools::CreateFileInput>>>::empty(),
            edit_file: <Option<Box<tools::EditFileInput>>>::empty(),
            execute_command: <Option<Box<tools::ExecuteCommandInput>>>::empty(),
            find_file: <Option<Box<tools::FindFileInput>>>::empty(),
            grep: <Option<Box<tools::GrepInput>>>::empty(),
            list_files: <Option<Box<tools::ListFilesInput>>>::empty(),
            read_file: <Option<Box<tools::ReadFileInput>>>::empty(),
            submit_report: <Option<Box<tools::SubmitReportInput>>>::empty(),
            ask_user: <Option<Box<tools::AskUserInput>>>::empty(),
            handoff: <Option<Box<tools::HandoffInput>>>::empty(),
            fetch: <Option<Box<tools::FetchInput>>>::empty(),
            interpreter: <Option<Box<tools::InterpreterInput>>>::empty(),
        }
    }
}
impl Sample for tools::ToolOutput {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            create_file: <Option<Box<tools::CreateFileResult>>>::sample(d + 1, s),
            edit_file: <Option<Box<tools::EditFileResult>>>::sample(d + 1, s),
            execute_command: <Option<Box<tools::ExecuteCommandResult>>>::sample(d + 1, s),
            find_file: <Option<Box<tools::FindFileResult>>>::sample(d + 1, s),
            grep: <Option<Box<tools::GrepResult>>>::sample(d + 1, s),
            list_files: <Option<Box<tools::ListFilesResult>>>::sample(d + 1, s),
            read_file: <Option<Box<tools::ReadFileResult>>>::sample(d + 1, s),
            submit_report: <Option<Box<tools::SubmitReportResult>>>::sample(d + 1, s),
            ask_user: <Option<Box<tools::AskUserResult>>>::sample(d + 1, s),
            fetch: <Option<Box<tools::FetchResult>>>::sample(d + 1, s),
            interpreter: <Option<Box<tools::InterpreterOutput>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            create_file: <Option<Box<tools::CreateFileResult>>>::empty(),
            edit_file: <Option<Box<tools::EditFileResult>>>::empty(),
            execute_command: <Option<Box<tools::ExecuteCommandResult>>>::empty(),
            find_file: <Option<Box<tools::FindFileResult>>>::empty(),
            grep: <Option<Box<tools::GrepResult>>>::empty(),
            list_files: <Option<Box<tools::ListFilesResult>>>::empty(),
            read_file: <Option<Box<tools::ReadFileResult>>>::empty(),
            submit_report: <Option<Box<tools::SubmitReportResult>>>::empty(),
            ask_user: <Option<Box<tools::AskUserResult>>>::empty(),
            fetch: <Option<Box<tools::FetchResult>>>::empty(),
            interpreter: <Option<Box<tools::InterpreterOutput>>>::empty(),
        }
    }
}
impl Sample for tools::ToolCallEvent {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            id: <String>::sample(d + 1, s),
            tool: <String>::sample(d + 1, s),
            input: <tools::ToolInput>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            id: <String>::empty(),
            tool: <String>::empty(),
            input: <tools::ToolInput>::empty(),
        }
    }
}
impl Sample for tools::ToolResultEvent {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            id: <String>::sample(d + 1, s),
            tool: <String>::sample(d + 1, s),
            output: <tools::ToolOutput>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            id: <String>::empty(),
            tool: <String>::empty(),
            output: <tools::ToolOutput>::empty(),
        }
    }
}
impl Sample for tools::ProviderData {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            kind: <String>::sample(d + 1, s),
            id: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            kind: <String>::empty(),
            id: <String>::empty(),
        }
    }
}
impl Sample for tools::ToolCall {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            provider: <Option<Box<tools::ProviderData>>>::sample(d + 1, s),
            tool: <String>::sample(d + 1, s),
            input: <Option<Box<tools::ToolInput>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            provider: <Option<Box<tools::ProviderData>>>::empty(),
            tool: <String>::empty(),
            input: <Option<Box<tools::ToolInput>>>::empty(),
        }
    }
}
impl Sample for tools::ToolResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            provider: <Option<Box<tools::ProviderData>>>::sample(d + 1, s),
            tool: <String>::sample(d + 1, s),
            output: <Option<Box<tools::ToolOutput>>>::sample(d + 1, s),
            succeeded: <bool>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            provider: <Option<Box<tools::ProviderData>>>::empty(),
            tool: <String>::empty(),
            output: <Option<Box<tools::ToolOutput>>>::empty(),
            succeeded: <bool>::empty(),
        }
    }
}
impl Sample for tools::InterpreterToolResult {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            id: <String>::sample(d + 1, s),
            provider_kind: <String>::sample(d + 1, s),
            output: <String>::sample(d + 1, s),
            function_calls: <Option<Vec<tools::FunctionCall>>>::sample(d + 1, s),
            tool_stats: <Option<BTreeMap<String, i64>>>::sample(d + 1, s),
            error: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            id: <String>::empty(),
            provider_kind: <String>::empty(),
            output: <String>::empty(),
            function_calls: <Option<Vec<tools::FunctionCall>>>::empty(),
            tool_stats: <Option<BTreeMap<String, i64>>>::empty(),
            error: <String>::empty(),
        }
    }
}
impl Sample for ToolsFixture {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            create_file_input: tools::CreateFileInput::sample(d + 1, s),
            create_file_result: tools::CreateFileResult::sample(d + 1, s),
            edit_file_input: tools::EditFileInput::sample(d + 1, s),
            diff_pair: tools::DiffPair::sample(d + 1, s),
            diff_validation_error: tools::DiffValidationError::sample(d + 1, s),
            conflict_warning: tools::ConflictWarning::sample(d + 1, s),
            patch_info: tools::PatchInfo::sample(d + 1, s),
            edit_file_result: tools::EditFileResult::sample(d + 1, s),
            find_file_input: tools::FindFileInput::sample(d + 1, s),
            find_file_result: tools::FindFileResult::sample(d + 1, s),
            grep_input: tools::GrepInput::sample(d + 1, s),
            grep_match: tools::GrepMatch::sample(d + 1, s),
            grep_result: tools::GrepResult::sample(d + 1, s),
            list_files_input: tools::ListFilesInput::sample(d + 1, s),
            list_files_result: tools::ListFilesResult::sample(d + 1, s),
            directory_entry: tools::DirectoryEntry::sample(d + 1, s),
            read_file_input: tools::ReadFileInput::sample(d + 1, s),
            read_file_result: tools::ReadFileResult::sample(d + 1, s),
            execute_command_input: tools::ExecuteCommandInput::sample(d + 1, s),
            execute_command_result: tools::ExecuteCommandResult::sample(d + 1, s),
            ask_user_input: tools::AskUserInput::sample(d + 1, s),
            ask_user_result: tools::AskUserResult::sample(d + 1, s),
            submit_report_input: tools::SubmitReportInput::sample(d + 1, s),
            submit_report_result: tools::SubmitReportResult::sample(d + 1, s),
            handoff_input: tools::HandoffInput::sample(d + 1, s),
            fetch_input: tools::FetchInput::sample(d + 1, s),
            fetch_result: tools::FetchResult::sample(d + 1, s),
            interpreter_input: tools::InterpreterInput::sample(d + 1, s),
            interpreter_output: tools::InterpreterOutput::sample(d + 1, s),
            function_call: tools::FunctionCall::sample(d + 1, s),
            tool_input: tools::ToolInput::sample(d + 1, s),
            tool_output: tools::ToolOutput::sample(d + 1, s),
            tool_call_event: tools::ToolCallEvent::sample(d + 1, s),
            tool_result_event: tools::ToolResultEvent::sample(d + 1, s),
            provider_data: tools::ProviderData::sample(d + 1, s),
            tool_call: tools::ToolCall::sample(d + 1, s),
            tool_result: tools::ToolResult::sample(d + 1, s),
            interpreter_tool_result: tools::InterpreterToolResult::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            create_file_input: tools::CreateFileInput::empty(),
            create_file_result: tools::CreateFileResult::empty(),
            edit_file_input: tools::EditFileInput::empty(),
            diff_pair: tools::DiffPair::empty(),
            diff_validation_error: tools::DiffValidationError::empty(),
            conflict_warning: tools::ConflictWarning::empty(),
            patch_info: tools::PatchInfo::empty(),
            edit_file_result: tools::EditFileResult::empty(),
            find_file_input: tools::FindFileInput::empty(),
            find_file_result: tools::FindFileResult::empty(),
            grep_input: tools::GrepInput::empty(),
            grep_match: tools::GrepMatch::empty(),
            grep_result: tools::GrepResult::empty(),
            list_files_input: tools::ListFilesInput::empty(),
            list_files_result: tools::ListFilesResult::empty(),
            directory_entry: tools::DirectoryEntry::empty(),
            read_file_input: tools::ReadFileInput::empty(),
            read_file_result: tools::ReadFileResult::empty(),
            execute_command_input: tools::ExecuteCommandInput::empty(),
            execute_command_result: tools::ExecuteCommandResult::empty(),
            ask_user_input: tools::AskUserInput::empty(),
            ask_user_result: tools::AskUserResult::empty(),
            submit_report_input: tools::SubmitReportInput::empty(),
            submit_report_result: tools::SubmitReportResult::empty(),
            handoff_input: tools::HandoffInput::empty(),
            fetch_input: tools::FetchInput::empty(),
            fetch_result: tools::FetchResult::empty(),
            interpreter_input: tools::InterpreterInput::empty(),
            interpreter_output: tools::InterpreterOutput::empty(),
            function_call: tools::FunctionCall::empty(),
            tool_input: tools::ToolInput::empty(),
            tool_output: tools::ToolOutput::empty(),
            tool_call_event: tools::ToolCallEvent::empty(),
            tool_result_event: tools::ToolResultEvent::empty(),
            provider_data: tools::ProviderData::empty(),
            tool_call: tools::ToolCall::empty(),
            tool_result: tools::ToolResult::empty(),
            interpreter_tool_result: tools::InterpreterToolResult::empty(),
        }
    }
}
#[test]
fn tools_go_goldens() {
    for mode in ["zero", "empty", "full", "special"] {
        let v = if mode == "zero" {
            ToolsFixture::default()
        } else if mode == "empty" {
            ToolsFixture::empty()
        } else {
            ToolsFixture::sample(0, mode == "special")
        };
        fixture(&format!("tools-{mode}"), &v);
    }
}

#[derive(Debug, Default, Serialize, Deserialize)]
struct ProviderFixture {
    #[serde(rename = "Message")]
    message: provider::Message,
    #[serde(rename = "TextBlock")]
    text_block: provider::TextBlock,
    #[serde(rename = "ToolCallBlock")]
    tool_call_block: provider::ToolCallBlock,
    #[serde(rename = "ToolResultBlock")]
    tool_result_block: provider::ToolResultBlock,
    #[serde(rename = "Usage")]
    usage: provider::Usage,
    #[serde(rename = "Model")]
    model: provider::Model,
    #[serde(rename = "ModelPricing")]
    model_pricing: provider::ModelPricing,
    #[serde(rename = "MessageSource")]
    message_source: provider::MessageSource,
    #[serde(rename = "Capability")]
    capability: provider::Capability,
    #[serde(rename = "ContentBlockType")]
    content_block_type: provider::ContentBlockType,
    #[serde(rename = "ProviderKind")]
    provider_kind: provider::ProviderKind,
}
impl Sample for provider::Message {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            source: <provider::MessageSource>::sample(d + 1, s),
            content: <Option<Vec<provider::ContentBlock>>>::sample(d + 1, s),
            usage: <provider::Usage>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            source: <provider::MessageSource>::empty(),
            content: <Option<Vec<provider::ContentBlock>>>::empty(),
            usage: <provider::Usage>::empty(),
        }
    }
}
impl Sample for provider::TextBlock {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            text: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            text: <String>::empty(),
        }
    }
}
impl Sample for provider::ToolCallBlock {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            id: <String>::sample(d + 1, s),
            tool: <String>::sample(d + 1, s),
            args: <Option<Box<RawValue>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            id: <String>::empty(),
            tool: <String>::empty(),
            args: <Option<Box<RawValue>>>::empty(),
        }
    }
}
impl Sample for provider::ToolResultBlock {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            id: <String>::sample(d + 1, s),
            name: <String>::sample(d + 1, s),
            result: <String>::sample(d + 1, s),
            succeeded: <bool>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            id: <String>::empty(),
            name: <String>::empty(),
            result: <String>::empty(),
            succeeded: <bool>::empty(),
        }
    }
}
impl Sample for provider::Usage {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            input_tokens: <i64>::sample(d + 1, s),
            output_tokens: <i64>::sample(d + 1, s),
            cache_write_tokens: <i64>::sample(d + 1, s),
            cache_read_tokens: <i64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            input_tokens: <i64>::empty(),
            output_tokens: <i64>::empty(),
            cache_write_tokens: <i64>::empty(),
            cache_read_tokens: <i64>::empty(),
        }
    }
}
impl Sample for provider::Model {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            id: <Uuid>::sample(d + 1, s),
            provider: <provider::ProviderKind>::sample(d + 1, s),
            name: <String>::sample(d + 1, s),
            capabilities: <Option<Vec<provider::Capability>>>::sample(d + 1, s),
            context_window: <i64>::sample(d + 1, s),
            pricing: <provider::ModelPricing>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            id: <Uuid>::empty(),
            provider: <provider::ProviderKind>::empty(),
            name: <String>::empty(),
            capabilities: <Option<Vec<provider::Capability>>>::empty(),
            context_window: <i64>::empty(),
            pricing: <provider::ModelPricing>::empty(),
        }
    }
}
impl Sample for provider::ModelPricing {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            input: <f64>::sample(d + 1, s),
            output: <f64>::sample(d + 1, s),
            cache_write: <f64>::sample(d + 1, s),
            cache_read: <f64>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            input: <f64>::empty(),
            output: <f64>::empty(),
            cache_write: <f64>::empty(),
            cache_read: <f64>::empty(),
        }
    }
}
impl Sample for provider::MessageSource {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for provider::Capability {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for provider::ContentBlockType {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for provider::ProviderKind {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for ProviderFixture {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            message: provider::Message::sample(d + 1, s),
            text_block: provider::TextBlock::sample(d + 1, s),
            tool_call_block: provider::ToolCallBlock::sample(d + 1, s),
            tool_result_block: provider::ToolResultBlock::sample(d + 1, s),
            usage: provider::Usage::sample(d + 1, s),
            model: provider::Model::sample(d + 1, s),
            model_pricing: provider::ModelPricing::sample(d + 1, s),
            message_source: provider::MessageSource::sample(d + 1, s),
            capability: provider::Capability::sample(d + 1, s),
            content_block_type: provider::ContentBlockType::sample(d + 1, s),
            provider_kind: provider::ProviderKind::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            message: provider::Message::empty(),
            text_block: provider::TextBlock::empty(),
            tool_call_block: provider::ToolCallBlock::empty(),
            tool_result_block: provider::ToolResultBlock::empty(),
            usage: provider::Usage::empty(),
            model: provider::Model::empty(),
            model_pricing: provider::ModelPricing::empty(),
            message_source: provider::MessageSource::empty(),
            capability: provider::Capability::empty(),
            content_block_type: provider::ContentBlockType::empty(),
            provider_kind: provider::ProviderKind::empty(),
        }
    }
}
#[test]
fn provider_go_goldens() {
    for mode in ["zero", "empty", "full", "special"] {
        let v = if mode == "zero" {
            ProviderFixture::default()
        } else if mode == "empty" {
            ProviderFixture::empty()
        } else {
            ProviderFixture::sample(0, mode == "special")
        };
        fixture(&format!("provider-{mode}"), &v);
    }
}

#[derive(Debug, Default, Serialize, Deserialize)]
struct SkillFixture {
    #[serde(rename = "LockFile")]
    lock_file: skill::LockFile,
    #[serde(rename = "LockEntry")]
    lock_entry: skill::LockEntry,
    #[serde(rename = "GitSource")]
    git_source: skill::GitSource,
    #[serde(rename = "URLSource")]
    url_source: skill::URLSource,
    #[serde(rename = "GitProvider")]
    git_provider: skill::GitProvider,
}
impl Sample for skill::LockFile {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            version: <i64>::sample(d + 1, s),
            skills: <Option<BTreeMap<String, Option<Box<skill::LockEntry>>>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            version: <i64>::empty(),
            skills: <Option<BTreeMap<String, Option<Box<skill::LockEntry>>>>>::empty(),
        }
    }
}
impl Sample for skill::LockEntry {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            installed_at: <Timestamp>::sample(d + 1, s),
            updated_at: <Timestamp>::sample(d + 1, s),
            git: <Option<Box<skill::GitSource>>>::sample(d + 1, s),
            url: <Option<Box<skill::URLSource>>>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            installed_at: <Timestamp>::empty(),
            updated_at: <Timestamp>::empty(),
            git: <Option<Box<skill::GitSource>>>::empty(),
            url: <Option<Box<skill::URLSource>>>::empty(),
        }
    }
}
impl Sample for skill::GitSource {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            provider: <skill::GitProvider>::sample(d + 1, s),
            clone_url: <String>::sample(d + 1, s),
            path: <String>::sample(d + 1, s),
            r#ref: <String>::sample(d + 1, s),
            tree_hash: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            provider: <skill::GitProvider>::empty(),
            clone_url: <String>::empty(),
            path: <String>::empty(),
            r#ref: <String>::empty(),
            tree_hash: <String>::empty(),
        }
    }
}
impl Sample for skill::URLSource {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            url: <String>::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            url: <String>::empty(),
        }
    }
}
impl Sample for skill::GitProvider {
    fn sample(d: usize, s: bool) -> Self {
        Self(String::sample(d, s))
    }
}
impl Sample for SkillFixture {
    fn sample(d: usize, s: bool) -> Self {
        if d > 5 {
            return Self::default();
        }
        Self {
            lock_file: skill::LockFile::sample(d + 1, s),
            lock_entry: skill::LockEntry::sample(d + 1, s),
            git_source: skill::GitSource::sample(d + 1, s),
            url_source: skill::URLSource::sample(d + 1, s),
            git_provider: skill::GitProvider::sample(d + 1, s),
        }
    }
    fn empty() -> Self {
        Self {
            lock_file: skill::LockFile::empty(),
            lock_entry: skill::LockEntry::empty(),
            git_source: skill::GitSource::empty(),
            url_source: skill::URLSource::empty(),
            git_provider: skill::GitProvider::empty(),
        }
    }
}
#[test]
fn skill_go_goldens() {
    for mode in ["zero", "empty", "full", "special"] {
        let v = if mode == "zero" {
            SkillFixture::default()
        } else if mode == "empty" {
            SkillFixture::empty()
        } else {
            SkillFixture::sample(0, mode == "special")
        };
        fixture(&format!("skill-{mode}"), &v);
    }
}

impl Sample for provider::ContentBlock {
    fn sample(d: usize, _: bool) -> Self {
        if d > 5 {
            Self::Null
        } else {
            Self::Text(provider::TextBlock {
                text: SPECIAL.into(),
            })
        }
    }
}
