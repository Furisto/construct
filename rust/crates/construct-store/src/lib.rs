//! SQLite repositories sharing the Go daemon's Ent on-disk format.
#![deny(missing_docs)]

mod codec;
mod repositories;

use async_trait::async_trait;
use construct_core::store::*;
use construct_types::entity::*;
use futures_util::future::BoxFuture;
use sqlx::{
    sqlite::{SqliteConnectOptions, SqliteJournalMode, SqlitePoolOptions},
    Sqlite, SqliteConnection, SqlitePool, Transaction,
};
use std::{path::Path, sync::Arc, time::Duration};
use tokio::sync::Mutex;
use uuid::Uuid;

/// Entity kinds covered by the Go event hooks.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum EntityKind {
    /// Agent configuration.
    Agent,
    /// Model configuration.
    Model,
    /// Provider configuration.
    ModelProvider,
    /// Task lifecycle (create/delete only).
    Task,
}

/// Complete scalar record supplied to mutation observers.
#[derive(Clone)]
pub enum HookRecord {
    /// Agent configuration.
    Agent(Agent),
    /// Model configuration.
    Model(Model),
    /// Provider configuration; may contain encrypted secret bytes.
    ModelProvider(ModelProvider),
    /// Task lifecycle.
    Task(Task),
}

/// Synchronous, non-failing observers invoked only after durable commit.
/// Implementations should be nonblocking and must not panic. As in Go, messages,
/// tokens and task updates do not trigger these hooks. Cascades do not emit hooks.
pub trait MutationHook: Send + Sync {
    /// Observe a committed creation.
    fn on_create(&self, _record: &HookRecord) {}
    /// Observe a committed configuration update.
    fn on_update(&self, _record: &HookRecord) {}
    /// Observe a committed deletion.
    fn on_delete(&self, _kind: EntityKind, _id: Uuid) {}
}

enum Mutation {
    Create(HookRecord),
    Update(HookRecord),
    Delete(EntityKind, Uuid),
}

struct TxState {
    transaction: Transaction<'static, Sqlite>,
    mutations: Vec<Mutation>,
}

enum Backend {
    Pool(SqlitePool),
    Transaction(Mutex<Option<TxState>>),
}

/// SQLite store with pooled connections and rollback-on-drop transactions.
/// UUIDs are canonical text, JSON uses Go encoding, times use modernc's format.
pub struct SqliteStore {
    backend: Backend,
    hooks: Vec<Arc<dyn MutationHook>>,
}

fn database_error(error: sqlx::Error) -> StoreError {
    match error {
        sqlx::Error::RowNotFound => StoreError::NotFound("record".into()),
        sqlx::Error::Database(ref e)
            if e.code().is_some_and(|code| {
                code.parse::<i32>()
                    .is_ok_and(|n| matches!(n & 255, 5 | 6 | 19))
            }) =>
        {
            StoreError::ConstraintViolation("SQLite constraint or write conflict".into())
        }
        _ => StoreError::Internal("SQLite operation failed".into()),
    }
}

impl SqliteStore {
    /// Open/create a database and add missing Ent tables/indexes atomically.
    /// Existing tables and data are never rebuilt or dropped. Uses the daemon's
    /// foreign_keys=ON, journal_mode=WAL and busy_timeout=5000 pragmas.
    pub async fn open(path: impl AsRef<Path>) -> Result<Self, StoreError> {
        Self::open_with_hooks(path, Vec::new()).await
    }

    /// Open a store with post-commit observers, in registration order.
    pub async fn open_with_hooks(
        path: impl AsRef<Path>,
        hooks: Vec<Arc<dyn MutationHook>>,
    ) -> Result<Self, StoreError> {
        let options = SqliteConnectOptions::new()
            .filename(path)
            .create_if_missing(true)
            .foreign_keys(true)
            .journal_mode(SqliteJournalMode::Wal)
            .busy_timeout(Duration::from_secs(5));
        let pool = SqlitePoolOptions::new()
            .max_connections(8)
            .connect_with(options)
            .await
            .map_err(database_error)?;
        let mut tx = pool.begin().await.map_err(database_error)?;
        for statement in include_str!("schema.sql")
            .split(';')
            .filter(|s| !s.trim().is_empty())
        {
            let statement = statement
                .trim()
                .replacen("CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1)
                .replacen(
                    "CREATE UNIQUE INDEX ",
                    "CREATE UNIQUE INDEX IF NOT EXISTS ",
                    1,
                )
                .replacen("CREATE INDEX ", "CREATE INDEX IF NOT EXISTS ", 1);
            sqlx::query(&statement)
                .execute(&mut *tx)
                .await
                .map_err(database_error)?;
        }
        tx.commit().await.map_err(database_error)?;
        Ok(Self {
            backend: Backend::Pool(pool),
            hooks,
        })
    }

    fn notify(&self, mutations: &[Mutation]) {
        for mutation in mutations {
            for hook in &self.hooks {
                match mutation {
                    Mutation::Create(r) => hook.on_create(r),
                    Mutation::Update(r) => hook.on_update(r),
                    Mutation::Delete(kind, id) => hook.on_delete(*kind, *id),
                }
            }
        }
    }

    async fn run<T: Send>(
        &self,
        operation: impl for<'a> FnOnce(
                &'a mut SqliteConnection,
            ) -> BoxFuture<'a, Result<(T, Option<Mutation>), StoreError>>
            + Send,
    ) -> Result<T, StoreError> {
        match &self.backend {
            Backend::Pool(pool) => {
                let mut tx = pool.begin().await.map_err(database_error)?;
                let (result, mutation) = operation(&mut tx).await?;
                tx.commit().await.map_err(database_error)?;
                if let Some(mutation) = mutation {
                    self.notify(&[mutation]);
                }
                Ok(result)
            }
            Backend::Transaction(state) => {
                let mut guard = state.lock().await;
                let mut state = guard
                    .take()
                    .ok_or_else(|| StoreError::Invalid("transaction finished".into()))?;
                let result = operation(&mut state.transaction).await;
                match result {
                    Ok((result, mutation)) => {
                        if let Some(mutation) = mutation {
                            state.mutations.push(mutation);
                        }
                        *guard = Some(state);
                        Ok(result)
                    }
                    Err(error) => {
                        *guard = Some(state);
                        Err(error)
                    }
                }
            }
        }
    }
}

#[async_trait]
impl Store for SqliteStore {
    async fn begin(&self) -> Result<Box<dyn StoreTransaction>, StoreError> {
        let Backend::Pool(pool) = &self.backend else {
            return Err(StoreError::Invalid(
                "nested transactions unsupported".into(),
            ));
        };
        let transaction = pool.begin().await.map_err(database_error)?;
        Ok(Box::new(Self {
            backend: Backend::Transaction(Mutex::new(Some(TxState {
                transaction,
                mutations: Vec::new(),
            }))),
            hooks: self.hooks.clone(),
        }))
    }
}

#[async_trait]
impl StoreTransaction for SqliteStore {
    async fn commit(self: Box<Self>) -> Result<(), StoreError> {
        let Backend::Transaction(state) = &self.backend else {
            return Err(StoreError::Invalid("not a transaction".into()));
        };
        let state = state
            .lock()
            .await
            .take()
            .ok_or_else(|| StoreError::Invalid("transaction finished".into()))?;
        state.transaction.commit().await.map_err(database_error)?;
        self.notify(&state.mutations);
        Ok(())
    }
    async fn rollback(self: Box<Self>) -> Result<(), StoreError> {
        let Backend::Transaction(state) = &self.backend else {
            return Err(StoreError::Invalid("not a transaction".into()));
        };
        let state = state
            .lock()
            .await
            .take()
            .ok_or_else(|| StoreError::Invalid("transaction finished".into()))?;
        state.transaction.rollback().await.map_err(database_error)
    }
}
