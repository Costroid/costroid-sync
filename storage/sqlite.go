package storage

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // registers the "sqlite3" driver
)

// Open returns a handle to the SQLite database at path.
// Schema creation and migrations land in C2.
func Open(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}
