// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"

	"github.com/gopherium/gouncer/authkit/sqlite"
)

// coreMigrations are the migrations of another owner of the same database file.
var coreMigrations = dbsqlite.Migrations{
	Table: "core_goose_db_version",
	FS: fstest.MapFS{"00001_create_core_items.sql": {
		Data: []byte("-- +goose Up\nCREATE TABLE core_items (id TEXT NOT NULL PRIMARY KEY);\n"),
	}},
	LockWait: 5 * time.Second,
	LockPoll: 10 * time.Millisecond,
}

func TestMigrateRecordsTheBaselineInItsOwnVersionTable(t *testing.T) {
	t.Parallel()

	db := migrated(t)

	if got := count(t, db, "SELECT max(version_id) FROM auth_goose_db_version"); got != 4 {
		t.Errorf("the highest version in auth_goose_db_version = %d, want the 00004 baseline", got)
	}
}

func TestMigrateRunsTwiceBesideAnotherOwnersVersionTable(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())
	if err := dbsqlite.Migrate(t.Context(), db, coreMigrations); err != nil {
		t.Fatalf("migrating the other owner: %v", err)
	}

	first, second := sqlite.Migrate(t.Context(), db, testMigrate), sqlite.Migrate(t.Context(), db, testMigrate)

	auth := count(t, db, "SELECT max(version_id) FROM auth_goose_db_version")
	core := count(t, db, "SELECT max(version_id) FROM core_goose_db_version")
	if first != nil || second != nil || auth != 4 || core != 1 {
		t.Errorf("Migrate() twice = %v and %v with versions auth %d and core %d, want nil, nil, 4 and 1",
			first, second, auth, core)
	}
}

func TestMigrateRefusesLockSettingsAtZero(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())

	err := sqlite.Migrate(t.Context(), db, sqlite.MigrateOptions{})

	tables := count(t, db, "SELECT count(*) FROM sqlite_master WHERE name = 'auth_users'")
	if err == nil || !strings.HasPrefix(err.Error(), "sqlite: apply migrations: ") || tables != 0 {
		t.Errorf("Migrate() with no lock settings = %v with %d users tables, want a refusal and none", err, tables)
	}
}
