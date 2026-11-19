use construct_core::{store::*, testing::MemoryStore};
use construct_store::SqliteStore;
use construct_types::{entity::*, memory::*, Timestamp};
use std::{ops::Deref, sync::Arc};
use uuid::Uuid;

struct Fixture {
    store: Arc<dyn Store>,
    _directory: Option<tempfile::TempDir>,
}
impl Deref for Fixture {
    type Target = Arc<dyn Store>;
    fn deref(&self) -> &Self::Target {
        &self.store
    }
}

mod memory {
    use super::*;
    async fn make_store() -> Fixture {
        Fixture {
            store: Arc::new(MemoryStore::default()),
            _directory: None,
        }
    }
    include!("common/suite.rs");
}

mod sqlite {
    use super::*;
    async fn make_store() -> Fixture {
        let directory = tempfile::tempdir().unwrap();
        let store = SqliteStore::open(directory.path().join("store.db"))
            .await
            .unwrap();
        Fixture {
            store: Arc::new(store),
            _directory: Some(directory),
        }
    }
    include!("common/suite.rs");
}
