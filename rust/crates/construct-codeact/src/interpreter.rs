use crate::{CodeActTool, FunctionCallState, Interceptor, Invocation, Session, Task};
use construct_core::tool::ToolContext;
use construct_types::tools::{InterpreterInput, InterpreterOutput};
use rquickjs::{function::Rest, Context, Ctx, Function, Runtime, Value};
use std::{cell::RefCell, fmt, rc::Rc, sync::Arc};
use tokio::runtime::Handle;

/// Uncaught script failure. Output is returned separately even on failure.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct InterpreterError(pub String);
impl fmt::Display for InterpreterError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}
impl std::error::Error for InterpreterError {}

/// QuickJS-backed interpreter. Each interpretation owns a fresh VM and session.
#[derive(Clone)]
pub struct Interpreter {
    tools: Vec<Arc<dyn CodeActTool>>,
    interceptors: Vec<Arc<dyn Interceptor>>,
}

impl Interpreter {
    /// Bind tools and wrappers. The last interceptor wraps all earlier entries.
    pub fn new(tools: Vec<Arc<dyn CodeActTool>>, interceptors: Vec<Arc<dyn Interceptor>>) -> Self {
        Self {
            tools,
            interceptors,
        }
    }
    /// Go's interpreter tool name.
    pub fn name(&self) -> &str {
        "code_interpreter"
    }
    /// Go's interpreter description, verbatim.
    pub fn description(&self) -> &str {
        "Can be used to call tools using Javascript syntax. Write a complete javascript program and use only the functions that have been specified. If you use any other functions the tool call will fail."
    }
    /// Go's reflected properties, without the reflector's outer metadata.
    pub fn input_schema(&self) -> serde_json::Value {
        serde_json::json!({"type":"object","properties":{"script":{"type":"string"}}})
    }
    /// Run on a blocking worker, returning captured output even after an error.
    pub async fn interpret(
        &self,
        context: ToolContext,
        input: InterpreterInput,
        task: Task,
    ) -> (InterpreterOutput, Option<InterpreterError>) {
        self.resume(context, input, task, FunctionCallState::default())
            .await
    }
    /// Re-run a script using stored successful results before executing new calls.
    /// Match tool/input in stored vector order, skipping obsolete Go duplicates.
    /// Stored numeric indices are ignored; successful logical calls are reindexed.
    /// Unmatched input fails closed while unconsumed stored records remain.
    pub async fn resume(
        &self,
        context: ToolContext,
        input: InterpreterInput,
        task: Task,
        stored: FunctionCallState,
    ) -> (InterpreterOutput, Option<InterpreterError>) {
        let this = self.clone();
        let handle = Handle::current();
        match tokio::task::spawn_blocking(move || this.run(context, input, task, handle, stored))
            .await
        {
            Ok(result) => result,
            Err(error) => (
                InterpreterOutput::default(),
                Some(InterpreterError(error.to_string())),
            ),
        }
    }
    fn run(
        &self,
        context: ToolContext,
        input: InterpreterInput,
        task: Task,
        handle: Handle,
        stored: FunctionCallState,
    ) -> (InterpreterOutput, Option<InterpreterError>) {
        let cancellation = context.cancellation.clone();
        let mut session = Session::new(context, task);
        session.replay = Some(crate::replay::Replay::new(stored.calls));
        let session = Rc::new(RefCell::new(session));
        let result = (|| -> Result<(), InterpreterError> {
            let vm = Runtime::new().map_err(|e| InterpreterError(e.to_string()))?;
            let token = cancellation.clone();
            vm.set_interrupt_handler(Some(Box::new(move || token.is_cancelled())));
            let context = Context::full(&vm).map_err(|e| InterpreterError(e.to_string()))?;
            context.with(|ctx| {
                for tool in &self.tools {
                    let tool = tool.clone();
                    let state = session.clone();
                    let interceptors = self.interceptors.clone();
                    let handle = handle.clone();
                    let name = tool.name().to_owned();
                    let function = binding(&ctx, state, tool, handle, interceptors)
                        .map_err(|e| InterpreterError(e.to_string()))?;
                    ctx.globals()
                        .set(name, function)
                        .map_err(|e| InterpreterError(e.to_string()))?;
                }
                match ctx.eval::<Value, _>(ensure_strict_mode(&input.script)) {
                    Ok(_) => Ok(()),
                    Err(error) => {
                        if cancellation.is_cancelled() {
                            return Err(InterpreterError("execution cancelled".into()));
                        }
                        if error.is_exception() {
                            let value = ctx.catch();
                            if let Some(object) = value.as_object() {
                                if let Ok(message) =
                                    object.get::<_, String>(crate::bindings::TOOL_ERROR_MESSAGE)
                                {
                                    return Err(InterpreterError(message));
                                }
                            }
                            let string: Function = ctx
                                .eval("String")
                                .map_err(|e| InterpreterError(e.to_string()))?;
                            let message: String = string
                                .call((value,))
                                .map_err(|e| InterpreterError(e.to_string()))?;
                            Err(InterpreterError(message))
                        } else {
                            Err(InterpreterError(error.to_string()))
                        }
                    }
                }
            })
        })();
        let mut session = session.borrow_mut();
        (
            InterpreterOutput {
                console_output: std::mem::take(&mut session.console),
                function_calls: Some(
                    session
                        .function_calls
                        .take()
                        .map_or_else(Vec::new, |s| s.calls),
                ),
                tool_stats: Some(std::mem::take(&mut session.tool_stats)),
            },
            result.err(),
        )
    }
}

fn binding<'js>(
    ctx: &Ctx<'js>,
    state: Rc<RefCell<Session>>,
    tool: Arc<dyn CodeActTool>,
    handle: Handle,
    interceptors: Vec<Arc<dyn Interceptor>>,
) -> rquickjs::Result<Function<'js>> {
    Function::new(
        ctx.clone(),
        move |ctx: Ctx<'js>, Rest(args): Rest<Value<'js>>| {
            state.borrow_mut().result = None;
            let result = (|| {
                let (input, input_error) = match tool.input(&ctx, &state, &args) {
                    Ok(input) => (input, None),
                    Err(error) => (
                        None,
                        Some(crate::middleware::capture_input_error(&ctx, error)?),
                    ),
                };
                let call = Invocation {
                    input,
                    arguments: &args,
                    input_error,
                };
                invoke(&ctx, &state, tool.as_ref(), &call, &handle, &interceptors)
            })();
            match result {
                Err(error) if error.is_exception() => {
                    let exception = ctx.catch();
                    if let Some(object) = exception.as_object().filter(|object| {
                        object
                            .contains_key(crate::bindings::TOOL_ERROR_MESSAGE)
                            .unwrap_or(false)
                    }) {
                        object.prop(
                            crate::bindings::PROPAGATED_ERROR,
                            rquickjs::object::Property::from(true),
                        )?;
                    }
                    Err(ctx.throw(exception))
                }
                result => result,
            }
        },
    )
}

fn invoke<'js>(
    ctx: &Ctx<'js>,
    session: &RefCell<Session>,
    tool: &dyn CodeActTool,
    call: &Invocation<'_, 'js>,
    runtime: &Handle,
    interceptors: &[Arc<dyn Interceptor>],
) -> rquickjs::Result<Value<'js>> {
    if let Some((outer, inner)) = interceptors.split_last() {
        outer.intercept(ctx, session, tool, call, &|session| {
            invoke(ctx, session, tool, call, runtime, inner)
        })
    } else {
        if let Some(error) = &call.input_error {
            return Err(ctx.throw(error.clone()));
        }
        if tool.name() != "print" {
            if let Some(input) = &call.input {
                let cached = session
                    .borrow_mut()
                    .replay
                    .as_mut()
                    .map_or(Ok(None), |replay| replay.take(tool.name(), input))
                    .map_err(|error| crate::bindings::throw(ctx, error))?;
                if let Some(output) = cached {
                    return tool.restore(ctx, session, output);
                }
            }
        }
        tool.call(ctx, session, call, runtime)
    }
}

/// Reproduce Go's literal prefix check (including its unquoted-prefix quirk).
pub fn ensure_strict_mode(script: &str) -> String {
    if script.starts_with("use strict;") {
        script.into()
    } else {
        format!("'use strict';\n{script}")
    }
}
