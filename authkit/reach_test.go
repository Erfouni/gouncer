// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

var (
	_ authkit.RoleSource = roleTable(nil)
	_ authkit.RoleSource = anyRole{}
	_ authkit.RoleSource = emptyAboveAll{}
	_ authkit.RoleSource = (*lateRoles)(nil)
)

// errLookupFailed is the failure a store answers when it cannot read accounts.
var errLookupFailed = errors.New("lookup failed")

// listOnly is an account store that answers no UserByID.
type listOnly struct {
	authkit.AdminStore
}

// failingLookup is an account store whose UserByID fails.
type failingLookup struct {
	*testkit.Store
}

// UserByID answers the lookup failure.
func (failingLookup) UserByID(context.Context, uuid.UUID) (gouncer.User, error) {
	return gouncer.User{}, errLookupFailed
}

// promotingStore is an account store that promotes each account to admin right after reading it.
type promotingStore struct {
	*testkit.Store
}

// UserByID returns the account as stored, then stores it promoted to admin.
func (s promotingStore) UserByID(ctx context.Context, id uuid.UUID) (gouncer.User, error) {
	read, err := s.Store.UserByID(ctx, id)
	promoted := read
	promoted.Role = "admin"
	s.Users[id] = promoted
	return read, err
}

// Known reports whether the table holds the role.
func (t roleTable) Known(role string) bool {
	_, ok := t[role]
	return ok
}

// Outranks reports whether the caller carries every capability the target carries.
func (t roleTable) Outranks(caller, target string) bool {
	carried, ok := t[target]
	if !ok {
		return false
	}
	for _, capability := range carried {
		if !slices.Contains(t[caller], capability) {
			return false
		}
	}
	return true
}

// anyRole is a stand-in source that knows every role and lets every caller hand it out.
type anyRole struct{}

// Known reports every role as known.
func (anyRole) Known(string) bool { return true }

// Outranks reports every caller as outranking every target.
func (anyRole) Outranks(string, string) bool { return true }

// emptyAboveAll is a stand-in source that knows every role and puts the empty role beyond every caller's reach.
type emptyAboveAll struct{}

// Known reports every role as known.
func (emptyAboveAll) Known(string) bool { return true }

// Outranks reports every caller as outranking every role but the empty one.
func (emptyAboveAll) Outranks(_, target string) bool { return target != "" }

// lateRoles is a stand-in source that learns each role of its table the second time it is asked about it.
type lateRoles struct {
	roleTable
	asked map[string]int
}

// Known reports whether the table holds the role, answering false the first time each role is asked.
func (l *lateRoles) Known(role string) bool {
	l.asked[role]++
	return l.asked[role] > 1 && l.roleTable.Known(role)
}

// ladder returns a table where admin outranks moderator, moderator outranks member, and organizer stands apart.
func ladder() roleTable {
	return roleTable{
		"admin":     {"events.manage", "settings.manage", "users.manage"},
		"moderator": {"users.manage"},
		"organizer": {"events.manage"},
		"member":    nil,
	}
}

// reachServer mounts the admin routes over the role source, signed in as the actor role.
func reachServer(t *testing.T, store *testkit.Store, roles authkit.RoleSource, actorRole string) http.Handler {
	t.Helper()
	return reachServerOver(t, store, store, roles, actorRole)
}

// reachServerOver mounts the admin routes writing through accounts, signed in to store as the actor role.
func reachServerOver(
	t *testing.T,
	store *testkit.Store,
	accounts authkit.AdminStore,
	roles authkit.RoleSource,
	actorRole string,
) http.Handler {
	t.Helper()
	addAdaAs(t, store, actorRole)
	privileged := gouncer.Roles{"admin", "moderator"}
	h := authkit.New(authkit.Config{Store: store, Privileged: privileged})
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: accounts, Privileged: privileged, Roles: roles})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.Handle("POST /api/users", h.RequireSession(http.HandlerFunc(admin.Create)))
	mux.Handle("PATCH /api/users/{id}", h.RequireSession(http.HandlerFunc(admin.SetDisabled)))
	mux.Handle("PUT /api/users/{id}/role", h.RequireSession(http.HandlerFunc(admin.SetRole)))
	return cookiedServer(mux, loginCookie(t, mux))
}

// addMariaAs stores a second test account holding the role.
func addMariaAs(t *testing.T, store *testkit.Store, role string) gouncer.User {
	t.Helper()
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	maria.Role = role
	store.Users[maria.ID] = maria
	return maria
}

// createMaria posts a new account for Maria Perez holding the role.
func createMaria(t *testing.T, srv http.Handler, role string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, srv, http.MethodPost, "/api/users",
		`{"email":"maria@example.com","name":"Maria Perez","password":"correct horse battery","role":"`+role+`"}`)
}

// setRole puts the role on the account.
func setRole(t *testing.T, srv http.Handler, account gouncer.User, role string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, srv, http.MethodPut, "/api/users/"+account.ID.String()+"/role", `{"role":"`+role+`"}`)
}

// setDisabled patches whether the account may log in.
func setDisabled(t *testing.T, srv http.Handler, account gouncer.User, disabled bool) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, srv, http.MethodPatch, "/api/users/"+account.ID.String(),
		`{"disabled":`+strconv.FormatBool(disabled)+`}`)
}

// disable marks the stored account as disabled.
func disable(store *testkit.Store, account gouncer.User) {
	account.Disabled = true
	store.Users[account.ID] = account
}

// wantRefused fails unless the response carries the status and the code.
func wantRefused(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, status, recorder.Body)
	}
	if got := decodeBody[errorBody](t, recorder).Code; got != code {
		t.Errorf("code = %q, want %q", got, code)
	}
}

// wantRoleUnknown fails unless the response refuses the role field as unknown.
func wantRoleUnknown(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	wantRefused(t, recorder, http.StatusUnprocessableEntity, "role_unknown")
	if field := decodeBody[errorBody](t, recorder).Meta["field"]; field != "role" {
		t.Errorf("meta field = %v, want %q", field, "role")
	}
}

// wantNoMaria fails when the store holds an account for Maria Perez.
func wantNoMaria(t *testing.T, store *testkit.Store) {
	t.Helper()
	if _, err := store.UserByEmail(t.Context(), "maria@example.com"); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("UserByEmail(maria) error = %v, want the refused account absent", err)
	}
}

func TestCreateRefusesARoleTheSourceDoesNotKnow(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "admin")

	wantRoleUnknown(t, createMaria(t, srv, "ghost"))
	wantNoMaria(t, store)
}

func TestCreateRefusesAnEmptyRoleWhateverTheSourceSays(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, anyRole{}, "admin")

	wantRoleUnknown(t, doRequest(t, srv, http.MethodPost, "/api/users",
		`{"email":"maria@example.com","name":"Maria Perez","password":"correct horse battery"}`))
	wantNoMaria(t, store)
}

func TestCreateRefusesARoleTheActorMayNotGive(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "moderator")

	wantRefused(t, createMaria(t, srv, "admin"), http.StatusForbidden, "role_not_grantable")
	wantRefused(t, createMaria(t, srv, "organizer"), http.StatusForbidden, "role_not_grantable")
	wantNoMaria(t, store)
}

func TestCreateGivesARoleWithinReach(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "moderator")

	recorder := createMaria(t, srv, "member")

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	if role := decodeBody[authkit.Account](t, recorder).Role; role != "member" {
		t.Errorf("role = %q, want %q", role, "member")
	}
}

func TestSetRoleRefusesARoleTheSourceDoesNotKnow(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "admin")
	maria := addMariaAs(t, store, "member")

	wantRoleUnknown(t, setRole(t, srv, maria, "ghost"))
	if held := store.Users[maria.ID].Role; held != "member" {
		t.Errorf("role = %q, want the refused write to leave %q", held, "member")
	}
}

func TestSetRoleRefusesAnEmptyRoleWhateverTheSourceSays(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, anyRole{}, "admin")
	maria := addMariaAs(t, store, "member")

	wantRoleUnknown(t, setRole(t, srv, maria, ""))
	if held := store.Users[maria.ID].Role; held != "member" {
		t.Errorf("role = %q, want the refused write to leave %q", held, "member")
	}
}

func TestSetRoleRefusesARoleTheActorMayNotGive(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "moderator")
	maria := addMariaAs(t, store, "member")

	wantRefused(t, setRole(t, srv, maria, "admin"), http.StatusForbidden, "role_not_grantable")
	if held := store.Users[maria.ID].Role; held != "member" {
		t.Errorf("role = %q, want the refused write to leave %q", held, "member")
	}
}

func TestSetRoleRefusesAnAccountTheActorMayNotEdit(t *testing.T) {
	t.Parallel()

	for _, role := range []string{"member", "organizer"} {
		t.Run("to "+role, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			srv := reachServer(t, store, ladder(), "moderator")
			maria := addMariaAs(t, store, "admin")

			wantRefused(t, setRole(t, srv, maria, role), http.StatusForbidden, "user_not_editable")
			if held := store.Users[maria.ID].Role; held != "admin" {
				t.Errorf("role = %q, want the refused write to leave %q", held, "admin")
			}
		})
	}
}

func TestSetRoleChangesAnAccountWhoseRoleNoSourceKnows(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "moderator")
	maria := addMariaAs(t, store, "withdrawn")

	recorder := setRole(t, srv, maria, "member")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
	}
	if held := store.Users[maria.ID].Role; held != "member" {
		t.Errorf("role = %q, want %q", held, "member")
	}
}

func TestSetRoleGivesARoleWithinReach(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "admin")
	maria := addMariaAs(t, store, "member")

	recorder := setRole(t, srv, maria, "moderator")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
	}
	if held := store.Users[maria.ID].Role; held != "moderator" {
		t.Errorf("role = %q, want %q", held, "moderator")
	}
}

func TestSetRoleOfAMissingAccountStaysNotFound(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "admin")

	recorder := doRequest(t, srv, http.MethodPut, "/api/users/"+uuidZero+"/role", `{"role":"member"}`)

	wantRefused(t, recorder, http.StatusNotFound, "user_not_found")
}

func TestReachChecksReportAFailedAccountLookup(t *testing.T) {
	t.Parallel()

	for _, held := range []struct {
		name   string
		method string
		target func(id string) string
		body   string
	}{
		{"role", http.MethodPut, func(id string) string { return "/api/users/" + id + "/role" }, `{"role":"member"}`},
		{"disable", http.MethodPatch, func(id string) string { return "/api/users/" + id }, `{"disabled":true}`},
	} {
		for _, lookup := range lookups() {
			t.Run(held.name+" through "+lookup.name, func(t *testing.T) {
				t.Parallel()

				store := testkit.NewStore()
				srv := reachServerOver(t, store, lookup.failing(store), ladder(), "admin")
				maria := addMariaAs(t, store, "moderator")

				recorder := doRequest(t, srv, held.method, held.target(maria.ID.String()), held.body)

				wantRefused(t, recorder, http.StatusInternalServerError, "internal")
				if after := store.Users[maria.ID]; after.Role != "moderator" || after.Disabled {
					t.Errorf("account = role %q disabled %v, want it untouched", after.Role, after.Disabled)
				}
			})
		}
	}
}

// accountLookup is one way the admin handlers read an account, over a store built to answer or to fail.
type accountLookup struct {
	name    string
	working func(store *testkit.Store) authkit.AdminStore
	failing func(store *testkit.Store) authkit.AdminStore
}

// lookups returns the read by id and the read through the account list.
func lookups() []accountLookup {
	return []accountLookup{
		{
			name:    "UserByID",
			working: func(store *testkit.Store) authkit.AdminStore { return store },
			failing: func(store *testkit.Store) authkit.AdminStore { return failingLookup{store} },
		},
		{
			name:    "ListUsers",
			working: func(store *testkit.Store) authkit.AdminStore { return listOnly{store} },
			failing: func(store *testkit.Store) authkit.AdminStore {
				store.ListUsersErr = errLookupFailed
				return listOnly{store}
			},
		},
	}
}

func TestReenablingAMissingAccountIsNotFound(t *testing.T) {
	t.Parallel()

	for _, lookup := range lookups() {
		t.Run(lookup.name, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			srv := reachServerOver(t, store, lookup.working(store), ladder(), "admin")

			recorder := doRequest(t, srv, http.MethodPatch, "/api/users/"+uuidZero, `{"disabled":false}`)

			wantRefused(t, recorder, http.StatusNotFound, "user_not_found")
		})
	}
}

func TestWritesReadTheAccountListWhenTheStoreCannotReadByID(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServerOver(t, store, listOnly{store}, ladder(), "admin")
	maria := addMariaAs(t, store, "member")

	if recorder := setRole(t, srv, maria, "moderator"); recorder.Code != http.StatusNoContent {
		t.Fatalf("role status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
	}
	if recorder := setDisabled(t, srv, maria, true); recorder.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
	}
	if after := store.Users[maria.ID]; after.Role != "moderator" || !after.Disabled {
		t.Errorf("account = role %q disabled %v, want moderator and disabled", after.Role, after.Disabled)
	}
}

func TestSetRoleUnderASourceRefusesTheActorsOwnRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "moderator")
	actor := byEmail(t, store, "ada@example.com")

	wantRefused(t, setRole(t, srv, actor, "member"), http.StatusUnprocessableEntity, "self_role_refused")
	if held := store.Users[actor.ID].Role; held != "moderator" {
		t.Errorf("role = %q, want the refused write to leave %q", held, "moderator")
	}
}

// byEmail returns the stored account at email, failing the test without one.
func byEmail(t *testing.T, store *testkit.Store, email string) gouncer.User {
	t.Helper()
	u, err := store.UserByEmail(t.Context(), email)
	if err != nil {
		t.Fatalf("UserByEmail(%s) error = %v, want nil", email, err)
	}
	return u
}

func TestTheAccountIsReadByIDWhenTheStoreCan(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, ladder(), "admin")
	maria := addMariaAs(t, store, "member")
	store.ListUsersErr = errLookupFailed

	if recorder := setRole(t, srv, maria, "moderator"); recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d without listing the accounts, body %s",
			recorder.Code, http.StatusNoContent, recorder.Body)
	}
}

func TestARoleMovedBeforeTheWriteAnswersConflict(t *testing.T) {
	t.Parallel()

	for _, write := range []struct {
		name string
		send func(t *testing.T, srv http.Handler, account gouncer.User) *httptest.ResponseRecorder
	}{
		{"role", func(t *testing.T, srv http.Handler, account gouncer.User) *httptest.ResponseRecorder {
			return setRole(t, srv, account, "moderator")
		}},
		{"disable", func(t *testing.T, srv http.Handler, account gouncer.User) *httptest.ResponseRecorder {
			return setDisabled(t, srv, account, true)
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			srv := reachServerOver(t, store, promotingStore{store}, ladder(), "moderator")
			maria := addMariaAs(t, store, "member")

			wantRefused(t, write.send(t, srv, maria), http.StatusConflict, "role_changed")
			if after := store.Users[maria.ID]; after.Role != "admin" || after.Disabled {
				t.Errorf("account = role %q disabled %v, want the promotion kept and the write refused",
					after.Role, after.Disabled)
			}
		})
	}
}

func TestErrorResponseForAuthErrorNamesAChangedRole(t *testing.T) {
	t.Parallel()

	status, response, ok := authkit.ErrorResponseForAuthError(authkit.ErrRoleChanged)

	if !ok || status != http.StatusConflict || response.Code != "role_changed" {
		t.Errorf("ErrorResponseForAuthError() = %d, %+v, %v, want 409 role_changed", status, response, ok)
	}
}

func TestSetDisabledRefusesAnAccountTheActorMayNotEdit(t *testing.T) {
	t.Parallel()

	for _, disabled := range []bool{true, false} {
		t.Run("disabled "+strconv.FormatBool(disabled), func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			srv := reachServer(t, store, ladder(), "moderator")
			maria := addMariaAs(t, store, "admin")
			if !disabled {
				disable(store, maria)
			}

			wantRefused(t, setDisabled(t, srv, maria, disabled), http.StatusForbidden, "user_not_editable")
			if store.Users[maria.ID].Disabled == disabled {
				t.Errorf("disabled = %v, want the refused write to leave %v", disabled, !disabled)
			}
		})
	}
}

func TestSetDisabledAdmitsAnAccountWithinReach(t *testing.T) {
	t.Parallel()

	for _, disabled := range []bool{true, false} {
		t.Run("disabled "+strconv.FormatBool(disabled), func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			srv := reachServer(t, store, ladder(), "moderator")
			maria := addMariaAs(t, store, "member")
			if !disabled {
				disable(store, maria)
			}

			recorder := setDisabled(t, srv, maria, disabled)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
			}
			if store.Users[maria.ID].Disabled != disabled {
				t.Errorf("disabled = %v, want %v", !disabled, disabled)
			}
		})
	}
}

func TestReenablingRefusesAnAccountHoldingNoDeclaredRole(t *testing.T) {
	t.Parallel()

	for _, held := range []string{"withdrawn", ""} {
		t.Run("role "+strconv.Quote(held), func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			srv := reachServer(t, store, ladder(), "admin")
			maria := addMariaAs(t, store, held)
			disable(store, maria)

			wantRefused(t, setDisabled(t, srv, maria, false), http.StatusForbidden, "role_not_grantable")
			if !store.Users[maria.ID].Disabled {
				t.Error("account enabled, want the refused write to leave it disabled")
			}
		})
	}
}

func TestDisablingReachesAnAccountHoldingNoDeclaredRole(t *testing.T) {
	t.Parallel()

	for _, held := range []string{"withdrawn", ""} {
		t.Run("role "+strconv.Quote(held), func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			srv := reachServer(t, store, ladder(), "moderator")
			maria := addMariaAs(t, store, held)

			recorder := setDisabled(t, srv, maria, true)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
			}
			if !store.Users[maria.ID].Disabled {
				t.Error("account enabled, want it disabled")
			}
		})
	}
}

func TestWritesWithoutARoleSourceListNoAccounts(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServerOver(t, store, failingLookup{store}, nil, "admin")
	maria := addMariaAs(t, store, "member")
	store.ListUsersErr = errLookupFailed

	if recorder := setRole(t, srv, maria, "editor"); recorder.Code != http.StatusNoContent {
		t.Errorf("role status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
	}
	if recorder := setDisabled(t, srv, maria, true); recorder.Code != http.StatusNoContent {
		t.Errorf("disable status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
	}
	if after := store.Users[maria.ID]; after.Role != "editor" || !after.Disabled {
		t.Errorf("account = role %q disabled %v, want role %q disabled", after.Role, after.Disabled, "editor")
	}
}

func TestAnEmptyRoleIsUnknownEvenToASourceThatDeclaresIt(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, emptyAboveAll{}, "admin")
	maria := addMariaAs(t, store, "member")

	wantRoleUnknown(t, doRequest(t, srv, http.MethodPost, "/api/users",
		`{"email":"other@example.com","name":"Maria Perez","password":"correct horse battery"}`))
	wantRoleUnknown(t, setRole(t, srv, maria, ""))
	if held := store.Users[maria.ID].Role; held != "member" {
		t.Errorf("role = %q, want the refused write to leave %q", held, "member")
	}
}

func TestAnAccountHoldingTheEmptyRoleStaysEditable(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, emptyAboveAll{}, "admin")
	maria := addMariaAs(t, store, "")

	recorder := setRole(t, srv, maria, "member")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
	}
	wantRefused(t, doRequest(t, srv, http.MethodPut, "/api/users/"+uuidZero+"/role", `{"role":"member"}`),
		http.StatusNotFound, "user_not_found")
}

func TestCreateNeverHandsOutARoleDeclaredDuringTheRequest(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, &lateRoles{roleTable: ladder(), asked: map[string]int{}}, "moderator")

	wantRoleUnknown(t, createMaria(t, srv, "admin"))
	wantNoMaria(t, store)
}

func TestSetRoleNeverHandsOutARoleDeclaredDuringTheRequest(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := reachServer(t, store, &lateRoles{roleTable: ladder(), asked: map[string]int{}}, "moderator")
	maria := addMariaAs(t, store, "member")

	wantRoleUnknown(t, setRole(t, srv, maria, "admin"))
	if held := store.Users[maria.ID].Role; held != "member" {
		t.Errorf("role = %q, want the refused write to leave %q", held, "member")
	}
}

func TestAccountWritesRefuseARoleTheSourceDoesNotKnow(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	actor := addAdaAs(t, store, "admin")
	maria := addMariaAs(t, store, "member")
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store, Roles: ladder()})

	for _, role := range []string{"ghost", ""} {
		_, err := admin.CreateAccount(t.Context(), "other@example.com", "Maria Perez", testPassword, role)
		if !errors.Is(err, authkit.ErrUnknownRole) {
			t.Errorf("CreateAccount(role %q) error = %v, want ErrUnknownRole", role, err)
		}
		err = admin.SetAccountRole(t.Context(), actor.ID, maria.ID, role)
		if !errors.Is(err, authkit.ErrUnknownRole) {
			t.Errorf("SetAccountRole(role %q) error = %v, want ErrUnknownRole", role, err)
		}
	}

	if held := store.Users[maria.ID].Role; held != "member" {
		t.Errorf("role = %q, want the refused writes to leave %q", held, "member")
	}
	if _, err := store.UserByEmail(t.Context(), "other@example.com"); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("UserByEmail(other) error = %v, want the refused account absent", err)
	}
}

func TestSetAccountRoleRefusesTheActorsOwnRoleFirst(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	actor := addAdaAs(t, store, "admin")
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store, Roles: ladder()})

	if err := admin.SetAccountRole(t.Context(), actor.ID, actor.ID, "ghost"); !errors.Is(err, authkit.ErrSelfRole) {
		t.Errorf("SetAccountRole(own, ghost) error = %v, want ErrSelfRole", err)
	}
}

func TestAccountWritesRefuseAnEmptyRoleWhateverTheSourceSays(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	actor := addAdaAs(t, store, "admin")
	maria := addMariaAs(t, store, "member")
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store, Roles: anyRole{}})
	service, _, _ := invites(authkit.InvitesConfig{Roles: anyRole{}})

	if _, err := admin.CreateAccount(t.Context(), "other@example.com", "Maria Perez", testPassword, ""); !errors.Is(
		err, authkit.ErrUnknownRole) {
		t.Errorf("CreateAccount(no role) error = %v, want ErrUnknownRole", err)
	}
	if err := admin.SetAccountRole(t.Context(), actor.ID, maria.ID, ""); !errors.Is(err, authkit.ErrUnknownRole) {
		t.Errorf("SetAccountRole(no role) error = %v, want ErrUnknownRole", err)
	}
	if _, err := service.Invite(t.Context(), "other@example.com", "Maria Perez", ""); !errors.Is(
		err, authkit.ErrUnknownRole) {
		t.Errorf("Invite(no role) error = %v, want ErrUnknownRole", err)
	}
}

func TestInviteRefusesARoleTheSourceDoesNotKnow(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{Roles: ladder()})

	if _, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "ghost"); !errors.Is(
		err, authkit.ErrUnknownRole) {
		t.Fatalf("Invite(ghost) error = %v, want ErrUnknownRole", err)
	}
	wantNoMaria(t, store)

	if _, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member"); err != nil {
		t.Errorf("Invite(member) error = %v, want nil", err)
	}
}

func TestAccountWritesTakeAnyRoleWithoutASource(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	actor := addAdaAs(t, store, "admin")
	maria := addMariaAs(t, store, "member")
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store})
	service, _, _ := invites(authkit.InvitesConfig{})

	if _, err := admin.CreateAccount(t.Context(), "other@example.com", "Maria Perez", testPassword, ""); err != nil {
		t.Errorf("CreateAccount(no role) error = %v, want nil", err)
	}
	if err := admin.SetAccountRole(t.Context(), actor.ID, maria.ID, "ghost"); err != nil {
		t.Errorf("SetAccountRole(ghost) error = %v, want nil", err)
	}
	if _, err := service.Invite(t.Context(), "other@example.com", "Maria Perez", "ghost"); err != nil {
		t.Errorf("Invite(ghost) error = %v, want nil", err)
	}
}

func TestErrorResponseForAuthErrorNamesAnUnknownRole(t *testing.T) {
	t.Parallel()

	status, response, ok := authkit.ErrorResponseForAuthError(authkit.ErrUnknownRole)

	if !ok || status != http.StatusUnprocessableEntity {
		t.Fatalf("ErrorResponseForAuthError() = %d, %v, want %d, true", status, ok, http.StatusUnprocessableEntity)
	}
	if response.Code != "role_unknown" || response.Meta["field"] != "role" {
		t.Errorf("response = %+v, want the role_unknown code naming the role field", response)
	}
}
