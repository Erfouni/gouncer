// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/postgres"
)

func TestAnInvitedAccountClosesTheFirstAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	invitedAccount(t, store)

	err := store.CreateFirstUser(t.Context(), namedUser(t, "maria+2@example.com", "Maria Perez"))

	if !errors.Is(err, authkit.ErrSetupDone) {
		t.Errorf("CreateFirstUser() beside an invited account error = %v, want authkit.ErrSetupDone", err)
	}
}

func TestTheFirstAccountStartsUnderTheRoleItIsGiven(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	first := namedUser(t, "owner@example.com", "Maria Perez")
	first.Role = "owner"
	ctx, cancel := context.WithTimeout(t.Context(), lockRelease)
	defer cancel()
	if err := store.CreateFirstUser(ctx, first); err != nil {
		t.Fatalf("CreateFirstUser() on an empty store error = %v, want nil within %v", err, lockRelease)
	}

	held, err := store.UserByID(t.Context(), first.ID)

	if err != nil || held.Role != "owner" {
		t.Errorf("the first account read back the role %q with %v, want owner", held.Role, err)
	}
}

func TestAFirstAccountInARepeatableReadTransactionIsRefused(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatalf("BeginTx() error = %v, want nil", err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	inside := store.Within(tx)
	if held, err := inside.HasUsers(t.Context()); err != nil || held {
		t.Fatalf("HasUsers() in the transaction = %t, %v, want false and nil", held, err)
	}
	storedUser(t, store)

	err = inside.CreateFirstUser(t.Context(), namedUser(t, "maria+2@example.com", "Maria Perez"))

	if !strings.Contains(fmt.Sprint(err), "needs READ COMMITTED") {
		t.Errorf("CreateFirstUser() in a repeatable read transaction error = %v, want a refusal naming READ COMMITTED",
			err)
	}
	wantAccountsAfterCommit(t, tx, store, 1)
}

// wantAccountsAfterCommit commits tx and fails t unless store then holds want accounts.
func wantAccountsAfterCommit(t *testing.T, tx pgx.Tx, store *postgres.UserStore, want int) {
	t.Helper()
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("Commit() error = %v, want nil", err)
	}
	listed, err := store.ListUsers(t.Context())
	if err != nil || len(listed) != want {
		t.Errorf("the store holds %d accounts with %v after the commit, want %d", len(listed), err, want)
	}
}
