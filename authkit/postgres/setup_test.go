// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"testing"

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
