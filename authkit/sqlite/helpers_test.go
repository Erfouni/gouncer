// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/sqlite"
)

var (
	_ gouncer.Store         = (*sqlite.UserStore)(nil)
	_ authkit.AdminStore    = (*sqlite.UserStore)(nil)
	_ authkit.SessionReaper = (*sqlite.UserStore)(nil)
	_ authkit.InviteStore   = (*sqlite.UserStore)(nil)
	_ authkit.TokenReaper   = (*sqlite.UserStore)(nil)
)

// testMigrate is how every test run of Migrate waits for the migration lock.
var testMigrate = sqlite.MigrateOptions{LockWait: 5 * time.Second, LockPoll: 10 * time.Millisecond}

// template is the migrated database each test opens a copy of.
var template *sqlitetest.Template

// testOptions returns the handle options every test opens with.
func testOptions() dbsqlite.Options {
	return dbsqlite.Options{
		BusyTimeout: 5 * time.Second, CacheSize: 2048, MaxConns: 4, Synchronous: dbsqlite.SynchronousNormal,
	}
}

// migrated returns a handle on a fresh copy of the migrated template, closed when t ends.
func migrated(t *testing.T) *sql.DB {
	t.Helper()
	return template.Open(t)
}

// mustExec runs statement with args on db and fails t when it errs.
func mustExec(t *testing.T, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

// count returns the single integer lookup with args answers on db.
func count(t *testing.T, db *sql.DB, lookup string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), lookup, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	return n
}

// TestMain builds the migrated template, runs the tests, then removes the template.
func TestMain(m *testing.M) {
	built, err := sqlitetest.NewTemplate(context.Background(), testOptions(), func(ctx context.Context, db *sql.DB) error {
		return sqlite.Migrate(ctx, db, testMigrate)
	})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "building the migrated template:", err)
		os.Exit(1)
	}
	template = built
	code := m.Run()
	_ = template.Close()
	os.Exit(code)
}
