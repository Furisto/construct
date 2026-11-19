use crate::{app::App, terminal, Result};
use clap::{Arg, ArgAction, Command};
use construct_connect::Request;
use construct_proto::construct::v1::*;
use futures_util::StreamExt;
use std::io::{IsTerminal, Read, Write};

pub fn command() -> Command {
    Command::new("exec")
        .about("Execute a non-interactive task with an agent")
        .display_order(2)
        .arg(Arg::new("question").index(1))
        .arg(Arg::new("agent").long("agent").short('a').default_value("").help("Specify the agent to use by its name or ID"))
        .arg(
            Arg::new("workspace")
                .long("workspace")
                .short('w')
                .help("Set the agent's working directory")
                .default_value(""),
        )
        .arg(
            Arg::new("max-turns")
                .long("max-turns")
                .help("Set a maximum number of conversational turns for the agent to complete the task")
                .default_value("5")
                .allow_hyphen_values(true)
                .value_parser(clap::value_parser!(i64)),
        )
        .arg(
            Arg::new("file")
                .long("file")
                .short('f')
                .help("Add a file to the agent's context. Can be used multiple times")
                .action(ArgAction::Append)
                .value_parser(parse_files),
        )
        .arg(
            Arg::new("continue")
                .long("continue")
                .short('c')
                .help("Continue the most recent task with this new question")
                .num_args(0..=1)
                .require_equals(true)
                .default_missing_value("last"),
        )
        .arg(
            Arg::new("output")
                .long("output")
                .short('o')
                .help("The format to output the result in")
                .default_value("text")
                .value_parser(parse_format),
        )
}

fn parse_format(s: &str) -> std::result::Result<String, String> {
    match s {
        "text" | "json" | "yaml" => Ok(s.into()),
        _ => Err("must be one of \"text\", \"json\", or \"yaml\"".into()),
    }
}

fn parse_files(s: &str) -> std::result::Result<Vec<String>, String> {
    if s.is_empty() {
        return Ok(Vec::new());
    }
    csv::ReaderBuilder::new()
        .has_headers(false)
        .from_reader(s.as_bytes())
        .records()
        .next()
        .transpose()
        .map(|record| {
            record
                .map(|r| r.iter().map(str::to_owned).collect())
                .unwrap_or_default()
        })
        .map_err(|e| e.to_string())
}

pub fn prepare_question(question: &str, stdin: &str, files: &[String]) -> Result<String> {
    let stdin = stdin.trim();
    let mut question = match (question.is_empty(), stdin.is_empty()) {
        (true, true) => {
            return Err("no question provided - provide as argument or pipe via stdin".into())
        }
        (false, false) => format!("{question}\n\n{stdin}"),
        (true, false) => stdin.into(),
        (false, true) => question.into(),
    };
    if !files.is_empty() {
        question.push_str("\n--- File Context ---\n");
        for (i, path) in files.iter().enumerate() {
            let content =
                std::fs::read(path).map_err(|e| format!("failed to read file {path}: {e}"))?;
            question.push_str(&format!(
                "### {path}\n```\n{}\n```",
                String::from_utf8_lossy(&content)
            ));
            if i + 1 < files.len() {
                question.push_str("\n\n");
            }
        }
    }
    Ok(question)
}

pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let mut input = String::new();
    if !std::io::stdin().is_terminal() {
        app.stdin
            .read_to_string(&mut input)
            .map_err(|e| format!("failed to read input from stdin: {e}"))?;
    }
    let files: Vec<_> = matches
        .get_many::<Vec<String>>("file")
        .into_iter()
        .flatten()
        .flatten()
        .cloned()
        .collect();
    let question = prepare_question(
        matches
            .get_one::<String>("question")
            .map(String::as_str)
            .unwrap_or_default(),
        &input,
        &files,
    )?;
    let workspace = matches.get_one::<String>("workspace").unwrap();
    let workspace = if workspace.is_empty() {
        std::env::current_dir()?.to_string_lossy().into_owned()
    } else {
        workspace.clone()
    };
    let client = app.client()?;
    let agent =
        terminal::resolve_agent(client, matches.get_one::<String>("agent").unwrap()).await?;
    let task = if let Some(id) = matches.get_one::<String>("continue") {
        terminal::continue_task(client, id).await?
    } else {
        terminal::create_task(client, &agent, &workspace).await?
    };
    let id = task
        .metadata
        .as_ref()
        .ok_or("missing task metadata")?
        .id
        .clone();
    let sent = terminal::send_message(client, &id, &question).await?;
    let sent_id = sent
        .metadata
        .as_ref()
        .ok_or("missing message metadata")?
        .id
        .clone();
    let mut stream = client
        .event
        .subscribe(Request::new(EventSubscribeRequest {
            event_types: vec!["message.*".into(), "task.*".into()],
            task_id: Some(id.clone()),
            ..Default::default()
        }))
        .await
        .map_err(|e| format!("failed to subscribe to task: {e}"))?
        .message;
    let mut seen_prompt = false;
    loop {
        let next = tokio::select! {
            _ = app.cancellation.cancelled() => break,
            next = stream.next() => next,
        };
        let Some(next) = next else { break };
        let event = next.map_err(|e| format!("stream error: {e}"))?.event;
        match event.and_then(|e| e.payload) {
            Some(event::Payload::Message(payload)) => {
                let Some(message) = payload.message else {
                    continue;
                };
                if message.metadata.as_ref().is_some_and(|m| m.id == sent_id) {
                    seen_prompt = true;
                }
                let task = terminal::get_task(app.client()?, &id).await?;
                format_message(
                    &mut app.stdout,
                    &task,
                    &message,
                    matches.get_one::<String>("output").unwrap(),
                )?;
                app.stdout.flush()?;
                if seen_prompt && terminal::is_complete_response(&message) {
                    break;
                }
            }
            Some(event::Payload::Task(payload)) if seen_prompt => {
                if payload
                    .task
                    .and_then(|t| t.status)
                    .is_some_and(|s| s.phase == TaskPhase::Suspended as i32)
                {
                    break;
                }
            }
            _ => {}
        }
    }
    Ok(())
}

pub fn format_message(
    out: &mut dyn Write,
    task: &Task,
    message: &Message,
    format: &str,
) -> Result<()> {
    let texts = terminal::text_parts(message);
    if format == "text" {
        for text in texts {
            writeln!(out, "{text}")?;
        }
        return Ok(());
    }
    let answer = terminal::DisplayAnswer::new(task, message);
    match format {
        "json" => writeln!(
            out,
            "{}",
            String::from_utf8(construct_types::to_go_json(&answer)?)?
        )?,
        "yaml" => {
            let mut value = serde_yaml::to_value(&answer)?;
            value["usage"]["cost"] = serde_yaml::from_str(&String::from_utf8(
                construct_types::to_go_json(&answer.usage.cost)?,
            )?)?;
            if answer.usage.tool_uses.is_none() {
                value["usage"]["tool_uses"] = serde_yaml::Value::Mapping(Default::default());
            }
            writeln!(out, "{}", crate::yaml::to_string(&value)?)?;
        }
        _ => return Err(format!("unsupported format: {format}").into()),
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn arguments() {
        let args = command()
            .try_get_matches_from(["exec", "hello", "-c", "-f", "a,b", "--max-turns", "-1"])
            .unwrap();
        assert_eq!(args.get_one::<String>("continue").unwrap(), "last");
        assert_eq!(
            args.get_many::<Vec<String>>("file")
                .unwrap()
                .next()
                .unwrap()
                .len(),
            2
        );
        assert_eq!(parse_files("\"a,b\",c").unwrap(), ["a,b", "c"]);
        assert!(command().try_get_matches_from(["exec", "a", "b"]).is_err());
        assert!(command()
            .try_get_matches_from(["exec", "-o", "table"])
            .is_err());
        assert!(command()
            .try_get_matches_from(["exec", "--max-turns", "x"])
            .is_err());
    }
    #[test]
    fn files_and_output() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("file");
        std::fs::write(&path, "hello").unwrap();
        let name = path.to_string_lossy().into_owned();
        assert_eq!(
            prepare_question("q", "", std::slice::from_ref(&name)).unwrap(),
            format!("q\n--- File Context ---\n### {name}\n```\nhello\n```")
        );
        let message = Message {
            spec: Some(MessageSpec {
                content: vec![MessagePart {
                    data: Some(message_part::Data::Text(message_part::Text {
                        content: "<answer>".into(),
                    })),
                }],
            }),
            ..Default::default()
        };
        let mut out = Vec::new();
        format_message(&mut out, &Task::default(), &message, "text").unwrap();
        assert_eq!(out, b"<answer>\n");
        out.clear();
        format_message(&mut out, &Task::default(), &message, "json").unwrap();
        assert_eq!(String::from_utf8(out).unwrap(), "{\"task_id\":\"\",\"agent\":\"\",\"model\":\"\",\"turn\":0,\"result\":\"\\u003canswer\\u003e\",\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"cache_write_tokens\":0,\"cache_read_tokens\":0,\"cost\":0,\"tool_uses\":null}}\n");
        assert!(format_message(&mut Vec::new(), &Task::default(), &message, "bad").is_err());
        let mut yaml = Vec::new();
        format_message(&mut yaml, &Task::default(), &message, "yaml").unwrap();
        let yaml = String::from_utf8(yaml).unwrap();
        assert!(yaml.contains("result: <answer>\n"));
        assert!(yaml.contains("    cost: 0\n"));
        let mut empty = Vec::new();
        format_message(&mut empty, &Task::default(), &Message::default(), "text").unwrap();
        assert!(empty.is_empty());
    }

    struct Fake;
    fn task() -> Task {
        Task {
            metadata: Some(TaskMetadata {
                id: "task".into(),
                ..Default::default()
            }),
            ..Default::default()
        }
    }
    #[construct_connect::async_trait]
    impl TaskService for Fake {
        async fn create_task(
            &self,
            _: Request<CreateTaskRequest>,
        ) -> std::result::Result<
            construct_connect::Response<CreateTaskResponse>,
            construct_connect::ConnectError,
        > {
            Ok(construct_connect::Response::new(CreateTaskResponse {
                task: Some(task()),
            }))
        }
        async fn get_task(
            &self,
            _: Request<GetTaskRequest>,
        ) -> std::result::Result<
            construct_connect::Response<GetTaskResponse>,
            construct_connect::ConnectError,
        > {
            Ok(construct_connect::Response::new(GetTaskResponse {
                task: Some(task()),
            }))
        }
    }
    #[construct_connect::async_trait]
    impl MessageService for Fake {
        async fn create_message(
            &self,
            _: Request<CreateMessageRequest>,
        ) -> std::result::Result<
            construct_connect::Response<CreateMessageResponse>,
            construct_connect::ConnectError,
        > {
            Ok(construct_connect::Response::new(CreateMessageResponse {
                message: Some(Message {
                    metadata: Some(MessageMetadata {
                        id: "prompt".into(),
                        ..Default::default()
                    }),
                    ..Default::default()
                }),
            }))
        }
    }
    #[construct_connect::async_trait]
    impl EventService for Fake {
        async fn subscribe(
            &self,
            _: Request<EventSubscribeRequest>,
        ) -> std::result::Result<
            construct_connect::Response<construct_connect::Streaming<EventSubscribeResponse>>,
            construct_connect::ConnectError,
        > {
            let messages = [
                ("old", MessageRole::Assistant),
                ("prompt", MessageRole::User),
                ("final", MessageRole::Assistant),
            ];
            let events = messages.map(|(id, role)| {
                Ok(EventSubscribeResponse {
                    event: Some(Event {
                        payload: Some(event::Payload::Message(MessageEvent {
                            message: Some(Message {
                                metadata: Some(MessageMetadata {
                                    id: id.into(),
                                    role: role as i32,
                                    ..Default::default()
                                }),
                                spec: Some(MessageSpec::default()),
                                status: Some(MessageStatus {
                                    is_final_response: false,
                                    ..Default::default()
                                }),
                            }),
                        })),
                        ..Default::default()
                    }),
                })
            });
            Ok(construct_connect::Response::new(Box::pin(
                futures_util::stream::iter(events).chain(futures_util::stream::pending()),
            )))
        }
    }
    #[tokio::test]
    async fn exits_after_final_response_on_open_stream() {
        let router = TaskServiceServer::new(Fake)
            .router()
            .merge(MessageServiceServer::new(Fake).router())
            .merge(EventServiceServer::new(Fake).router());
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = listener.local_addr().unwrap();
        let server = tokio::spawn(async move {
            construct_connect::serve_tcp(
                listener,
                construct_connect::Router::new().nest("/api", router),
                std::future::pending(),
            )
            .await
            .unwrap()
        });
        let dir = tempfile::tempdir().unwrap();
        let user = crate::paths::UserInfo {
            home: dir.path().into(),
            config: dir.path().join("config"),
            data: dir.path().join("data"),
            log: dir.path().join("log"),
            runtime: dir.path().join("runtime"),
        };
        let mut app = App::new(user, String::new()).unwrap();
        app.contexts
            .upsert(
                "test",
                crate::context::EndpointContext {
                    address: format!("http://{address}"),
                    kind: "http".into(),
                    ..Default::default()
                },
                true,
            )
            .unwrap();
        app.stdout = Box::new(Vec::<u8>::new());
        app.stdin = Box::new(std::io::empty());
        let args = command()
            .try_get_matches_from([
                "exec",
                "q",
                "--agent",
                "00000000-0000-0000-0000-000000000001",
            ])
            .unwrap();
        tokio::time::timeout(std::time::Duration::from_secs(5), run(&mut app, &args))
            .await
            .expect("exec must not wait for server EOF")
            .unwrap();
        server.abort();
    }
    #[test]
    fn questions() {
        assert_eq!(prepare_question("q", " x\n", &[]).unwrap(), "q\n\nx");
        assert_eq!(prepare_question("", " x\n", &[]).unwrap(), "x");
        assert_eq!(
            prepare_question("", "", &[]).unwrap_err().to_string(),
            "no question provided - provide as argument or pipe via stdin"
        );
        assert!(
            prepare_question("q", "", &["/nonexistent/context-file".into()])
                .unwrap_err()
                .to_string()
                .starts_with("failed to read file /nonexistent/context-file:")
        );
    }
}
