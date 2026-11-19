use construct_connect::Request;
use construct_proto::construct::v1::*;

pub fn command() -> clap::Command {
    clap::Command::new("resume")
        .about("Continue a previous task")
        .display_order(1)
        .arg(clap::Arg::new("arguments").num_args(0..))
        .arg(clap::Arg::new("last").long("last").action(clap::ArgAction::SetTrue).help("Immediately resume the most recent session without showing the interactive picker"))
        .arg(clap::Arg::new("all").long("all").action(clap::ArgAction::SetTrue).help("Show all tasks in the picker, including non-interactive ones"))
        .arg(clap::Arg::new("limit").long("limit").default_value("10").allow_hyphen_values(true).value_parser(clap::value_parser!(i64)).help("Maximum number of tasks to show in the picker"))
        .arg(clap::Arg::new("agent").long("agent").default_value("").help("Resume the task with a specific agent"))
}
fn validate_task_id(id: &str) -> crate::Result<()> {
    if id.len() < 8 {
        return Err("task ID must be at least 8 characters long".into());
    }
    Ok(())
}
pub async fn resolve_task(client: &crate::app::ApiClient, id: &str) -> crate::Result<Task> {
    validate_task_id(id)?;
    if let Ok(uuid) = uuid::Uuid::parse_str(id) {
        return client
            .task
            .get_task(Request::new(GetTaskRequest {
                id: uuid.to_string(),
            }))
            .await
            .map_err(|e| format!("failed to get task {id}: {e}"))?
            .message
            .task
            .ok_or_else(|| "missing task".into());
    }
    let tasks = client
        .task
        .list_tasks(Request::new(ListTasksRequest {
            filter: Some(list_tasks_request::Filter {
                task_id_prefix: Some(id.into()),
                ..Default::default()
            }),
            ..Default::default()
        }))
        .await
        .map_err(|e| format!("failed to list tasks: {e}"))?
        .message
        .tasks;
    match tasks.as_slice() {
        [] => Err(format!("no task found matching {id}").into()),
        [task] => Ok(task.clone()),
        _ => Err(format!("multiple tasks found matching {id}").into()),
    }
}
pub fn list_request(last: bool, all: bool, limit: i64) -> ListTasksRequest {
    ListTasksRequest {
        filter: Some(list_tasks_request::Filter {
            has_messages: if last || all { None } else { Some(true) },
            ..Default::default()
        }),
        page_size: Some(if last { 1 } else { limit as i32 }),
        sort_field: Some(if last {
            SortField::UpdatedAt
        } else {
            SortField::CreatedAt
        } as i32),
        sort_order: Some(SortOrder::Desc as i32),
        ..Default::default()
    }
}
pub async fn run(app: &mut crate::app::App, matches: &clap::ArgMatches) -> crate::Result<()> {
    let client = app.client()?;
    let task = if let Some(id) = matches
        .get_many::<String>("arguments")
        .and_then(|mut args| args.next())
    {
        resolve_task(client, id)
            .await
            .map_err(|e| format!("failed to resolve task ID {id}: {e}"))?
    } else {
        let last = matches.get_flag("last");
        let tasks = client
            .task
            .list_tasks(Request::new(list_request(
                last,
                matches.get_flag("all"),
                *matches.get_one::<i64>("limit").unwrap(),
            )))
            .await
            .map_err(|e| format!("failed to list tasks: {e}"))?
            .message
            .tasks;
        if tasks.is_empty() {
            return Err(if last {
                "no tasks created yet"
            } else {
                "no tasks found"
            }
            .into());
        }
        if last {
            let id = &tasks[0]
                .metadata
                .as_ref()
                .ok_or("missing task metadata")?
                .id;
            resolve_task(client, id)
                .await
                .map_err(|e| format!("failed to resolve task ID {id}: {e}"))?
        } else {
            let Some(task) = crate::terminal::ui::pick_task(tasks, app.cancellation.clone())
                .await
                .map_err(|e| format!("error running task picker: {e}"))?
            else {
                return Ok(());
            };
            task
        }
    };
    let name = matches.get_one::<String>("agent").unwrap();
    let id = if name.is_empty() {
        task.spec
            .as_ref()
            .and_then(|s| s.agent_id.clone())
            .ok_or("missing task agent")?
    } else {
        crate::terminal::resolve_agent(client, name)
            .await
            .map_err(|e| format!("failed to get agent {name}: {e}"))?
    };
    let agent = client
        .agent
        .get_agent(Request::new(GetAgentRequest { id: id.clone() }))
        .await
        .map_err(|e| format!("failed to get agent {id}: {e}"))?
        .message
        .agent
        .ok_or("missing agent")?;
    println!(
        "Subscribed to task {}",
        task.metadata.as_ref().ok_or("missing task metadata")?.id
    );
    crate::terminal::ui::run(client, task, agent, app.cancellation.clone()).await
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn flags_and_selection() {
        let args = command()
            .try_get_matches_from([
                "resume", "prefix12", "ignored", "--last", "--all", "--limit", "-1",
            ])
            .unwrap();
        assert!(args.get_flag("all"));
        assert_eq!(
            list_request(false, false, 10).filter.unwrap().has_messages,
            Some(true)
        );
        assert_eq!(list_request(true, false, 10).page_size, Some(1));
        assert_eq!(
            list_request(true, false, 10).sort_field,
            Some(SortField::UpdatedAt as i32)
        );
        assert_eq!(
            list_request(true, false, 10).filter.unwrap().has_messages,
            None
        );
        assert_eq!(list_request(false, false, -1).page_size, Some(-1));
        assert_eq!(
            list_request(false, false, 10).sort_field,
            Some(SortField::CreatedAt as i32)
        );
        assert_eq!(
            list_request(false, false, 10).sort_order,
            Some(SortOrder::Desc as i32)
        );
        assert!(command()
            .try_get_matches_from(["resume", "--limit", "bad"])
            .is_err());
    }
    #[test]
    fn all_includes_tasks_without_messages() {
        for (flags, has_messages) in [
            (vec!["resume"], Some(true)),
            (vec!["resume", "--all"], None),
        ] {
            let args = command().try_get_matches_from(flags).unwrap();
            let request = list_request(
                args.get_flag("last"),
                args.get_flag("all"),
                *args.get_one::<i64>("limit").unwrap(),
            );
            assert_eq!(request.filter.unwrap().has_messages, has_messages);
            assert_eq!(request.page_size, Some(10));
            assert_eq!(request.sort_field, Some(SortField::CreatedAt as i32));
        }
    }
    #[test]
    fn prefix_minimum_is_eight_characters() {
        for id in ["", "1234", "1234567"] {
            assert_eq!(
                validate_task_id(id).unwrap_err().to_string(),
                "task ID must be at least 8 characters long"
            );
        }
        for id in ["12345678", "00000000-0000-0000-0000-000000000001"] {
            validate_task_id(id).unwrap();
        }
    }
}
