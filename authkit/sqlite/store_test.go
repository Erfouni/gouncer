// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/sqlite"
)

// newAccount returns an enabled account at email named name.
func newAccount(t *testing.T, email, name string) gouncer.User {
	t.Helper()
	user, err := gouncer.NewUser(email, name, "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	return user
}

// created stores user in store and fails t when it cannot.
func created(t *testing.T, store *sqlite.UserStore, user gouncer.User) gouncer.User {
	t.Helper()
	if err := store.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return user
}

// storedText returns the SQLite type and the text of column in the one row of table on db.
func storedText(t *testing.T, db *sql.DB, table, column string) (string, string) {
	t.Helper()
	var kind, value string
	lookup := "SELECT typeof(" + column + "), " + column + " FROM " + table
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&kind, &value); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	return kind, value
}

// storedInteger returns the SQLite type and the integer of column in the one row of table on db.
func storedInteger(t *testing.T, db *sql.DB, table, column string) (string, int64) {
	t.Helper()
	var kind string
	var value int64
	lookup := "SELECT typeof(" + column + "), " + column + " FROM " + table
	if err := db.QueryRowContext(t.Context(), lookup).Scan(&kind, &value); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	return kind, value
}

func TestCreateUserStoresTheIDInItsTextForm(t *testing.T) {
	t.Parallel()

	db := migrated(t)
	user := created(t, sqlite.NewUserStore(db), newAccount(t, "maria@example.com", "Maria Perez"))

	kind, value := storedText(t, db, "auth_users", "id")

	if kind != "text" || value != user.ID.String() || len(value) != 36 {
		t.Errorf("the stored id is %s %q, want the text %q of 36 characters", kind, value, user.ID.String())
	}
}

func TestTheStoreKeepsEveryTimeAsMicrosecondsSinceTheEpoch(t *testing.T) {
	t.Parallel()

	db := migrated(t)
	store := sqlite.NewUserStore(db)
	user := created(t, store, newAccount(t, "maria@example.com", "Maria Perez"))
	session, err := gouncer.NewSession(user.ID)
	if err != nil {
		t.Fatalf("gouncer.NewSession() error = %v, want nil", err)
	}
	if err := store.CreateSession(t.Context(), session); err != nil {
		t.Fatalf("CreateSession() error = %v, want nil", err)
	}
	token, err := gouncer.NewToken(user.ID, gouncer.PurposeReset, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	if err := store.CreateToken(t.Context(), token, 1); err != nil {
		t.Fatalf("CreateToken() error = %v, want nil", err)
	}

	for _, stored := range []struct {
		table, column string
		want          time.Time
	}{
		{"auth_users", "created_at", user.CreatedAt},
		{"auth_sessions", "expires_at", session.ExpiresAt},
		{"auth_tokens", "expires_at", token.ExpiresAt},
	} {
		kind, value := storedInteger(t, db, stored.table, stored.column)
		if kind != "integer" || value != stored.want.UnixMicro() {
			t.Errorf("%s.%s is %s %d, want the integer %d", stored.table, stored.column, kind, value,
				stored.want.UnixMicro())
		}
	}
}

func TestUserStoreListsNamesThatDifferOnlyInCaseByID(t *testing.T) {
	t.Parallel()

	store := sqlite.NewUserStore(migrated(t))
	alpha, bravo := newAccount(t, "alpha@example.com", "alpha account"), newAccount(t, "bravo@example.com",
		"Bravo account")
	lower, upper := newAccount(t, "lower@example.com", "ivory account"), newAccount(t, "upper@example.com",
		"Ivory account")
	lower.ID = uuid.MustParse("019a0000-0000-7000-8000-000000000001")
	upper.ID = uuid.MustParse("019a0000-0000-7000-8000-000000000002")
	for _, user := range []gouncer.User{upper, bravo, lower, alpha} {
		created(t, store, user)
	}

	listed, err := store.ListUsers(t.Context())

	if err != nil {
		t.Fatalf("ListUsers() error = %v, want nil", err)
	}
	got := make([]uuid.UUID, len(listed))
	for i, user := range listed {
		got[i] = user.ID
	}
	want := []uuid.UUID{alpha.ID, bravo.ID, lower.ID, upper.ID}
	if !slices.Equal(got, want) {
		t.Errorf("ListUsers() ids = %v, want %v (alpha, Bravo, then the ivory twins by id)", got, want)
	}
}
