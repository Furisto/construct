package memory

import (
	"database/sql"
	
	"modernc.org/sqlite"
)

func init() {
	// Register modernc.org/sqlite driver as "sqlite3" for compatibility with Ent
	// The driver registers itself as "sqlite" by default, but Ent expects "sqlite3"
	sql.Register("sqlite3", &sqlite.Driver{})
}
