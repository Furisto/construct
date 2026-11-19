use crate::Session;
use construct_core::{
    store::Store,
    tool::{Tool, ToolError},
};
use construct_types::tools::ToolInput;
use rquickjs::{Array, Ctx, Function, Value};
use std::sync::Arc;
use tokio::runtime::Handle;

/// A synchronous VM binding. Arguments stay in the VM to preserve undefined,
/// JavaScript coercions, getters and array-like objects.
pub trait CodeActTool: Send + Sync {
    /// JavaScript global function name.
    fn name(&self) -> &str;
    /// Go's rendered CodeAct description.
    fn description(&self) -> &str;
    /// Map JavaScript positional arguments to the persisted input union.
    fn input<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &Session,
        args: &[Value<'js>],
    ) -> rquickjs::Result<Option<ToolInput>>;
    /// Execute synchronously on the interpreter's blocking thread.
    fn call<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &mut Session,
        args: &[Value<'js>],
        runtime: &Handle,
    ) -> rquickjs::Result<Value<'js>>;
}

/// Wrap a host tool with Go's positional binding for its name.
pub fn host_tool(tool: Arc<dyn Tool>) -> Arc<dyn CodeActTool> {
    Arc::new(HostTool(tool))
}

struct HostTool(Arc<dyn Tool>);

pub(crate) const TOOL_ERROR_MESSAGE: &str = "__construct_tool_error_message";

pub(crate) fn throw(ctx: &Ctx<'_>, mut error: ToolError) -> rquickjs::Error {
    error.details.insert("toolName".into(), "".into());
    let exception = rquickjs::Exception::from_message(ctx.clone(), &error.to_string())
        .expect("JS error allocation");
    let details = ctx
        .json_parse(serde_json::to_string(&error.details).expect("error JSON"))
        .expect("details object");
    exception.set("details", details).expect("error details");
    exception
        .prop(
            TOOL_ERROR_MESSAGE,
            rquickjs::object::Property::from(error.to_string()),
        )
        .expect("original tool error");
    exception.throw()
}

impl CodeActTool for HostTool {
    fn name(&self) -> &str {
        self.0.name()
    }
    fn description(&self) -> &str {
        self.0.description()
    }
    fn input<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &Session,
        args: &[Value<'js>],
    ) -> rquickjs::Result<Option<ToolInput>> {
        let mapper: Function = ctx.eval(include_str!("mapping.js"))?;
        let array = Array::new(ctx.clone())?;
        for (index, value) in args.iter().enumerate() {
            array.set(index, value.clone())?;
        }
        let value: Value = mapper.call((
            self.name(),
            array.clone(),
            session.task.id.to_string(),
            session.agent_id.to_string(),
            session.task.project_directory.to_string_lossy().as_ref(),
        ))?;
        let json = ctx
            .json_stringify(value)?
            .expect("mapping returns JSON")
            .to_string()?;
        let mut input: serde_json::Value = serde_json::from_str(&json).expect("VM JSON");
        if input.is_null() {
            return Ok(None);
        }
        if input.get("invalid").is_some() {
            let arguments = if self.name() == "edit_file" {
                let json = ctx
                    .json_stringify(array)?
                    .expect("argument array")
                    .to_string()?;
                serde_json::from_str(&json).expect("VM argument JSON")
            } else {
                serde_json::Value::Null
            };
            return Err(throw(ctx, invalid(self.name(), arguments)));
        }
        for field in ["start_line", "end_line", "max_results", "timeout"] {
            if let Some(value) = input.get_mut(field) {
                if let Some(text) = value.as_str() {
                    *value = text.parse::<i64>().expect("clipped VM integer").into();
                }
            }
        }
        serde_json::from_value(serde_json::json!({self.name(): input}))
            .map(Some)
            .map_err(|e| throw(ctx, ToolError::custom(e.to_string(), vec![])))
    }
    fn call<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &mut Session,
        args: &[Value<'js>],
        runtime: &Handle,
    ) -> rquickjs::Result<Value<'js>> {
        let input = self
            .input(ctx, session, args)?
            .ok_or_else(|| rquickjs::Exception::throw_type(ctx, "missing tool input"))?;
        let input = serde_json::to_value(input).expect("tool input JSON")[self.name()].clone();
        if self.name() == "edit_file" && input["diffs"].as_array().is_none_or(|a| a.is_empty()) {
            return Err(throw(
                ctx,
                ToolError::custom(
                    "diffs array cannot be empty",
                    vec!["Provide at least one diff object with 'old' and 'new' properties".into()],
                ),
            ));
        }
        let output = runtime
            .block_on(self.0.execute(&session.context, input))
            .map_err(|e| throw(ctx, e))?;
        if self.name() == "handoff" {
            return Ok(Value::new_undefined(ctx.clone()));
        }
        if self.name() == "submit_report" {
            session.console.push_str("REPORT SUBMITTED\n");
        }
        let json = construct_types::to_go_json(&output).expect("tool output JSON");
        let object: std::collections::BTreeMap<String, Box<serde_json::value::RawValue>> =
            serde_json::from_slice(&json).expect("tool output object");
        session.result = Some(output);
        let value = ctx.json_parse(object.get(self.name()).map_or("null", |arm| arm.get()))?;
        crate::result::tag(&value, &format!("&{}", self.name()))?;
        Ok(value)
    }
}

fn invalid(name: &str, arguments: serde_json::Value) -> ToolError {
    match name {
        "create_file" => ToolError::custom("Invalid argument", vec![
            "Ensure that you provide the correct input arguments as specified in the tool description".into(),
            "- **path** (string, required): Absolute path to the file beginning with a forward slash (e.g., \"/workspace/construct/src/components/button.js\"). Forward slashes (/) work on all platforms. All necessary parent directories will be created automatically.".into(),
            "- **content** (string, required): ENTIRE content to write to the file. Do not use placeholders, ellipses, or \"rest of file unchanged\".".into(),
            "For example: create_file('/project/src/app.js', 'console.log(\"Hello World\");')".into(),
        ]),
        "list_files" => ToolError::custom("Invalid argument", LIST_SUGGESTIONS.iter().map(|s| (*s).into()).collect()),
        "edit_file" => {
            let mut error = ToolError::custom("invalid arguments", vec!["The edit_file tool requires exactly two arguments: path and diffs".into()]);
            error.details.insert("arguments".into(), arguments);
            error
        },
        "submit_report" => ToolError::custom("submit_report requires at least 1 argument", vec![
            "- **summary** (string, required): A clear, concise summary of what was accomplished".into(),
            "- **completed** (boolean, required): Whether the task has been completed".into(),
            "- **deliverables** (array, optional): List of specific files, features, or outputs created".into(),
            "- **next_steps** (string, optional): Suggestions for follow-up actions".into(),
        ]),
        _ => unreachable!(),
    }
}

const LIST_SUGGESTIONS: &[&str] = &[
    "Ensure that you provide the correct input arguments as specified in the tool description",
    "- **path** (string, required): Absolute path to the directory you want to list (e.g., \"/workspace/project/src\"). Forward slashes (/) work on all platforms.",
    "- **recursive** (boolean, required): When set to true, lists all files and directories recursively through all subdirectories. When false, only lists the top-level contents of the specified directory.",
    "For example: list_files('/project/src', false)",
];

macro_rules! constructor {
    ($name:ident, $host:path) => {
        #[doc = concat!("Construct Go's `", stringify!($name), "` binding over the host implementation.")]
        pub fn $name() -> Arc<dyn CodeActTool> { host_tool(Arc::new($host)) }
    };
}
constructor!(create_file, construct_tools::filesystem::CreateFile);
constructor!(read_file, construct_tools::filesystem::ReadFile);
constructor!(edit_file, construct_tools::filesystem::EditFile);
constructor!(list_files, construct_tools::filesystem::ListFiles);
constructor!(grep, construct_tools::filesystem::Grep);
constructor!(find_file, construct_tools::filesystem::FindFile);
constructor!(execute_command, construct_tools::system::ExecuteCommand);
constructor!(submit_report, construct_tools::communication::SubmitReport);
/// Construct the HTTP fetch binding with the default host client.
pub fn fetch() -> Arc<dyn CodeActTool> {
    host_tool(Arc::new(construct_tools::web::Fetch::default()))
}
/// Construct the Store-backed handoff binding.
pub fn handoff(store: Arc<dyn Store>) -> Arc<dyn CodeActTool> {
    host_tool(Arc::new(construct_tools::communication::Handoff::new(
        store,
    )))
}

/// Construct Go's print binding.
pub fn print() -> Arc<dyn CodeActTool> {
    Arc::new(Print)
}
struct Print;
impl CodeActTool for Print {
    fn name(&self) -> &str {
        "print"
    }
    fn description(&self) -> &str {
        include_str!("print.md")
    }
    fn input<'js>(
        &self,
        _ctx: &Ctx<'js>,
        _session: &Session,
        _args: &[Value<'js>],
    ) -> rquickjs::Result<Option<ToolInput>> {
        Ok(None)
    }
    fn call<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &mut Session,
        args: &[Value<'js>],
        _runtime: &Handle,
    ) -> rquickjs::Result<Value<'js>> {
        for (index, arg) in args.iter().enumerate() {
            if index != 0 {
                session.console.push(' ');
            }
            session.console.push_str(&format_value(ctx, arg)?);
        }
        session.console.push('\n');
        Ok(Value::new_undefined(ctx.clone()))
    }
}

pub(crate) fn format_value<'js>(ctx: &Ctx<'js>, value: &Value<'js>) -> rquickjs::Result<String> {
    if value.is_null() || value.is_undefined() {
        return Ok("<nil>".into());
    }
    if let Some(s) = value.as_string() {
        return s.to_string();
    }
    if let Some(b) = value.as_bool() {
        return Ok(b.to_string());
    }
    if let Some(n) = value.as_number() {
        if n.is_nan() {
            return Ok("NaN".into());
        }
        if n.is_infinite() {
            return Ok(if n.is_sign_positive() { "+Inf" } else { "-Inf" }.into());
        }
        if n == 0.0 {
            return Ok(if n.is_sign_negative() { "-0" } else { "0" }.into());
        }
        if n.fract() == 0.0 && n >= i64::MIN as f64 && n < -(i64::MIN as f64) {
            return Ok(format!("{n:.0}"));
        }
        if n.abs() >= 1e6 || n.abs() < 1e-4 {
            let raw = format!("{n:e}");
            let (base, exponent) = raw.split_once('e').expect("exponent");
            let exponent: i32 = exponent.parse().expect("numeric exponent");
            return Ok(format!("{base}e{exponent:+03}"));
        }
        return Ok(n.to_string());
    }
    if let Some(object) = value.as_object() {
        if let Some(text) = crate::result::format(ctx, object)? {
            return Ok(text);
        }
    }
    if let Some(array) = value.as_array() {
        let mut values = Vec::new();
        for item in array.iter::<Value>() {
            values.push(format_value(ctx, &item?)?);
        }
        return Ok(format!("[{}]", values.join(" ")));
    }
    if let Some(object) = value.as_object() {
        let mut values = std::collections::BTreeMap::new();
        for key in object.keys::<String>() {
            let key = key?;
            values.insert(
                key.clone(),
                format_value(ctx, &object.get::<_, Value>(key)?)?,
            );
        }
        return Ok(format!(
            "map[{}]",
            values
                .into_iter()
                .map(|(k, v)| format!("{k}:{v}"))
                .collect::<Vec<_>>()
                .join(" ")
        ));
    }
    let string: Function = ctx.eval("String")?;
    string.call((value.clone(),))
}
