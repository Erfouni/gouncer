// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"testing"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/sqlite"
)

func TestASecondFirstAccountAnswersSetupDoneWithoutTheStoresWrapping(t *testing.T) {
	t.Parallel()

	store := sqlite.NewUserStore(migrated(t))
	created(t, store, newAccount(t, "admin@example.com", "admin account"))

	err := store.CreateFirstUser(t.Context(), newAccount(t, "maria@example.com", "Maria Perez"))

	if err != authkit.ErrSetupDone {
		t.Errorf("CreateFirstUser() on a store with an account = %v, want authkit.ErrSetupDone as it is", err)
	}
}

func TestTheFirstAccountStartsUnderTheRoleItIsGiven(t *testing.T) {
	t.Parallel()

	store := sqlite.NewUserStore(migrated(t))
	first := newAccount(t, "owner@example.com", "owner account")
	first.Role = "owner"
	if err := store.CreateFirstUser(t.Context(), first); err != nil {
		t.Fatalf("CreateFirstUser() error = %v, want nil", err)
	}

	held, err := store.UserByID(t.Context(), first.ID)

	if err != nil || held.Role != "owner" {
		t.Errorf("the first account read back the role %q with %v, want owner", held.Role, err)
	}
}
