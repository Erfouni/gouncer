// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
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

// wantBackendFailure fails t unless err wraps the mocked failure and every expectation ran.
func wantBackendFailure(t *testing.T, mock pgxmock.PgxPoolIface, method string, err error) {
	t.Helper()
	if !errors.Is(err, errBackend) {
		t.Errorf("%s() error = %v, want it to wrap %v", method, err, errBackend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUserStoreSetUserRoleFromReportsWriteFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs("editor", id, "member").WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantBackendFailure(t, mock, "SetUserRoleFrom", err)
}

func TestUserStoreSetUserRoleFromReportsLookupFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs("editor", id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectQuery("FROM auth.users").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserRoleFrom(t.Context(), id, "member", "editor", nil)

	wantBackendFailure(t, mock, "SetUserRoleFrom", err)
}

func TestUserStoreSetUserDisabledFromReportsWriteFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantBackendFailure(t, mock, "SetUserDisabledFrom", err)
}

func TestUserStoreSetUserDisabledFromReportsLookupFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectQuery("FROM auth.users").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantBackendFailure(t, mock, "SetUserDisabledFrom", err)
}

func TestUserStoreSetUserDisabledFromReportsTokenRevokeFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM auth.sessions").WithArgs(id).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("DELETE FROM auth.tokens").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantBackendFailure(t, mock, "SetUserDisabledFrom", err)
}

func TestUserStoreGuardedDisableReportsWriteFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(id, true).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledUnderCover(t.Context(), id, true, nil)

	wantBackendFailure(t, mock, "SetUserDisabledUnderCover", err)
}

func TestUserStoreSetUserDisabledFromReportsSessionRevokeFailure(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	id := uuid.Must(uuid.NewV7())
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(true, id, "member").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM auth.sessions").WithArgs(id).WillReturnError(errBackend)
	mock.ExpectRollback()

	err := newUserStore(mock).SetUserDisabledFrom(t.Context(), id, "member", true, nil)

	wantBackendFailure(t, mock, "SetUserDisabledFrom", err)
}
