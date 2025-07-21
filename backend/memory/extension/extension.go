package extension

import (
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/furisto/construct/backend/memory/predicate"
)

func UUIDHasPrefix(field string, prefix string) predicate.Task {
	return func(s *sql.Selector) {
		d := s.Dialect()

		switch d {
		case dialect.SQLite:
			// UUIDs are stored as formatted strings in SQLite
			s.Where(sql.P(func(b *sql.Builder) {
				b.Ident(field).WriteString(" LIKE ").Arg(prefix + "%")
			}))

		case dialect.Postgres:
			// PostgreSQL can cast UUID to text
			s.Where(sql.P(func(b *sql.Builder) {
				b.Ident(field).WriteString("::text LIKE ").Arg(prefix + "%")
			}))

		default:
			s.Where(sql.P(func(b *sql.Builder) {
				b.WriteString("CAST(").Ident(field).WriteString(" AS CHAR) LIKE ").
					Arg(prefix + "%")
			}))
		}
	}
}
