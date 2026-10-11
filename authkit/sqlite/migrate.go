// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
)

// versionTable is the goose version table of the auth tables, beside any other owner's in the same file.
const versionTable = "auth_goose_db_version"

// migrationSource holds the migration files at its root.
var migrationSource = mustSub(migrations, "migrations")

// MigrateOptions sets how a run of Migrate waits for the migration lock of the database file.
type MigrateOptions struct {
	// LockWait is how long a run waits for the migration lock, above zero.
	LockWait time.Duration
	// LockPoll is how often a waiting run tries the migration lock, above zero.
	LockPoll time.Duration
}

// Migrate applies the auth table migrations to db under the migration lock of its file.
func Migrate(ctx context.Context, db *sql.DB, opts MigrateOptions) error {
	err := dbsqlite.Migrate(ctx, db, dbsqlite.Migrations{
		Table: versionTable, FS: migrationSource, LockWait: opts.LockWait, LockPoll: opts.LockPoll,
	})
	if err != nil {
		return fmt.Errorf("sqlite: apply migrations: %w", dbsqlite.Classify(err))
	}
	return nil
}

// mustSub returns the subtree of fsys at dir, and panics when dir is not a valid path.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
