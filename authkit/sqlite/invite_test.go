// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/sqlite"
)

// invitedAccount returns an enabled account at email that has not set a password yet.
func invitedAccount(t *testing.T, email string) gouncer.User {
	t.Helper()
	user, err := gouncer.NewInvitedUser(email, "invited account")
	if err != nil {
		t.Fatalf("gouncer.NewInvitedUser() error = %v, want nil", err)
	}
	return user
}

// tokenAt returns a token for user under purpose that expires at expires.
func tokenAt(t *testing.T, user gouncer.User, purpose gouncer.TokenPurpose, expires time.Time) gouncer.Token {
	t.Helper()
	token, err := gouncer.NewToken(user.ID, purpose, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	token.CreatedAt, token.ExpiresAt = expires.Add(-time.Hour), expires
	return token
}

func TestDeleteExpiredTokensSweepsEveryStrandedAccountButOneWithALiveInvite(t *testing.T) {
	t.Parallel()

	db := migrated(t)
	store := sqlite.NewUserStore(db)
	plant := plantOn(db)
	now := time.Now().UTC()
	strandedA, strandedB := invitedAccount(t, "stranded-a@example.com"), invitedAccount(t, "stranded-b@example.com")
	renewed, settled := invitedAccount(t, "renewed@example.com"), newAccount(t, "settled@example.com", "settled account")
	for _, user := range []gouncer.User{strandedA, strandedB, renewed, settled} {
		created(t, store, user)
	}
	for _, token := range []gouncer.Token{
		tokenAt(t, strandedA, gouncer.PurposeInvite, now.Add(-time.Minute)),
		tokenAt(t, strandedB, gouncer.PurposeInvite, now.Add(-time.Minute)),
		tokenAt(t, renewed, gouncer.PurposeInvite, now.Add(-time.Minute)),
		tokenAt(t, renewed, gouncer.PurposeInvite, now.Add(time.Hour)),
		tokenAt(t, settled, gouncer.PurposeReset, now.Add(-time.Minute)),
	} {
		if err := plant(t.Context(), token); err != nil {
			t.Fatalf("planting a token: %v", err)
		}
	}

	swept, err := store.DeleteExpiredTokens(t.Context(), now)

	if err != nil || swept != 4 {
		t.Fatalf("DeleteExpiredTokens() = %d, %v, want 4 expired tokens and nil", swept, err)
	}
	for _, user := range []gouncer.User{strandedA, strandedB} {
		if _, err := store.UserByID(t.Context(), user.ID); !errors.Is(err, gouncer.ErrUserNotFound) {
			t.Errorf("UserByID() of %s after the sweep = %v, want gouncer.ErrUserNotFound", user.Email, err)
		}
	}
	for _, user := range []gouncer.User{renewed, settled} {
		if _, err := store.UserByID(t.Context(), user.ID); err != nil {
			t.Errorf("UserByID() of %s after the sweep = %v, want the account kept", user.Email, err)
		}
	}
	if live := count(t, db, "SELECT count(*) FROM auth_tokens WHERE user_id = ?", renewed.ID); live != 1 {
		t.Errorf("tokens the renewed account holds after the sweep = %d, want its live invite", live)
	}
}
