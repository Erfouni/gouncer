// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

var (
	_ authkit.CapabilitySource = roleTable(nil)
	_ authkit.GrantableSource  = rankedTable{}
)

const adaCredentials = `{"email":"ada@example.com","password":"correct horse battery"}`

// roleTable is a stand-in source holding the capabilities each role carries.
type roleTable map[string][]string

// CapabilitiesOf returns the capabilities the table holds for the role.
func (t roleTable) CapabilitiesOf(role string) []string {
	return t[role]
}

// rankedTable is a stand-in source that also answers the roles each caller may hand out.
type rankedTable struct {
	roleTable
	grantable map[string][]string
}

// Grantable returns the roles the table lists for the caller.
func (t rankedTable) Grantable(caller string) []string {
	return t.grantable[caller]
}

// addAdaAs stores the default test user holding the role.
func addAdaAs(t *testing.T, store *testkit.Store, role string) gouncer.User {
	t.Helper()
	ada := addAda(t, store)
	ada.Role = role
	store.Users[ada.ID] = ada
	return ada
}

// capabilityServer mounts login, the session route and a route guarded by the events.manage capability.
func capabilityServer(cfg authkit.Config) http.Handler {
	h := authkit.New(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.Handle("GET /api/auth/session", h.RequireSession(http.HandlerFunc(h.Session)))
	mux.Handle("GET /api/events", h.RequireSession(h.RequireCapability("events.manage")(noContent())))
	return mux
}

// noContent answers every request with 204.
func noContent() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// getSigned performs a GET against handler carrying the session cookie.
func getSigned(t *testing.T, handler http.Handler, target string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// signedIn logs the default test user in, failing without a session, and returns the login response.
func signedIn(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	recorder := doLogin(t, handler, adaCredentials)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", recorder.Code, http.StatusOK)
	}
	return recorder
}

// wantCapabilityMissing fails unless the response refuses the request for lacking the capability.
func wantCapabilityMissing(t *testing.T, recorder *httptest.ResponseRecorder, capability string) {
	t.Helper()
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	body := decodeBody[errorBody](t, recorder)
	if body.Code != "capability_missing" || body.Error != "capability missing" {
		t.Errorf("body = %+v, want the capability_missing code and message", body)
	}
	if body.Meta["capability"] != capability {
		t.Errorf("meta = %v, want the capability %q", body.Meta, capability)
	}
}

func TestLoginAndSessionKeepTheirBytesWithoutACapabilitySource(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	ada := addAdaAs(t, store, "admin")
	handler := capabilityServer(authkit.Config{Store: store})
	want := `{"id":"` + ada.ID.String() + `","email":"ada@example.com","name":"Ada Lovelace","role":"admin"}`

	login := signedIn(t, handler)
	session := getSigned(t, handler, "/api/auth/session", sessionCookie(t, login, testCookieName))

	if got := login.Body.String(); got != want {
		t.Errorf("login body = %s, want %s", got, want)
	}
	if got := session.Body.String(); got != want {
		t.Errorf("session body = %s, want %s", got, want)
	}
}

func TestLoginAndSessionCarryTheCapabilitiesOfTheRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "admin")
	handler := capabilityServer(authkit.Config{
		Store:        store,
		Capabilities: roleTable{"admin": {"events.manage", "users.manage"}},
	})

	login := signedIn(t, handler)
	session := getSigned(t, handler, "/api/auth/session", sessionCookie(t, login, testCookieName))

	for name, recorder := range map[string]*httptest.ResponseRecorder{"login": login, "session": session} {
		fields := decodeBody[map[string]json.RawMessage](t, recorder)
		if got, want := string(fields["capabilities"]), `["events.manage","users.manage"]`; got != want {
			t.Errorf("%s capabilities = %s, want %s", name, got, want)
		}
		if grantable, ok := fields["grantable"]; ok {
			t.Errorf("%s grantable = %s, want the key absent for a source that answers no Grantable", name, grantable)
		}
	}
}

func TestARoleCarryingNothingSendsEmptyLists(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "member")
	handler := capabilityServer(authkit.Config{
		Store:        store,
		Capabilities: rankedTable{roleTable: roleTable{"member": nil}},
	})

	login := signedIn(t, handler)
	session := getSigned(t, handler, "/api/auth/session", sessionCookie(t, login, testCookieName))

	for name, recorder := range map[string]*httptest.ResponseRecorder{"login": login, "session": session} {
		fields := decodeBody[map[string]json.RawMessage](t, recorder)
		if got := string(fields["capabilities"]); got != "[]" {
			t.Errorf("%s capabilities = %q, want []", name, got)
		}
		if got := string(fields["grantable"]); got != "[]" {
			t.Errorf("%s grantable = %q, want []", name, got)
		}
	}
}

func TestLoginAndSessionCarryGrantableInTheSourcesOrder(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "admin")
	handler := capabilityServer(authkit.Config{
		Store: store,
		Capabilities: rankedTable{
			roleTable: roleTable{"admin": {"users.manage"}},
			grantable: map[string][]string{"admin": {"admin", "moderator", "editor", "member"}},
		},
	})

	login := signedIn(t, handler)
	session := getSigned(t, handler, "/api/auth/session", sessionCookie(t, login, testCookieName))

	for name, recorder := range map[string]*httptest.ResponseRecorder{"login": login, "session": session} {
		fields := decodeBody[map[string]json.RawMessage](t, recorder)
		if got, want := string(fields["grantable"]), `["admin","moderator","editor","member"]`; got != want {
			t.Errorf("%s grantable = %s, want %s", name, got, want)
		}
	}
}

func TestAuthenticateCarriesTheCapabilitiesOfTheRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "editor")
	handlers := authkit.New(authkit.Config{
		Store:        store,
		Capabilities: roleTable{"editor": {"posts.edit", "posts.publish"}},
	})

	identity, err := handlers.Authenticate(t.Context(), "ada@example.com", testPassword)
	if err != nil {
		t.Fatalf("Authenticate() error = %v, want nil", err)
	}

	if want := []string{"posts.edit", "posts.publish"}; !slices.Equal(identity.Capabilities, want) {
		t.Errorf("capabilities = %q, want %q", identity.Capabilities, want)
	}
	if identity.Grantable != nil {
		t.Errorf("grantable = %q, want nil for a source that answers no Grantable", identity.Grantable)
	}
}

func TestIdentityKeepsItsOwnCopyOfTheLists(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "editor")
	source := rankedTable{
		roleTable: roleTable{"editor": {"posts.edit"}},
		grantable: map[string][]string{"editor": {"member"}},
	}
	handlers := authkit.New(authkit.Config{Store: store, Capabilities: source})

	identity, err := handlers.Authenticate(t.Context(), "ada@example.com", testPassword)
	if err != nil {
		t.Fatalf("Authenticate() error = %v, want nil", err)
	}
	if len(identity.Capabilities) != 1 || len(identity.Grantable) != 1 {
		t.Fatalf("identity lists = %q and %q, want the one entry each the source holds",
			identity.Capabilities, identity.Grantable)
	}
	identity.Capabilities[0] = "users.manage"
	identity.Grantable[0] = "admin"

	if got := source.roleTable["editor"][0]; got != "posts.edit" {
		t.Errorf("source capability = %q after changing the identity, want %q", got, "posts.edit")
	}
	if got := source.grantable["editor"][0]; got != "member" {
		t.Errorf("source grantable role = %q after changing the identity, want %q", got, "member")
	}
}

func TestAGrantOrARevokeReachesTheNextRequest(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "editor")
	table := roleTable{"editor": {"posts.edit"}}
	handler := capabilityServer(authkit.Config{Store: store, Capabilities: table})
	cookie := sessionCookie(t, signedIn(t, handler), testCookieName)

	wantCapabilityMissing(t, getSigned(t, handler, "/api/events", cookie), "events.manage")

	table["editor"] = []string{"events.manage", "posts.edit"}
	if recorder := getSigned(t, handler, "/api/events", cookie); recorder.Code != http.StatusNoContent {
		t.Errorf("status after the grant = %d, want %d", recorder.Code, http.StatusNoContent)
	}

	table["editor"] = []string{"posts.edit"}
	wantCapabilityMissing(t, getSigned(t, handler, "/api/events", cookie), "events.manage")
}

func TestRequireCapabilityAdmitsARoleCarryingIt(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "organizer")
	handler := capabilityServer(authkit.Config{
		Store:        store,
		Capabilities: roleTable{"organizer": {"events.manage"}},
	})
	cookie := sessionCookie(t, signedIn(t, handler), testCookieName)

	if recorder := getSigned(t, handler, "/api/events", cookie); recorder.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRequireCapabilityRefusesARoleWithoutIt(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "editor")
	handler := capabilityServer(authkit.Config{
		Store:        store,
		Capabilities: roleTable{"editor": {"posts.edit"}, "organizer": {"events.manage"}},
	})
	cookie := sessionCookie(t, signedIn(t, handler), testCookieName)

	wantCapabilityMissing(t, getSigned(t, handler, "/api/events", cookie), "events.manage")
}

func TestRequireCapabilityRefusesAnAccountHoldingNoRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "")
	handler := capabilityServer(authkit.Config{
		Store:        store,
		Capabilities: roleTable{"organizer": {"events.manage"}},
	})
	cookie := sessionCookie(t, signedIn(t, handler), testCookieName)

	wantCapabilityMissing(t, getSigned(t, handler, "/api/events", cookie), "events.manage")
}

func TestRequireCapabilityRefusesARequestWithNoIdentity(t *testing.T) {
	t.Parallel()

	handlers := authkit.New(authkit.Config{
		Store:        testkit.NewStore(),
		Capabilities: roleTable{"organizer": {"users.manage"}},
	})
	recorder := httptest.NewRecorder()

	handlers.RequireCapability("users.manage")(noContent()).ServeHTTP(
		recorder, httptest.NewRequest(http.MethodGet, "/api/users", nil))

	wantCapabilityMissing(t, recorder, "users.manage")
}

func TestRequireCapabilityRefusesEveryRequestWithoutASource(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAdaAs(t, store, "admin")
	handler := capabilityServer(authkit.Config{Store: store})
	cookie := sessionCookie(t, signedIn(t, handler), testCookieName)

	wantCapabilityMissing(t, getSigned(t, handler, "/api/events", cookie), "events.manage")
}
