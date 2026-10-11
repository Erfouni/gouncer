// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/framework/dbkit"
	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"

	"github.com/gopherium/gouncer/authkit/sqlite"
)

func TestAWriteHeldPastTheBusyTimeoutAnswersDbkitsBusyClass(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.BusyTimeout = 50 * time.Millisecond
	db := sqlitetest.Open(t, opts)
	if err := sqlite.Migrate(t.Context(), db, testMigrate); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	store := sqlite.NewUserStore(db)
	user := created(t, store, newAccount(t, "maria@example.com", "Maria Perez"))
	hold, err := holdOn(db)(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("holding the account: %v", err)
	}
	t.Cleanup(func() { _ = hold.Rollback(t.Context()) })

	err = store.SetUserDisabled(t.Context(), user.ID, true)

	if !errors.Is(err, dbkit.ErrBusy) || !strings.HasPrefix(err.Error(), "sqlite: set user disabled: ") {
		t.Errorf("SetUserDisabled() behind a hold past the busy timeout = %v, want dbkit.ErrBusy under the action", err)
	}
}
