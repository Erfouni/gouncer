// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/framework/dbkit/sqlite/sqlitetest"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/sqlite"
	"github.com/gopherium/gouncer/authkit/sqlite/internal/db"
)

// errPlanted is the failure a chosen statement answers with.
var errPlanted = errors.New("planted failure")

// faultWorld holds the accounts and tokens every failure case starts from.
type faultWorld struct {
	// admin is an enabled account holding the admin role.
	admin gouncer.User
	// invitee is an unconfirmed account holding a live invite.
	invitee gouncer.User
	// invite is the live invite token of invitee.
	invite gouncer.Token
	// reset is a live reset token of admin.
	reset gouncer.Token
}

// faultCase fails one statement and calls the store method that runs it.
type faultCase struct {
	// name says which statement fails in which call.
	name string
	// statement is the exact text of the statement that fails.
	statement string
	// action is the action the error names.
	action string
	// call runs the store method under test.
	call func(ctx context.Context, store *sqlite.UserStore, world faultWorld) error
}

// newFaultWorld stores the accounts and tokens of a failure case in store and db.
func newFaultWorld(t *testing.T, store *sqlite.UserStore, db *sql.DB) faultWorld {
	t.Helper()
	now := time.Now().UTC()
	admin := newAccount(t, "admin@example.com", "admin account")
	admin.Role = "admin"
	invitee, stranded := invitedAccount(t, "invitee@example.com"), invitedAccount(t, "stranded@example.com")
	for _, user := range []gouncer.User{admin, invitee, stranded} {
		created(t, store, user)
	}
	world := faultWorld{
		admin: admin, invitee: invitee,
		invite: tokenAt(t, invitee, gouncer.PurposeInvite, now.Add(time.Hour)),
		reset:  tokenAt(t, admin, gouncer.PurposeReset, now.Add(time.Hour)),
	}
	plant := plantOn(db)
	for _, token := range []gouncer.Token{world.invite, world.reset,
		tokenAt(t, stranded, gouncer.PurposeInvite, now.Add(-time.Minute))} {
		if err := plant(t.Context(), token); err != nil {
			t.Fatalf("planting a token: %v", err)
		}
	}
	return world
}

// ignoreValue drops the value of a call and keeps its error.
func ignoreValue[T any](_ T, err error) error {
	return err
}

// statementFaults are the failure cases, one for each statement a store method runs.
var statementFaults = []faultCase{
	{"get user by email", db.GetUserByEmail, "get user by email",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return ignoreValue(s.UserByEmail(ctx, w.admin.Email))
		}},
	{"get user by id", db.GetUserByID, "get user by id",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return ignoreValue(s.UserByID(ctx, w.admin.ID))
		}},
	{"get user by session", db.GetUserBySession, "get user by session",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return ignoreValue(s.UserBySession(ctx, []byte("session-hash"), time.Now()))
		}},
	{"create session", db.CreateSession, "create session",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.CreateSession(ctx, gouncer.Session{TokenHash: []byte("session-hash"), UserID: w.admin.ID,
				CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)})
		}},
	{"create user", db.CreateUser, "create user",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			user := w.admin
			user.Email = "another@example.com"
			return s.CreateUser(ctx, user)
		}},
	{"list users", db.ListUsers, "list users",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return ignoreValue(s.ListUsers(ctx))
		}},
	{"delete expired sessions", db.DeleteExpiredSessions, "delete expired sessions",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return ignoreValue(s.DeleteExpiredSessions(ctx, time.Now()))
		}},
	{"delete session", db.DeleteSession, "delete session",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return s.DeleteSession(ctx, []byte("session-hash"))
		}},
	{"grant role", db.GrantRoleToRoleless, "grant role",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return ignoreValue(s.GrantRoleToRoleless(ctx, "member"))
		}},
	{"disable the account", db.SetUserDisabled, "set user disabled",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.SetUserDisabled(ctx, w.admin.ID, true)
		}},
	{"end the sessions of a disabled account", db.DeleteUserSessions, "set user disabled",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.SetUserDisabled(ctx, w.admin.ID, true)
		}},
	{"end the tokens of a disabled account", db.DeleteUserTokens, "set user disabled",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.SetUserDisabled(ctx, w.admin.ID, true)
		}},
	{"write the role", db.SetUserRole, "set user role",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.SetUserRole(ctx, w.admin.ID, "member", nil)
		}},
	{"read whether the account is privileged", db.HoldsPrivilege, "set user role",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.SetUserRole(ctx, w.admin.ID, "member", gouncer.Roles{"admin"})
		}},
	{"count the cover", db.CountCover, "set user role",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.SetUserRole(ctx, w.admin.ID, "member", gouncer.Roles{"admin"})
		}},
	{"check the account a token is for", db.LockEnabledUser, "create token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.CreateToken(ctx, tokenFor(w.admin), 5)
		}},
	{"count the live tokens", db.CountLiveTokens, "create token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.CreateToken(ctx, tokenFor(w.admin), 5)
		}},
	{"store the token", db.CreateToken, "create token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.CreateToken(ctx, tokenFor(w.admin), 5)
		}},
	{"check the account an invite replaces for", db.LockUnactivatedUser, "replace token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.ReplaceToken(ctx, w.invite)
		}},
	{"delete the replaced tokens", db.DeleteUserTokensForPurpose, "replace token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return s.ReplaceToken(ctx, w.invite)
		}},
	{"find the invite", db.FindLiveToken, "activate by token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return ignoreValue(s.ActivateByToken(ctx, w.invite.TokenHash, time.Now(), "new-hash"))
		}},
	{"spend the invite", db.DeleteToken, "activate by token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return ignoreValue(s.ActivateByToken(ctx, w.invite.TokenHash, time.Now(), "new-hash"))
		}},
	{"confirm the account", db.ActivateUser, "activate by token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return ignoreValue(s.ActivateByToken(ctx, w.invite.TokenHash, time.Now(), "new-hash"))
		}},
	{"store the reset password", db.SetUserPassword, "reset by token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return ignoreValue(s.ResetByToken(ctx, w.reset.TokenHash, time.Now(), "new-hash"))
		}},
	{"end the sessions on a reset", db.DeleteUserSessions, "reset by token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return ignoreValue(s.ResetByToken(ctx, w.reset.TokenHash, time.Now(), "new-hash"))
		}},
	{"end the reset siblings", db.DeleteUserTokensForPurpose, "reset by token",
		func(ctx context.Context, s *sqlite.UserStore, w faultWorld) error {
			return ignoreValue(s.ResetByToken(ctx, w.reset.TokenHash, time.Now(), "new-hash"))
		}},
	{"read the expired tokens", db.ExpiredTokens, "delete expired tokens",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return ignoreValue(s.DeleteExpiredTokens(ctx, time.Now()))
		}},
	{"read the stranded accounts", db.LockUnconfirmedAccounts, "delete expired tokens",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return ignoreValue(s.DeleteExpiredTokens(ctx, time.Now()))
		}},
	{"delete the stranded accounts", db.DeleteUnconfirmedAccounts, "delete expired tokens",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return ignoreValue(s.DeleteExpiredTokens(ctx, time.Now()))
		}},
	{"delete the expired tokens", db.DeleteExpiredTokens, "delete expired tokens",
		func(ctx context.Context, s *sqlite.UserStore, _ faultWorld) error {
			return ignoreValue(s.DeleteExpiredTokens(ctx, time.Now()))
		}},
}

// tokenFor returns a reset token for user that lasts an hour.
func tokenFor(user gouncer.User) gouncer.Token {
	token, err := gouncer.NewToken(user.ID, gouncer.PurposeReset, time.Hour)
	if err != nil {
		panic(err)
	}
	return token
}

// faultyStore returns a store over a fresh copy of the template whose connections answer with faults.
func faultyStore(t *testing.T) (*sqlite.UserStore, *sqlitetest.Faults, faultWorld) {
	t.Helper()
	faults := &sqlitetest.Faults{}
	handle := template.OpenWithFaults(t, faults)
	store := sqlite.NewUserStore(handle)
	return store, faults, newFaultWorld(t, store, handle)
}

// wantFailure fails t unless err is errPlanted under the action the store names.
func wantFailure(t *testing.T, err error, action string) {
	t.Helper()
	if !errors.Is(err, errPlanted) || !strings.HasPrefix(err.Error(), "sqlite: "+action+": ") {
		t.Errorf("the call answered %v, want the planted failure under %q", err, "sqlite: "+action+": ")
	}
}

func TestEveryStatementFailureReachesTheCallerUnderItsAction(t *testing.T) {
	t.Parallel()

	for _, tc := range statementFaults {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, faults, world := faultyStore(t)
			faults.FailStatement(tc.statement, errPlanted)

			err := tc.call(t.Context(), store, world)

			wantFailure(t, err, tc.action)
		})
	}
}

func TestAFailedCommitReachesTheCallerAndKeepsNothing(t *testing.T) {
	t.Parallel()

	store, faults, world := faultyStore(t)
	faults.FailNextCommit(errPlanted)

	err := store.SetUserDisabled(t.Context(), world.admin.ID, true)

	wantFailure(t, err, "set user disabled")
	if stored, readErr := store.UserByID(t.Context(), world.admin.ID); readErr != nil || stored.Disabled {
		t.Errorf("after the failed commit the account reads disabled %t with %v, want it enabled", stored.Disabled,
			readErr)
	}
}

func TestACancelledContextReachesTheCallerFromATransaction(t *testing.T) {
	t.Parallel()

	store, _, world := faultyStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := store.SetUserDisabled(ctx, world.admin.ID, true)

	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "sqlite: set user disabled: ") {
		t.Errorf("SetUserDisabled() with a cancelled context = %v, want context.Canceled under the action", err)
	}
}
