use crate::{CodeActTool, Session};
use construct_core::event::{Event, EventPayload, EventPublisher};
use construct_types::{
    tools::{FunctionCall, ToolCallEvent, ToolResultEvent},
    Timestamp,
};
use rquickjs::{Ctx, Value};
use std::sync::Arc;

/// The next synchronous call in an interceptor chain.
pub type Next<'a, 'js> = dyn Fn(&mut Session) -> rquickjs::Result<Value<'js>> + 'a;

/// A synchronous wrapper; later entries in the interpreter list are outermost.
pub trait Interceptor: Send + Sync {
    /// Wrap a tool invocation. Errors propagate without running success hooks.
    fn intercept<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &mut Session,
        tool: &dyn CodeActTool,
        args: &[Value<'js>],
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>>;
}

/// Count attempts before executing the inner binding, excluding print.
pub struct ToolStatistics;
impl Interceptor for ToolStatistics {
    fn intercept<'js>(
        &self,
        _ctx: &Ctx<'js>,
        session: &mut Session,
        tool: &dyn CodeActTool,
        _args: &[Value<'js>],
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        if tool.name() != "print" {
            *session.tool_stats.entry(tool.name().into()).or_default() += 1;
        }
        next(session)
    }
}

/// Record successful calls only; failed calls do not consume an index.
pub struct DurableFunction;
impl Interceptor for DurableFunction {
    fn intercept<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &mut Session,
        tool: &dyn CodeActTool,
        args: &[Value<'js>],
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        if tool.name() == "print" {
            return next(session);
        }
        let input = match tool.input(ctx, session, args) {
            Ok(input) => input.unwrap_or_default(),
            Err(error) => {
                suppress_input_error(ctx, error)?;
                Default::default()
            }
        };
        let index = session.function_calls.as_ref().map_or(0, |s| s.index);
        let value = next(session)?;
        let output = session.result.clone().unwrap_or_default();
        let state = session.function_calls.get_or_insert_with(Default::default);
        state.calls.push(FunctionCall {
            tool_name: tool.name().into(),
            input,
            output,
            index,
        });
        state.index += 1;
        Ok(value)
    }
}

/// Reset Go's temporary result before every binding, including print.
pub struct ResetTemporarySessionValues;
impl Interceptor for ResetTemporarySessionValues {
    fn intercept<'js>(
        &self,
        _ctx: &Ctx<'js>,
        session: &mut Session,
        _tool: &dyn CodeActTool,
        _args: &[Value<'js>],
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        session.result = None;
        next(session)
    }
}

/// Go-compatible adapter publishing typed payloads on the core event bus.
pub struct ToolEventPublisher {
    /// Destination for transient tool.called/tool.result events.
    pub publisher: Arc<dyn EventPublisher>,
}

impl ToolEventPublisher {
    fn publish(&self, session: &Session, name: &str, payload: EventPayload) {
        self.publisher.publish(Event {
            r#type: name.into(),
            action: "created".into(),
            timestamp: Timestamp(std::time::SystemTime::now().into()),
            task_id: Some(session.task.id),
            payload,
        });
    }
}
impl Interceptor for ToolEventPublisher {
    fn intercept<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &mut Session,
        tool: &dyn CodeActTool,
        args: &[Value<'js>],
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        if tool.name() == "print" {
            return next(session);
        }
        match tool.input(ctx, session, args) {
            Ok(Some(input)) => self.publish(
                session,
                "tool.called",
                Arc::new(ToolCallEvent {
                    tool: tool.name().into(),
                    input,
                    ..Default::default()
                }),
            ),
            Ok(None) => {}
            Err(error) => {
                suppress_input_error(ctx, error)?;
            }
        }
        let value = next(session)?;
        if let Some(output) = &session.result {
            self.publish(
                session,
                "tool.result",
                Arc::new(ToolResultEvent {
                    tool: tool.name().into(),
                    output: output.clone(),
                    ..Default::default()
                }),
            );
        }
        Ok(value)
    }
}

fn suppress_input_error(ctx: &Ctx<'_>, error: rquickjs::Error) -> rquickjs::Result<()> {
    if !error.is_exception() {
        return Err(error);
    }
    let value = ctx.catch();
    if value.as_object().is_some_and(|o| {
        o.contains_key(crate::bindings::TOOL_ERROR_MESSAGE)
            .unwrap_or(false)
    }) {
        Ok(())
    } else {
        Err(ctx.throw(value))
    }
}

/// Runtime production order: stats, durable, events, reset (last is outermost).
pub fn production_interceptors(publisher: Arc<dyn EventPublisher>) -> Vec<Arc<dyn Interceptor>> {
    vec![
        Arc::new(ToolStatistics),
        Arc::new(DurableFunction),
        Arc::new(ToolEventPublisher { publisher }),
        Arc::new(ResetTemporarySessionValues),
    ]
}
