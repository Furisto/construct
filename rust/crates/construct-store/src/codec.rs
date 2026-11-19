use crate::{database_error, EntityKind, HookRecord, Mutation};
use construct_core::store::{ListOptions, SortOrder, StoreError};
use construct_types::{entity::*, Timestamp};
use serde::{de::DeserializeOwned, Serialize};
use sqlx::{
    query::Query,
    sqlite::{SqliteArguments, SqliteRow},
    Row, Sqlite, SqliteConnection,
};
use uuid::Uuid;

pub(crate) enum Value {
    Text(String),
    Integer(i64),
    Real(f64),
    Bytes(Vec<u8>),
    Null,
}

fn corrupt() -> StoreError {
    StoreError::Internal("invalid stored record".into())
}
fn invalid() -> StoreError {
    StoreError::Invalid("invalid record value".into())
}

fn time_text(t: &Timestamp) -> Result<String, StoreError> {
    construct_types::to_go_json(t).map_err(|_| invalid())?;
    Ok(t.0.format("%Y-%m-%d %H:%M:%S%.f +0000 UTC").to_string())
}

fn parse_time(s: &str) -> Result<Timestamp, StoreError> {
    let s = s.split(" m=").next().unwrap_or(s).trim();
    if let Ok(t) = chrono::DateTime::parse_from_rfc3339(s) {
        return Ok(Timestamp(t.to_utc()));
    }
    for format in [
        "%Y-%m-%d %H:%M:%S%.f %z %Z",
        "%Y-%m-%d %H:%M:%S%.f%:z",
        "%Y-%m-%dT%H:%M:%S%.f%:z",
    ] {
        if let Ok(t) = chrono::DateTime::parse_from_str(s, format) {
            return Ok(Timestamp(t.to_utc()));
        }
    }
    for format in ["%Y-%m-%d %H:%M:%S%.f", "%Y-%m-%dT%H:%M:%S%.f"] {
        if let Ok(t) = chrono::NaiveDateTime::parse_from_str(s, format) {
            return Ok(Timestamp(t.and_utc()));
        }
    }
    Err(corrupt())
}

fn text(row: &SqliteRow, key: &str) -> Result<String, StoreError> {
    row.try_get_unchecked::<Option<String>, _>(key)
        .map(|v| v.unwrap_or_default())
        .map_err(database_error)
}
fn opt_text(row: &SqliteRow, key: &str) -> Result<Option<String>, StoreError> {
    row.try_get_unchecked(key).map_err(database_error)
}
fn json<T: Serialize>(value: &T) -> Result<Value, StoreError> {
    Ok(Value::Bytes(
        construct_types::to_go_json(value).map_err(|_| invalid())?,
    ))
}
fn read_json<T: DeserializeOwned>(row: &SqliteRow, key: &str) -> Result<T, StoreError> {
    serde_json::from_str(&opt_text(row, key)?.unwrap_or_else(|| "null".into()))
        .map_err(|_| corrupt())
}

macro_rules! encode {
    ($v:expr, text) => {
        Value::Text($v.clone())
    };
    ($v:expr, enum) => {
        Value::Text($v.0.clone())
    };
    ($v:expr, integer) => {
        Value::Integer($v)
    };
    ($v:expr, bool) => {
        Value::Integer(i64::from($v))
    };
    ($v:expr, real) => {
        if $v.is_finite() {
            Value::Real($v)
        } else {
            return Err(invalid());
        }
    };
    ($v:expr, bytes) => {
        Value::Bytes($v.clone())
    };
    ($v:expr, uuid) => {
        Value::Text($v.to_string())
    };
    ($v:expr, opt_uuid) => {
        $v.map(|id| Value::Text(id.to_string()))
            .unwrap_or(Value::Null)
    };
    ($v:expr, time) => {
        Value::Text(time_text(&$v)?)
    };
    ($v:expr, opt_time) => {
        $v.as_ref()
            .map(time_text)
            .transpose()?
            .map(Value::Text)
            .unwrap_or(Value::Null)
    };
    ($v:expr, json) => {
        json(&$v)?
    };
    ($v:expr, opt_json) => {
        if $v.is_some() {
            json(&$v)?
        } else {
            Value::Null
        }
    };
}
macro_rules! decode {
    ($r:ident, $k:expr, text) => {
        text($r, $k)?
    };
    ($r:ident, $k:expr, enum) => {
        text($r, $k)?.as_str().into()
    };
    ($r:ident, $k:expr, integer) => {
        $r.try_get::<Option<i64>, _>($k)
            .map_err(database_error)?
            .unwrap_or_default()
    };
    ($r:ident, $k:expr, bool) => {
        $r.try_get::<bool, _>($k).map_err(database_error)?
    };
    ($r:ident, $k:expr, real) => {
        $r.try_get::<Option<f64>, _>($k)
            .map_err(database_error)?
            .unwrap_or_default()
    };
    ($r:ident, $k:expr, bytes) => {
        $r.try_get::<Vec<u8>, _>($k).map_err(database_error)?
    };
    ($r:ident, $k:expr, uuid) => {
        Uuid::parse_str(&text($r, $k)?).map_err(|_| corrupt())?
    };
    ($r:ident, $k:expr, opt_uuid) => {
        opt_text($r, $k)?
            .map(|s| Uuid::parse_str(&s).map_err(|_| corrupt()))
            .transpose()?
    };
    ($r:ident, $k:expr, time) => {
        parse_time(&text($r, $k)?)?
    };
    ($r:ident, $k:expr, opt_time) => {
        opt_text($r, $k)?.map(|s| parse_time(&s)).transpose()?
    };
    ($r:ident, $k:expr, json) => {
        read_json($r, $k)?
    };
    ($r:ident, $k:expr, opt_json) => {
        read_json($r, $k)?
    };
}

pub(crate) trait Record: Clone + Send + Sync + 'static {
    const TABLE: &'static str;
    const COLUMNS: &'static [&'static str];
    fn id(&self) -> Uuid;
    fn created(&self) -> &Timestamp;
    fn updated(&self) -> &Timestamp;
    fn name(&self) -> Option<&str>;
    fn values(&self) -> Result<Vec<Value>, StoreError>;
    fn decode(row: &SqliteRow) -> Result<Self, StoreError>;
    fn validate(&self) -> Result<(), StoreError>;
    fn hook(&self) -> Option<HookRecord>;
    fn kind() -> Option<EntityKind>;
}

macro_rules! record_name {
    ($r:ident, named) => {
        Some(&$r.name)
    };
    ($r:ident, unnamed) => {
        None
    };
}

macro_rules! record {
    ($t:ident, $table:literal, $name:ident, $kind:expr, $hook:expr, $validate:expr, {$($field:ident: $codec:ident),* $(,)?}) => {
        impl Record for $t {
            const TABLE: &'static str = $table;
            const COLUMNS: &'static [&'static str] = &["id", "create_time", "update_time", $(stringify!($field)),*];
            fn id(&self) -> Uuid { self.id }
            fn created(&self) -> &Timestamp { &self.create_time }
            fn updated(&self) -> &Timestamp { &self.update_time }
            fn name(&self) -> Option<&str> { record_name!(self, $name) }
            fn hook(&self) -> Option<HookRecord> { ($hook)(self) }
            fn kind() -> Option<EntityKind> { $kind }
            fn validate(&self) -> Result<(), StoreError> { ($validate)(self) }
            fn values(&self) -> Result<Vec<Value>, StoreError> {
                Ok(vec![encode!(self.id, uuid), encode!(self.create_time, time), encode!(self.update_time, time), $(encode!(self.$field, $codec)),*])
            }
            fn decode(row: &SqliteRow) -> Result<Self, StoreError> {
                Ok(Self { id: decode!(row, "id", uuid), create_time: decode!(row, "create_time", time), update_time: decode!(row, "update_time", time), $($field: decode!(row, stringify!($field).trim_start_matches("r#"), $codec)),* })
            }
        }
    };
}
fn require(condition: bool) -> Result<(), StoreError> {
    if condition {
        Ok(())
    } else {
        Err(StoreError::ConstraintViolation(
            "record validation failed".into(),
        ))
    }
}
record!(Agent, "agents", named, Some(EntityKind::Agent), |r: &Agent| Some(HookRecord::Agent(r.clone())), |r: &Agent| require(!r.name.is_empty()), {
    name: text, description: text, instructions: text, builtin: bool, model_id: opt_uuid
});
record!(Task, "tasks", unnamed, Some(EntityKind::Task), |r: &Task| Some(HookRecord::Task(r.clone())), |r: &Task| require(["unspecified", "running", "awaiting", "suspended"].contains(&r.phase.as_ref()) && ["unspecified", "running", "awaiting", "suspended"].contains(&r.desired_phase.as_ref())), {
    project_directory: text, input_tokens: integer, output_tokens: integer, cache_write_tokens: integer, cache_read_tokens: integer, cost: real, turns: integer, tool_uses: json, desired_phase: enum, phase: enum, description: text, agent_id: opt_uuid
});
record!(Message, "messages", unnamed, None, |_: &Message| None, |r: &Message| require(["user", "assistant", "system"].contains(&r.source.as_ref())), {
    source: enum, content: json, usage: opt_json, processed_time: opt_time, task_id: uuid, agent_id: opt_uuid, model_id: opt_uuid
});
record!(Model, "models", named, Some(EntityKind::Model), |r: &Model| Some(HookRecord::Model(r.clone())), |r: &Model| require([r.input_cost, r.output_cost, r.cache_write_cost, r.cache_read_cost].iter().all(|n| n.is_finite() && *n >= 0.0)), {
    name: text, context_window: integer, capabilities: opt_json, input_cost: real, output_cost: real, cache_write_cost: real, cache_read_cost: real, enabled: bool, alias: text, model_provider_id: uuid
});
record!(ModelProvider, "model_providers", named, Some(EntityKind::ModelProvider), |r: &ModelProvider| Some(HookRecord::ModelProvider(r.clone())), |r: &ModelProvider| require(!r.name.is_empty() && !r.secret.is_empty() && ["anthropic", "openai", "gemini", "xai"].contains(&r.provider_type.as_ref())), {
    name: text, provider_type: enum, url: text, secret: bytes, enabled: bool
});
record!(Token, "tokens", named, None, |_: &Token| None, |r: &Token| require(!r.name.is_empty() && ["setup_code", "api_token"].contains(&r.r#type.as_ref())), {
    name: text, r#type: enum, token_hash: bytes, description: text, expires_at: time
});

fn bind<'a>(
    mut query: Query<'a, Sqlite, SqliteArguments<'a>>,
    values: Vec<Value>,
) -> Query<'a, Sqlite, SqliteArguments<'a>> {
    for value in values {
        query = match value {
            Value::Text(v) => query.bind(v),
            Value::Integer(v) => query.bind(v),
            Value::Real(v) => query.bind(v),
            Value::Bytes(v) => query.bind(v),
            Value::Null => query.bind(None::<String>),
        };
    }
    query
}
pub(crate) async fn get<T: Record>(
    connection: &mut SqliteConnection,
    id: Uuid,
) -> Result<T, StoreError> {
    let sql = format!("SELECT * FROM `{}` WHERE id = ?", T::TABLE);
    let row = sqlx::query(&sql)
        .bind(id.to_string())
        .fetch_one(connection)
        .await
        .map_err(database_error)?;
    T::decode(&row)
}
#[derive(Default)]
pub(crate) struct Selection {
    predicates: Vec<String>,
    values: Vec<Value>,
}

impl Selection {
    pub(crate) fn predicate(&mut self, sql: &str, values: Vec<Value>) {
        self.predicates.push(sql.into());
        self.values.extend(values);
    }

    pub(crate) fn equal(&mut self, column: &str, value: Option<Value>) {
        if let Some(value) = value {
            self.predicate(&format!("`{column}` = ?"), vec![value]);
        }
    }

    pub(crate) fn one_of(&mut self, column: &str, values: Vec<Value>) {
        if !values.is_empty() {
            self.predicate(
                &format!("`{column}` IN ({})", vec!["?"; values.len()].join(", ")),
                values,
            );
        }
    }

    pub(crate) async fn fetch<T: Record>(
        mut self,
        connection: &mut SqliteConnection,
        name_page: Option<&ListOptions>,
    ) -> Result<Vec<T>, StoreError> {
        let mut sql = format!("SELECT * FROM `{}`", T::TABLE);
        if !self.predicates.is_empty() {
            sql.push_str(" WHERE ");
            sql.push_str(&self.predicates.join(" AND "));
        }
        if let Some(options) = name_page {
            let Ok(offset) = i64::try_from(options.offset) else {
                return Ok(Vec::new());
            };
            let direction = match options.sort_order {
                SortOrder::Ascending => "ASC",
                SortOrder::Descending => "DESC",
            };
            sql.push_str(&format!(
                " ORDER BY name {direction}, id {direction} LIMIT ? OFFSET ?"
            ));
            self.values.push(Value::Integer(
                options
                    .limit
                    .map(|n| i64::try_from(n).unwrap_or(i64::MAX))
                    .unwrap_or(-1),
            ));
            self.values.push(Value::Integer(offset));
        }
        bind(sqlx::query(&sql), self.values)
            .fetch_all(connection)
            .await
            .map_err(database_error)?
            .iter()
            .map(T::decode)
            .collect()
    }
}
pub(crate) async fn write<T: Record>(
    connection: &mut SqliteConnection,
    record: T,
    update: bool,
) -> Result<(T, Option<Mutation>), StoreError> {
    if update {
        let old: T = get(connection, record.id()).await?;
        if old.created() != record.created() {
            return Err(StoreError::Invalid("create_time is immutable".into()));
        }
    }
    record.validate()?;
    let columns: Vec<_> = T::COLUMNS
        .iter()
        .map(|s| format!("`{}`", s.trim_start_matches("r#")))
        .collect();
    let mut values = record.values()?;
    let sql = if update {
        let assignments: Vec<_> = columns.iter().skip(2).map(|s| format!("{s} = ?")).collect();
        values.drain(0..2);
        values.push(Value::Text(record.id().to_string()));
        format!(
            "UPDATE `{}` SET {} WHERE id = ?",
            T::TABLE,
            assignments.join(", ")
        )
    } else {
        format!(
            "INSERT INTO `{}` ({}) VALUES ({})",
            T::TABLE,
            columns.join(", "),
            vec!["?"; columns.len()].join(", ")
        )
    };
    bind(sqlx::query(&sql), values)
        .execute(connection)
        .await
        .map_err(database_error)?;
    let mutation = record.hook().and_then(|r| {
        if update {
            if matches!(r, HookRecord::Task(_)) {
                None
            } else {
                Some(Mutation::Update(r))
            }
        } else {
            Some(Mutation::Create(r))
        }
    });
    Ok((record, mutation))
}
pub(crate) async fn delete<T: Record>(
    connection: &mut SqliteConnection,
    id: Uuid,
) -> Result<((), Option<Mutation>), StoreError> {
    let result = sqlx::query(&format!("DELETE FROM `{}` WHERE id = ?", T::TABLE))
        .bind(id.to_string())
        .execute(connection)
        .await
        .map_err(database_error)?;
    if result.rows_affected() == 0 {
        return Err(StoreError::NotFound(id.to_string()));
    }
    Ok(((), T::kind().map(|kind| Mutation::Delete(kind, id))))
}
