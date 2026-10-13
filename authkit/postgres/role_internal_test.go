// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
)

// errBackend is the failure the mocked database answers.
var errBackend = errors.New("backend unavailable")

// mockPool returns a mocked pool closed when t ends.
func mockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v, want nil", err)
	}
	t.Cleanup(mock.Close)
	return mock
}

// expectRole expects the account id to be read inside the transaction, holding role.
func expectRole(mock pgxmock.PgxPoolIface, id uuid.UUID, role string) {
	columns := []string{"id", "email", "name", "password_hash", "disabled", "created_at", "role", "confirmed"}
	mock.ExpectQuery("FROM auth.users").WithArgs(id).WillReturnRows(pgxmock.NewRows(columns).
		AddRow(id, "maria@example.com", "Maria Perez", "", false, time.Now().UTC(), role, true))
}

// expectMember expects the account id to be read inside the transaction, holding the member role.
func expectMember(mock pgxmock.PgxPoolIface, id uuid.UUID) {
	expectRole(mock, id, "member")
}

// expectLastAdmin expects the guard to lock the admins and find the account id the last enabled one.
func expectLastAdmin(mock pgxmock.PgxPoolIface, id uuid.UUID) {
	admins := []string{"admin"}
	mock.ExpectExec("FOR UPDATE").WithArgs(admins).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery(`count\(\*\)`).WithArgs(id, admins).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`count\(\*\)`).WithArgs(admins, id).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
}

func TestUserStoreCheckedWritesAnswerARolePromotedBeforeTheGuard(t *testing.T) {
	t.Parallel()

	for _, write := range []struct {
		name string
		run  func(t *testing.T, store *UserStore, id uuid.UUID) error
	}{
		{"SetUserRoleFrom", func(t *testing.T, store *UserStore, id uuid.UUID) error {
			return store.SetUserRoleFrom(t.Context(), id, "member", "editor", []string{"admin"})
		}},
		{"SetUserDisabledFrom", func(t *testing.T, store *UserStore, id uuid.UUID) error {
			return store.SetUserDisabledFrom(t.Context(), id, "member", true, []string{"admin"})
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			t.Parallel()

			mock := mockPool(t)
			id := uuid.Must(uuid.NewV7())
			mock.ExpectBegin()
			expectMember(mock, id)
			expectLastAdmin(mock, id)
			expectRole(mock, id, "admin")
			mock.ExpectRollback()

			err := write.run(t, newUserStore(mock), id)

			wantError(t, mock, write.name, err, authkit.ErrRoleChanged)
		})
	}
}

func TestUserStoreCheckedWriteReportsARecheckFailureAtTheGuard(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	expectLastAdmin(mock, id)
	mock.ExpectQuery("FROM auth.users").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", []string{"admin"})

	wantError(t, mock, "SetUserRoleFrom", err, errBackend)
}

func TestUserStoreCheckedWriteReportsAGuardFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("FOR UPDATE").WithArgs([]string{"admin"}).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, []string{"admin"})

	wantError(t, mock, "SetUserDisabledFrom", err, errBackend)
}

func TestUserStoreCheckedWriteKeepsTheGuardWhenTheRoleStands(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectRole(mock, id, "admin")
	expectLastAdmin(mock, id)
	expectRole(mock, id, "admin")
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "admin", "editor", []string{"admin"})

	wantError(t, mock, "SetUserRoleFrom", err, gouncer.ErrLastPrivileged)
}

func TestUserStoreSetUserRoleReportsWriteFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(id, "editor").WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRole(t.Context(), id, "editor", nil)

	wantError(t, mock, "SetUserRole", err, errBackend)
}

func TestUserStoreGuardedWriteReportsBeginFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin().WillReturnError(errBackend)

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantError(t, mock, "SetUserRoleFrom", err, errBackend)
}

func TestUserStoreGuardedWriteReportsCommitFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs("editor", id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit().WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantError(t, mock, "SetUserRoleFrom", err, errBackend)
}

func TestUserStoreUsersByIDsReportsQueryFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	ids := []uuid.UUID{uuid.Must(uuid.NewV7())}
	mock.ExpectQuery(`id = ANY\(`).WithArgs(ids).WillReturnError(errBackend)

	_, err := newUserStore(mock).UsersByIDs(t.Context(), ids)

	wantError(t, mock, "UsersByIDs", err, errBackend)
}

// wantError fails t unless err wraps want and every expectation ran.
func wantError(t *testing.T, mock pgxmock.PgxPoolIface, method string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s() error = %v, want it to wrap %v", method, err, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUserStoreSetUserRoleFromReportsReadFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectQuery("FROM auth.users").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantError(t, mock, "SetUserRoleFrom", err, errBackend)
}

func TestUserStoreSetUserRoleFromReportsWriteFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs("editor", id, "member").WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantError(t, mock, "SetUserRoleFrom", err, errBackend)
}

func TestUserStoreSetUserRoleFromAnswersARoleMovedAfterTheCheck(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs("editor", id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	expectMember(mock, id)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantError(t, mock, "SetUserRoleFrom", err, authkit.ErrRoleChanged)
}

func TestUserStoreSetUserRoleFromAnswersAnAccountDeletedAfterTheCheck(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs("editor", id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectQuery("FROM auth.users").WithArgs(id).WillReturnError(pgx.ErrNoRows)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantError(t, mock, "SetUserRoleFrom", err, gouncer.ErrUserNotFound)
}

func TestUserStoreSetUserRoleFromReportsARecheckFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs("editor", id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectQuery("FROM auth.users").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantError(t, mock, "SetUserRoleFrom", err, errBackend)
}

func TestUserStoreSetUserDisabledFromReportsWriteFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantError(t, mock, "SetUserDisabledFrom", err, errBackend)
}

func TestUserStoreSetUserDisabledFromAnswersARoleMovedAfterTheCheck(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	expectMember(mock, id)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantError(t, mock, "SetUserDisabledFrom", err, authkit.ErrRoleChanged)
}

func TestUserStoreSetUserDisabledFromAnswersAnAccountDeletedAfterTheCheck(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectQuery("FROM auth.users").WithArgs(id).WillReturnError(pgx.ErrNoRows)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantError(t, mock, "SetUserDisabledFrom", err, gouncer.ErrUserNotFound)
}

func TestUserStoreSetUserDisabledFromReportsTokenRevokeFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM auth.sessions").WithArgs(id).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("DELETE FROM auth.tokens").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantError(t, mock, "SetUserDisabledFrom", err, errBackend)
}

func TestUserStoreGuardedDisableReportsWriteFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(id, true).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledUnderCover(t.Context(), id, true, nil)

	wantError(t, mock, "SetUserDisabledUnderCover", err, errBackend)
}

func TestUserStoreSetUserDisabledFromReportsSessionRevokeFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	expectMember(mock, id)
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM auth.sessions").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantError(t, mock, "SetUserDisabledFrom", err, errBackend)
}
