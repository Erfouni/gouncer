// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/gopherium/gouncer"
)

// firstUserStep expects one statement of CreateFirstUser on an empty store, answering errBackend when failing.
type firstUserStep func(mock pgxmock.PgxPoolIface, u gouncer.User, failing bool)

// firstUserSteps are the statements CreateFirstUser runs on an empty store, in order.
var firstUserSteps = []struct {
	// name names the statement.
	name string
	// expect expects the statement.
	expect firstUserStep
}{
	{"the begin", expectFirstBegin},
	{"the lock", expectUsersLock},
	{"the isolation read", expectReadCommitted},
	{"the check", expectNoUsers},
	{"the insert", expectFirstInsert},
	{"the commit", expectFirstCommit},
}

// expectFirstBegin expects the transaction to begin, failing when failing.
func expectFirstBegin(mock pgxmock.PgxPoolIface, _ gouncer.User, failing bool) {
	begin := mock.ExpectBegin()
	if failing {
		begin.WillReturnError(errBackend)
	}
}

// expectUsersLock expects the accounts to be locked against every other write, failing when failing.
func expectUsersLock(mock pgxmock.PgxPoolIface, _ gouncer.User, failing bool) {
	lock := mock.ExpectExec(`^LOCK TABLE auth\.users IN SHARE ROW EXCLUSIVE MODE$`)
	if failing {
		lock.WillReturnError(errBackend)
		return
	}
	lock.WillReturnResult(pgxmock.NewResult("LOCK TABLE", 0))
}

// expectIsolation expects the transaction's isolation level to be read, answering level.
func expectIsolation(mock pgxmock.PgxPoolIface, level string) *pgxmock.ExpectedQuery {
	read := mock.ExpectQuery(`^SELECT current_setting\('transaction_isolation'\)$`)
	return read.WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow(level))
}

// expectReadCommitted expects the isolation level to be read as read committed, failing when failing.
func expectReadCommitted(mock pgxmock.PgxPoolIface, _ gouncer.User, failing bool) {
	read := expectIsolation(mock, "read committed")
	if failing {
		read.WillReturnError(errBackend)
	}
}

// expectNoUsers expects the check for any account to find none, failing when failing.
func expectNoUsers(mock pgxmock.PgxPoolIface, _ gouncer.User, failing bool) {
	check := mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM auth\.users\)\s*$`)
	if failing {
		check.WillReturnError(errBackend)
		return
	}
	check.WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
}

// expectFirstInsert expects u to be inserted, failing when failing.
func expectFirstInsert(mock pgxmock.PgxPoolIface, u gouncer.User, failing bool) {
	insert := mock.ExpectExec(`INSERT INTO auth\.users`).
		WithArgs(u.ID, u.Email, u.Name, u.PasswordHash, u.Disabled, u.CreatedAt, u.Role, u.Confirmed)
	if failing {
		insert.WillReturnError(errBackend)
		return
	}
	insert.WillReturnResult(pgxmock.NewResult("INSERT", 1))
}

// expectFirstCommit expects the transaction to commit, failing when failing.
func expectFirstCommit(mock pgxmock.PgxPoolIface, _ gouncer.User, failing bool) {
	commit := mock.ExpectCommit()
	if failing {
		commit.WillReturnError(errBackend)
	}
}

// firstAccount returns the synthetic account the failure rows try to store.
func firstAccount() gouncer.User {
	return gouncer.User{
		ID:           uuid.Must(uuid.NewV7()),
		Email:        "maria@example.com",
		Name:         "Maria Perez",
		PasswordHash: "argon2id-hash",
		Confirmed:    true,
		Role:         "owner",
		CreatedAt:    time.Now().UTC(),
	}
}

// wantNamed fails t unless err reads action then the planted failure, wraps it, and every expectation ran.
func wantNamed(t *testing.T, mock pgxmock.PgxPoolIface, err error, action string) {
	t.Helper()
	want := action + ": " + errBackend.Error()
	if fmt.Sprint(err) != want || !errors.Is(err, errBackend) {
		t.Errorf("error = %v, want %q wrapping %v", err, want, errBackend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCreateFirstUserNamesEachFailure(t *testing.T) {
	t.Parallel()

	for failing, step := range firstUserSteps {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()

			mock := mockPool(t)
			u := firstAccount()
			for i, expected := range firstUserSteps[:failing+1] {
				expected.expect(mock, u, i == failing)
			}
			if failing > 0 {
				mock.ExpectRollback()
			}

			err := newUserStore(mock).CreateFirstUser(t.Context(), u)

			wantNamed(t, mock, err, "postgres: create first user")
		})
	}
}

func TestCreateFirstUserRefusesALevelThatKeepsAnOlderSnapshot(t *testing.T) {
	t.Parallel()

	for _, level := range []string{"repeatable read", "serializable"} {
		t.Run(level, func(t *testing.T) {
			t.Parallel()

			mock := mockPool(t)
			u := firstAccount()
			expectFirstBegin(mock, u, false)
			expectUsersLock(mock, u, false)
			expectIsolation(mock, level)
			mock.ExpectRollback()

			err := newUserStore(mock).CreateFirstUser(t.Context(), u)

			want := "postgres: create first user: a first account needs READ COMMITTED, the transaction runs " + level
			if fmt.Sprint(err) != want {
				t.Errorf("error = %v, want %q", err, want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestHasUsersNamesItsFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	expectNoUsers(mock, gouncer.User{}, true)

	_, err := newUserStore(mock).HasUsers(t.Context())

	wantNamed(t, mock, err, "postgres: has users")
}
