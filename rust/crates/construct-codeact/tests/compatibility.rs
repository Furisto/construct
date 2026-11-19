use construct_codeact::*;
use construct_core::{
    event::{Event, EventPublisher},
    tool::{Tool, ToolContext, ToolError},
    CancellationToken,
};
use construct_types::tools::{InterpreterInput, ToolCallEvent, ToolOutput, ToolResultEvent};
use rquickjs::{Array, Context, Ctx, Runtime, Value};
use serde_json::{json, Value as Json};
use std::{
    cell::RefCell,
    path::Path,
    sync::{Arc, Mutex},
};
use uuid::Uuid;

fn scope(path: &Path) -> (ToolContext, Task) {
    let task = Task {
        id: Uuid::from_u128(1),
        project_directory: path.into(),
    };
    (
        ToolContext {
            workspace_root: path.into(),
            task_id: Some(task.id),
            agent_id: Some(Uuid::from_u128(2)),
            cancellation: CancellationToken::new(),
            events: None,
        },
        task,
    )
}

fn mapped(tool: Arc<dyn CodeActTool>, expression: &str) -> Option<Json> {
    let vm = Runtime::new().unwrap();
    let context = Context::full(&vm).unwrap();
    let (ctx, task) = scope(Path::new("/project"));
    let session = RefCell::new(Session::new(ctx, task));
    context.with(|ctx| {
        let args: Array = ctx.eval(expression).unwrap();
        let args = args
            .iter::<Value>()
            .collect::<rquickjs::Result<Vec<_>>>()
            .unwrap();
        tool.input(&ctx, &session, &args)
            .unwrap()
            .map(|i| serde_json::to_value(i).unwrap()[tool.name()].clone())
    })
}

#[test]
fn positional_mapping_for_every_tool() {
    assert_eq!(
        mapped(create_file(), "['/a','text']"),
        Some(json!({"Path":"/a","Content":"text"}))
    );
    assert_eq!(
        mapped(create_file(), "[{path:'/a',content:'text'}]"),
        Some(json!({"Path":"/a","Content":"text"}))
    );
    assert_eq!(
        mapped(read_file(), "[null, '2.9', undefined]"),
        Some(json!({"path":"null","start_line":2}))
    );
    assert_eq!(
        mapped(read_file(), "[undefined,null,3.8]"),
        Some(json!({"path":"undefined","end_line":3}))
    );
    assert_eq!(
        mapped(edit_file(), "['/a',[{old:12,new:null},{}]]"),
        Some(json!({"path":"/a","diffs":[{"old":"12","new":"null"},{"old":"","new":""}]}))
    );
    assert_eq!(
        mapped(list_files(), "['/a',false]"),
        Some(json!({"Path":"/a","Recursive":false}))
    );
    assert_eq!(
        mapped(list_files(), "[{path:'/a',recursive:true}]"),
        Some(json!({"Path":"/a","Recursive":true}))
    );
    assert_eq!(
        mapped(
            grep(),
            "[{query:12,path:null,case_sensitive:'false',max_results:0}]"
        ),
        Some(
            json!({"query":"12","path":"null","case_sensitive":true,"max_results":50,"context":0,"include_pattern":"","exclude_pattern":""})
        )
    );
    assert_eq!(
        mapped(find_file(), "[{pattern:'*.rs',max_results:-2.8}]"),
        Some(json!({"pattern":"*.rs","path":"","max_results":-2,"exclude_pattern":""}))
    );
    assert_eq!(
        mapped(execute_command(), "[['echo','ok']]"),
        Some(json!({"Command":"echo,ok","WorkingDirectory":"/project"}))
    );
    assert_eq!(
        mapped(
            fetch(),
            "[{url:null,headers:{a:2,b:undefined,c:null},timeout:'3.8'}]"
        ),
        Some(json!({"url":"null","headers":{"a":"2","c":"null"},"timeout":3}))
    );
    assert_eq!(
        mapped(
            submit_report(),
            "[{summary:12,completed:'',deliverables:[undefined,null,3],next_steps:false}]"
        ),
        Some(
            json!({"Summary":"12","Completed":false,"Deliverables":["null","3"],"NextSteps":"false"})
        )
    );
    let store = Arc::new(construct_core::testing::MemoryStore::default());
    assert_eq!(
        mapped(handoff(store), "['worker',null]"),
        Some(
            json!({"TaskID":Uuid::from_u128(1),"CurrentAgentID":Uuid::nil(),"RequestedAgent":"worker","HandoverMessage":"null"})
        )
    );
    assert_eq!(mapped(print(), "[1,null,undefined]"), None);
}

#[test]
fn empty_and_invalid_mapping() {
    for tool in [read_file(), grep(), find_file(), execute_command(), fetch()] {
        assert_eq!(mapped(tool, "[]"), None);
    }
    assert_eq!(
        mapped(edit_file(), "['/a',[]]"),
        Some(json!({"path":"/a","diffs":null}))
    );
    assert_eq!(mapped(fetch(), "[{}]"), Some(json!({"url":""})));
    assert_eq!(
        mapped(submit_report(), "[{}]"),
        Some(json!({"Summary":"","Completed":false,"Deliverables":null,"NextSteps":""}))
    );
    for (tool, expression) in [
        (create_file(), "[]"),
        (create_file(), "['x',null]"),
        (list_files(), "['x']"),
        (list_files(), "['x',0]"),
        (edit_file(), "[]"),
        (submit_report(), "[]"),
        (fetch(), "[null]"),
    ] {
        let vm = Runtime::new().unwrap();
        let context = Context::full(&vm).unwrap();
        let (scope, task) = scope(Path::new("/project"));
        let session = RefCell::new(Session::new(scope, task));
        context.with(|ctx| {
            let args: Array = ctx.eval(expression).unwrap();
            let args = args
                .iter::<Value>()
                .collect::<rquickjs::Result<Vec<_>>>()
                .unwrap();
            assert!(
                tool.input(&ctx, &session, &args).is_err(),
                "{} {expression}",
                tool.name()
            );
            assert!(ctx.catch().is_error());
        });
    }
}

#[test]
fn integer_clipping_and_javascript_coercion() {
    assert_eq!(
        mapped(read_file(), "['/a',Infinity,-Infinity]"),
        Some(json!({"path":"/a","start_line":i64::MAX,"end_line":i64::MIN}))
    );
    assert_eq!(
        mapped(read_file(), "['/a',NaN,'0xff']"),
        Some(json!({"path":"/a","start_line":0,"end_line":255}))
    );
    assert_eq!(
        mapped(read_file(), "['/a',1000000000000000100]"),
        Some(json!({"path":"/a","start_line":1000000000000000128_i64}))
    );
    assert_eq!(
        mapped(fetch(), "[{timeout:Infinity}]"),
        Some(json!({"url":"","timeout":i64::MAX}))
    );
}

#[test]
fn mapping_reads_each_accessor_once() {
    assert_eq!(
        mapped(fetch(), "(()=>{let n=0;return [{get url(){return 'http://example/'+(++n)},get timeout(){return ++n},get headers(){return {get x(){return ++n}}}}]})()"),
        Some(json!({"url":"http://example/1","timeout":2,"headers":{"x":"3"}}))
    );
    assert_eq!(
        mapped(submit_report(), "(()=>{let n=0;return [{get summary(){return ++n},get next_steps(){return ++n},get completed(){return ++n},get deliverables(){return [++n]}}]})()"),
        Some(json!({"Summary":"1","NextSteps":"2","Completed":true,"Deliverables":["4"]}))
    );
    assert_eq!(
        mapped(edit_file(), "['/a',{get length(){if(this.seen)throw new Error('read twice');this.seen=true;return 1},0:{old:'a',new:'b'}}]"),
        Some(json!({"path":"/a","diffs":[{"old":"a","new":"b"}]}))
    );
}

#[derive(Default)]
struct Events(Mutex<Vec<Event>>);
impl EventPublisher for Events {
    fn publish(&self, event: Event) {
        self.0.lock().unwrap().push(event);
    }
}

async fn run(
    script: &str,
    tools: Vec<Arc<dyn CodeActTool>>,
    interceptors: Vec<Arc<dyn Interceptor>>,
) -> (
    construct_types::tools::InterpreterOutput,
    Option<InterpreterError>,
) {
    let (ctx, task) = scope(Path::new("/project"));
    Interpreter::new(tools, interceptors)
        .interpret(
            ctx,
            InterpreterInput {
                script: script.into(),
            },
            task,
        )
        .await
}

#[tokio::test]
async fn print_go_exports() {
    let (output,error) = run("print('hello',42,true,null,undefined,[1,'x',null],{z:2,a:{b:false}}); print(); print(1e21,1e20,1.25e-5,NaN,Infinity,-Infinity);", vec![print()], vec![Arc::new(ToolStatistics),Arc::new(DurableFunction)]).await;
    assert_eq!(error, None);
    assert_eq!(output.console_output,"hello 42 true <nil> <nil> [1 x <nil>] map[a:map[b:false] z:2]\n\n1e+21 1e+20 1.25e-05 NaN +Inf -Inf\n");
    assert!(output.function_calls.unwrap().is_empty());
    assert!(output.tool_stats.unwrap().is_empty());
}

#[tokio::test]
async fn negative_zero_and_go_numeric_thresholds() {
    let (output, error) = run("print(-0,1e6,1.2e6,1e-4,1.2e-5)", vec![print()], vec![]).await;
    assert_eq!(error, None);
    assert_eq!(output.console_output, "-0 1000000 1200000 0.0001 1.2e-05\n");
}

struct SuccessfulHandoff;
#[construct_core::async_trait]
impl Tool for SuccessfulHandoff {
    fn name(&self) -> &str {
        "handoff"
    }
    fn description(&self) -> &str {
        "test"
    }
    fn input_schema(&self) -> Json {
        json!({})
    }
    async fn execute(&self, _ctx: &ToolContext, _input: Json) -> Result<ToolOutput, ToolError> {
        Ok(ToolOutput::default())
    }
}

#[tokio::test]
async fn handoff_has_no_result_and_reset_removes_previous_result() {
    let events = Arc::new(Events::default());
    let (output, error) = run(
        "submit_report({summary:'ok',completed:true});print(handoff('other'));",
        vec![
            submit_report(),
            host_tool(Arc::new(SuccessfulHandoff)),
            print(),
        ],
        production_interceptors(events.clone()),
    )
    .await;
    assert_eq!(error, None);
    assert_eq!(output.console_output, "REPORT SUBMITTED\n<nil>\n");
    let calls = output.function_calls.unwrap();
    assert_eq!(calls.len(), 2);
    assert_eq!(calls[1].index, 1);
    assert_eq!(
        construct_types::to_go_json(&calls[1].output).unwrap(),
        b"{}"
    );
    let events = events.0.lock().unwrap();
    assert_eq!(events.len(), 3);
    assert_eq!(events[2].r#type, "tool.called");
}

#[tokio::test]
async fn converted_nested_result_does_not_leak_into_handoff() {
    let events = Arc::new(Events::default());
    let (output, error) = run(
        "print(handoff({toString(){submit_report({summary:'nested',completed:true});return 'other'}}));",
        vec![submit_report(), host_tool(Arc::new(SuccessfulHandoff)), print()],
        production_interceptors(events.clone()),
    ).await;
    assert_eq!(error, None);
    assert_eq!(output.console_output, "REPORT SUBMITTED\n<nil>\n");
    let calls = output.function_calls.unwrap();
    assert_eq!(
        calls
            .iter()
            .map(|call| (call.tool_name.as_str(), call.index))
            .collect::<Vec<_>>(),
        [("submit_report", 0), ("handoff", 1)]
    );
    assert_eq!(
        construct_types::to_go_json(&calls[1].output).unwrap(),
        b"{}"
    );
    assert_eq!(
        output.tool_stats.unwrap(),
        std::collections::BTreeMap::from([("handoff".into(), 1), ("submit_report".into(), 1)])
    );
    let events = events.0.lock().unwrap();
    assert_eq!(
        events
            .iter()
            .map(|event| event.r#type.as_str())
            .collect::<Vec<_>>(),
        ["tool.called", "tool.result", "tool.called"]
    );
    assert_eq!(
        events[2]
            .payload
            .downcast_ref::<ToolCallEvent>()
            .unwrap()
            .tool,
        "handoff"
    );
}

struct Fail;
#[construct_core::async_trait]
impl Tool for Fail {
    fn name(&self) -> &str {
        "read_file"
    }
    fn description(&self) -> &str {
        "test"
    }
    fn input_schema(&self) -> Json {
        json!({})
    }
    async fn execute(&self, _ctx: &ToolContext, _input: Json) -> Result<ToolOutput, ToolError> {
        Err(ToolError::custom("boom", vec![]))
    }
}

#[tokio::test]
async fn catchable_tool_errors_and_uncaught_unwrap() {
    let tools = || vec![host_tool(Arc::new(Fail)), print()];
    let (output, error) = run(
        "try { read_file('/a'); } catch(e) { print(e.message); print(e.details.toolName); }",
        tools(),
        vec![],
    )
    .await;
    assert_eq!(error, None);
    let mut expected = ToolError::custom("boom", vec![]);
    expected.details.insert("toolName".into(), "".into());
    assert_eq!(output.console_output, format!("{expected}\n\n"));
    let (output, error) = run(
        "print('before'); read_file('/a'); print('after');",
        tools(),
        vec![],
    )
    .await;
    assert_eq!(output.console_output, "before\n");
    assert_eq!(error.unwrap().0, expected.to_string());
    let (_, error) = run(
        "try { read_file('/a'); } catch(e) { e.message='changed'; throw e; }",
        tools(),
        vec![],
    )
    .await;
    assert_eq!(error.unwrap().0, expected.to_string());
}

#[tokio::test]
async fn input_errors_and_conversion_exceptions_have_distinct_interceptor_paths() {
    let events = Arc::new(Events::default());
    let (output,error)=run("try{create_file('path',null)}catch(e){print(e.message)};try{fetch(null)}catch(e){print('conversion')}",vec![create_file(),fetch(),print()],production_interceptors(events.clone())).await;
    assert_eq!(error, None);
    assert!(output
        .console_output
        .starts_with("Invalid argument\n\nSuggestions:"));
    assert!(output.console_output.ends_with("conversion\n"));
    assert_eq!(output.tool_stats.unwrap().get("create_file"), Some(&1));
    assert!(output.function_calls.unwrap().is_empty());
    assert!(events.0.lock().unwrap().is_empty());
    let (_, error) = run(
        "let e=new Error('script');e.details={};throw e",
        vec![],
        vec![],
    )
    .await;
    assert_eq!(error.unwrap().0, "Error: script");
}

#[tokio::test]
async fn failed_calls_count_but_do_not_record_or_publish_results() {
    let events = Arc::new(Events::default());
    let tools = vec![host_tool(Arc::new(Fail)), print(), submit_report()];
    let (output,error)=run("try{read_file('/missing')}catch(e){};print('ok');submit_report({summary:'done',completed:true});",tools,production_interceptors(events.clone())).await;
    assert_eq!(error, None);
    assert_eq!(output.console_output, "ok\nREPORT SUBMITTED\n");
    assert_eq!(
        output.tool_stats.as_ref().unwrap().get("read_file"),
        Some(&1)
    );
    assert_eq!(output.tool_stats.as_ref().unwrap().get("print"), None);
    let calls = output.function_calls.unwrap();
    assert_eq!(calls.len(), 1);
    assert_eq!(calls[0].index, 0);
    assert_eq!(calls[0].tool_name, "submit_report");
    let events = events.0.lock().unwrap();
    assert_eq!(events.len(), 3);
    assert_eq!(
        events.iter().map(|e| e.r#type.as_str()).collect::<Vec<_>>(),
        ["tool.called", "tool.called", "tool.result"]
    );
    assert_eq!(events[0].action, "created");
    assert_eq!(events[0].task_id, Some(Uuid::from_u128(1)));
    let call = events[0].payload.downcast_ref::<ToolCallEvent>().unwrap();
    assert_eq!(call.id, "");
    assert_eq!(call.tool, "read_file");
    let result = events[2].payload.downcast_ref::<ToolResultEvent>().unwrap();
    assert_eq!(result.id, "");
    assert!(result.output.submit_report.is_some());
}

struct Trace(&'static str);
impl Interceptor for Trace {
    fn intercept<'js>(
        &self,
        _ctx: &Ctx<'js>,
        session: &RefCell<Session>,
        _tool: &dyn CodeActTool,
        _call: &Invocation<'_, 'js>,
        next: &Next<'_, 'js>,
    ) -> rquickjs::Result<Value<'js>> {
        session.borrow_mut().console.push_str(self.0);
        let result = next(session);
        session.borrow_mut().console.push_str(self.0);
        result
    }
}

#[tokio::test]
async fn last_interceptor_is_outermost() {
    let (out, error) = run(
        "print('x')",
        vec![print()],
        vec![Arc::new(Trace("A")), Arc::new(Trace("B"))],
    )
    .await;
    assert_eq!(error, None);
    assert_eq!(out.console_output, "BAx\nAB");
}

#[test]
fn strict_mode_prefix_and_schema() {
    assert_eq!(ensure_strict_mode("use strict;x"), "use strict;x");
    assert_eq!(
        ensure_strict_mode("'use strict';x"),
        "'use strict';\n'use strict';x"
    );
    assert_eq!(ensure_strict_mode(""), "'use strict';\n");
    let interpreter = Interpreter::new(vec![], vec![]);
    assert_eq!(interpreter.name(), "code_interpreter");
    assert_eq!(
        interpreter.input_schema(),
        json!({"type":"object","properties":{"script":{"type":"string"}}})
    );
}

#[tokio::test]
async fn strict_errors_and_empty_script() {
    let (output, error) = run("", vec![], vec![]).await;
    assert_eq!(error, None);
    assert_eq!(
        construct_types::to_go_json(&output).unwrap(),
        br#"{"console_output":"","function_calls":[]}"#
    );
    let (_, error) = run("undeclared = 1", vec![], vec![]).await;
    assert!(error.unwrap().0.starts_with("ReferenceError:"));
    let (_, error) = run("throw new Error('oops')", vec![], vec![]).await;
    assert!(error.unwrap().0.starts_with("Error: oops"));
}

#[tokio::test]
async fn cancels_infinite_javascript() {
    let (ctx, task) = scope(Path::new("/project"));
    let token = ctx.cancellation.clone();
    let cancel = tokio::spawn(async move {
        tokio::time::sleep(std::time::Duration::from_millis(20)).await;
        token.cancel();
    });
    let (output, error) = tokio::time::timeout(
        std::time::Duration::from_secs(5),
        Interpreter::new(vec![], vec![]).interpret(
            ctx,
            InterpreterInput {
                script: "while(true){}".into(),
            },
            task,
        ),
    )
    .await
    .unwrap();
    cancel.await.unwrap();
    assert_eq!(error.unwrap().0, "execution cancelled");
    assert_eq!(output.console_output, "");
}

#[tokio::test]
async fn real_tools_durable_json_and_fresh_sessions() {
    let dir = tempfile::tempdir().unwrap();
    let file = dir.path().join("sample.txt");
    let events = Arc::new(Events::default());
    let interpreter = Interpreter::new(
        vec![
            create_file(),
            read_file(),
            edit_file(),
            list_files(),
            execute_command(),
            print(),
        ],
        production_interceptors(events),
    );
    let script=format!("create_file({},'first\\n'); edit_file({},[{{old:'first',new:'second'}}]); print(read_file({}).content); print(execute_command('pwd').stdout);",json!(file),json!(file),json!(file));
    let (ctx, task) = scope(dir.path());
    let (output, error) = interpreter
        .interpret(ctx, InterpreterInput { script }, task)
        .await;
    assert_eq!(error, None);
    assert_eq!(std::fs::read_to_string(&file).unwrap(), "second\n");
    assert!(output.console_output.contains("second"));
    assert!(output.console_output.contains(dir.path().to_str().unwrap()));
    let calls = output.function_calls.as_ref().unwrap();
    assert_eq!(calls.len(), 4);
    assert_eq!(
        calls.iter().map(|c| c.index).collect::<Vec<_>>(),
        [0, 1, 2, 3]
    );
    let encoded = construct_types::to_go_json(&output).unwrap();
    let parsed: Json = serde_json::from_slice(&encoded).unwrap();
    assert_eq!(
        parsed["function_calls"][0]["input"]["create_file"],
        json!({"Path":file,"Content":"first\n"})
    );
    assert_eq!(
        parsed["function_calls"][0]["output"]["create_file"],
        json!({"overwritten":false})
    );
    let (ctx, task) = scope(dir.path());
    let (output, error) = interpreter
        .interpret(ctx, InterpreterInput::default(), task)
        .await;
    assert_eq!(error, None);
    assert!(output.function_calls.unwrap().is_empty());
    assert!(output.tool_stats.unwrap().is_empty());
}
