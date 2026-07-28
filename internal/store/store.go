package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS photos (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id     TEXT    NOT NULL UNIQUE,
  path        TEXT    NOT NULL,
  thumb_path  TEXT    NOT NULL,
  byte_size   INTEGER NOT NULL,
  width       INTEGER NOT NULL,
  height      INTEGER NOT NULL,
  source      TEXT    NOT NULL,
  caption     TEXT,
  created_at  TEXT    NOT NULL,
  hidden      INTEGER NOT NULL DEFAULT 0,
  client_ip   TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_photos_created ON photos(created_at DESC);

CREATE TABLE IF NOT EXISTS guestbook_entries (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT    NOT NULL,
  message    TEXT    NOT NULL,
  photo_id   INTEGER REFERENCES photos(id),
  created_at TEXT    NOT NULL,
  hidden     INTEGER NOT NULL DEFAULT 0,
  client_ip  TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_guestbook_created ON guestbook_entries(created_at DESC);
`

// Open opens (creating if needed) a file-backed SQLite database in WAL
// mode — lower write amplification than the default rollback journal,
// which matters when the database shares a microSD card with the OS —
// and applies the schema migration idempotently.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: opening %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("store: connecting to %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("store: migrating %s: %w", path, err)
	}
	return db, nil
}

// OpenMemory opens an in-memory database for tests. Connections are
// capped at one: SQLite's :memory: databases are private per connection,
// so a pool of more than one would see an empty schema on the second
// connection.
func OpenMemory() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("store: opening in-memory db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("store: migrating in-memory db: %w", err)
	}
	return db, nil
}
