// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gopherium/framework/dbkit"
	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"
)

const (
	// accountID is the id of the account the schema tests insert.
	accountID = "019a0000-0000-7000-8000-000000000001"
	// insertAccount stores an enabled account row with every column that has no default.
	insertAccount = "INSERT INTO auth_users (id, email, name, password_hash, disabled, created_at) " +
		"VALUES (?, 'maria@example.com', 'Maria Perez', 'placeholder-hash', 0, 1767225600000000)"
	// insertFlagged stores an account row with the given disabled and confirmed values.
	insertFlagged = "INSERT INTO auth_users (id, email, name, password_hash, disabled, confirmed, created_at) " +
		"VALUES (?, 'maria@example.com', 'Maria Perez', 'placeholder-hash', ?, ?, 1767225600000000)"
	// insertSession stores a session row for an account.
	insertSession = "INSERT INTO auth_sessions (token_hash, user_id, created_at, expires_at) " +
		"VALUES (?, ?, 1767225600000000, 1767229200000000)"
	// insertToken stores an invite token row for an account.
	insertToken = "INSERT INTO auth_tokens (token_hash, user_id, purpose, created_at, expires_at) " +
		"VALUES (?, ?, 'invite', 1767225600000000, 1767229200000000)"
)

// refusal returns the error statement with args answers on db, classed by dbkit.
func refusal(t *testing.T, db *sql.DB, statement string, args ...any) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(), statement, args...)
	return dbsqlite.Classify(err)
}

func TestSchemaRefusesANullKeyOnEveryTable(t *testing.T) {
	t.Parallel()

	db := migrated(t)
	mustExec(t, db, insertAccount, accountID)

	for statement, args := range map[string][]any{
		insertAccount: {nil},
		insertSession: {nil, accountID},
		insertToken:   {nil, accountID},
	} {
		if err := refusal(t, db, statement, args...); !errors.Is(err, dbkit.ErrNotNull) {
			t.Errorf("%s with a NULL key = %v, want dbkit.ErrNotNull", statement, err)
		}
	}
}

func TestSchemaRefusesABooleanOtherThanZeroOrOne(t *testing.T) {
	t.Parallel()

	db := migrated(t)

	for column, flags := range map[string][2]int{"disabled": {2, 1}, "confirmed": {0, 2}} {
		if err := refusal(t, db, insertFlagged, accountID, flags[0], flags[1]); !errors.Is(err, dbkit.ErrCheck) {
			t.Errorf("an account with %s at 2 = %v, want dbkit.ErrCheck", column, err)
		}
	}
}

func TestSchemaFillsTheConfirmationAndTheRole(t *testing.T) {
	t.Parallel()

	db := migrated(t)
	mustExec(t, db, insertAccount, accountID)

	var confirmed bool
	var role string
	lookup := "SELECT confirmed, role FROM auth_users WHERE id = ?"
	if err := db.QueryRowContext(t.Context(), lookup, accountID).Scan(&confirmed, &role); err != nil {
		t.Fatalf("%s: %v", lookup, err)
	}
	if !confirmed || role != "" {
		t.Errorf("an account inserted without them reads confirmed %t and role %q, want true and none", confirmed, role)
	}
}

func TestSchemaCascadesAnAccountsSessionsAndTokens(t *testing.T) {
	t.Parallel()

	db := migrated(t)
	mustExec(t, db, insertAccount, accountID)
	mustExec(t, db, insertSession, []byte("session-hash"), accountID)
	mustExec(t, db, insertToken, []byte("token-hash"), accountID)

	mustExec(t, db, "DELETE FROM auth_users WHERE id = ?", accountID)

	sessions := count(t, db, "SELECT count(*) FROM auth_sessions")
	tokens := count(t, db, "SELECT count(*) FROM auth_tokens")
	if sessions != 0 || tokens != 0 {
		t.Errorf("after the account went %d sessions and %d tokens are left, want none", sessions, tokens)
	}
}

func TestSchemaNamesEveryIndexWithTheAuthPrefix(t *testing.T) {
	t.Parallel()

	db := migrated(t)
	rows, err := db.QueryContext(t.Context(),
		"SELECT name, sql FROM sqlite_master WHERE type = 'index' AND sql IS NOT NULL ORDER BY name")
	if err != nil {
		t.Fatalf("listing the indexes: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	var roleIndex string
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			t.Fatalf("reading an index: %v", err)
		}
		names = append(names, name)
		if name == "auth_users_role_idx" {
			roleIndex = definition
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("listing the indexes: %v", err)
	}

	want := []string{"auth_sessions_expires_at_idx", "auth_sessions_user_id_idx", "auth_tokens_expires_at_idx",
		"auth_tokens_user_id_purpose_idx", "auth_users_role_idx"}
	if !slices.Equal(names, want) || !strings.HasSuffix(roleIndex, "WHERE role <> ''") {
		t.Errorf("indexes = %v with the role index %q, want %v and a partial role index", names, roleIndex, want)
	}
}
