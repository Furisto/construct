use crate::{CodeActTool, Invocation, Session};
use construct_core::event::{Event, EventPayload, EventPublisher};
use construct_types::{
    tools::{FunctionCall, ToolCallEvent, ToolResultEvent},
    Timestamp,
};
use rquickjs::{Ctx, Value};
use std::{cell::RefCell, sync::Arc};

/// The next synchronous call in an interceptor chain.
pub type Next<'a, 'js> = dyn Fn(&RefCell<Session>) -> rquickjs::Result<Value<'js>> + 'a;

/// A synchronous wrapper; later entries in the interpreter list are outermost.
/// Release session borrows before invoking JavaScript or the next wrapper.
pub trait Interceptor: Send + Sync {
    /// Wrap a tool invocation. Errors propagate without running success hooks.
    fn intercept<'js>(
        &self,
        ctx: &Ctx<'js>,
        session: &RefCell<Session>,
        tool: &dyn CodeActTool,
        call: &Invocation<'_, 'js>,
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>>;
}

/// Count attempts before executing the inner binding, excluding print.
pub struct ToolStatistics;
impl Interceptor for ToolStatistics {
    fn intercept<'js>(
        &self,
        _ctx: &Ctx<'js>,
        session: &RefCell<Session>,
        tool: &dyn CodeActTool,
        _call: &Invocation<'_, 'js>,
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        if tool.name() != "print" {
            *session
                .borrow_mut()
                .tool_stats
                .entry(tool.name().into())
                .or_default() += 1;
        }
        next(session)
    }
}

/// Record successful calls only; failed calls do not consume an index.
pub struct DurableFunction;
impl Interceptor for DurableFunction {
    fn intercept<'js>(
        &self,
        _ctx: &Ctx<'js>,
        session: &RefCell<Session>,
        tool: &dyn CodeActTool,
        call: &Invocation<'_, 'js>,
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        if tool.name() == "print" {
            return next(session);
        }
        let value = next(session)?;
        let mut session = session.borrow_mut();
        let output = session.result.clone().unwrap_or_default();
        let state = session.function_calls.get_or_insert_with(Default::default);
        state.calls.push(FunctionCall {
            tool_name: tool.name().into(),
            input: call.input.clone().unwrap_or_default(),
            output,
            index: state.index,
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
        session: &RefCell<Session>,
        _tool: &dyn CodeActTool,
        _call: &Invocation<'_, 'js>,
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        session.borrow_mut().result = None;
        next(session)
    }
}

/// Go-compatible adapter publishing typed payloads on the core event bus.
pub struct ToolEventPublisher {
    /// Destination for transient tool.called/tool.result events.
    pub publisher: Arc<dyn EventPublisher>,
}

impl ToolEventPublisher {
    fn publish(&self, session: &RefCell<Session>, name: &str, payload: EventPayload) {
        self.publisher.publish(Event {
            r#type: name.into(),
            action: "created".into(),
            timestamp: Timestamp(std::time::SystemTime::now().into()),
            task_id: Some(session.borrow().task.id),
            payload,
        });
    }
}
impl Interceptor for ToolEventPublisher {
    fn intercept<'js>(
        &self,
        _ctx: &Ctx<'js>,
        session: &RefCell<Session>,
        tool: &dyn CodeActTool,
        call: &Invocation<'_, 'js>,
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        if tool.name() == "print" {
            return next(session);
        }
        if let Some(input) = &call.input {
            self.publish(
                session,
                "tool.called",
                Arc::new(ToolCallEvent {
                    tool: tool.name().into(),
                    input: input.clone(),
                    ..Default::default()
                }),
            );
        }
        let value = next(session)?;
        let output = session.borrow().result.clone();
        if let Some(output) = output {
            self.publish(
                session,
                "tool.result",
                Arc::new(ToolResultEvent {
                    tool: tool.name().into(),
                    output,
                    ..Default::default()
                }),
            );
        }
        Ok(value)
    }
}

pub(crate) fn capture_input_error<'js>(
    ctx: &Ctx<'js>,
    error: rquickjs::Error,
) -> rquickjs::Result<Value<'js>> {
    if !error.is_exception() {
        return Err(error);
    }
    let value = ctx.catch();
    if value.as_object().is_some_and(|o| {
        o.contains_key(crate::bindings::TOOL_ERROR_MESSAGE)
            .unwrap_or(false)
            && !o
                .get::<_, bool>(crate::bindings::PROPAGATED_ERROR)
                .unwrap_or(false)
    }) {
        Ok(value)
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
