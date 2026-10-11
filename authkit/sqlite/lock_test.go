// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin

package sqlite_test

import (
	"database/sql"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"

	"github.com/gopherium/gouncer/authkit/sqlite"
)

// migrationLock opens the migration lock file beside the main file of db, closed when t ends.
func migrationLock(t *testing.T, db *sql.DB) *os.File {
	t.Helper()
	var path string
	lookup := "SELECT file FROM pragma_database_list WHERE name = 'main'"
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&path); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	lock, err := os.OpenFile(path+".migrate.lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("opening the migration lock: %v", err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	return lock
}

func TestMigrateGivesUpOnAMigrationLockHeldPastItsWait(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())
	lock := migrationLock(t, db)
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("holding the migration lock: %v", err)
	}

	err := sqlite.Migrate(t.Context(), db, sqlite.MigrateOptions{LockWait: 100 * time.Millisecond,
		LockPoll: 10 * time.Millisecond})

	tables := count(t, db, "SELECT count(*) FROM sqlite_master WHERE name = 'auth_users'")
	if err == nil || !strings.Contains(err.Error(), "migration lock") || tables != 0 {
		t.Errorf("Migrate() behind a held lock = %v with %d users tables, want the lock named and none", err, tables)
	}
}

func TestMigrateLeavesTheMigrationLockFreeOnceItHasRun(t *testing.T) {
	t.Parallel()

	db := sqlitetest.Open(t, testOptions())

	err := sqlite.Migrate(t.Context(), db, testMigrate)

	tables := count(t, db, "SELECT count(*) FROM sqlite_master WHERE name = 'auth_users'")
	free := syscall.Flock(int(migrationLock(t, db).Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil || tables != 1 || free != nil {
		t.Errorf("Migrate() = %v with %d users tables and the lock taken with %v, want nil, 1 and the lock free",
			err, tables, free)
	}
}
