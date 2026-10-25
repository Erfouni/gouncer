// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// ErrUnknownRole reports a role the role source does not declare.
var ErrUnknownRole = errors.New("authkit: unknown role")

// ErrUserNotEditable reports an account whose role carries a capability the actor's role lacks.
var ErrUserNotEditable = errors.New("authkit: user not editable")

// ErrRoleNotGrantable reports a role the actor may not give, or an undeclared one held by an account to re-enable.
var ErrRoleNotGrantable = errors.New("authkit: role not grantable")

// ErrRoleChanged reports an account whose role is no longer the one the write was checked against.
var ErrRoleChanged = errors.New("authkit: role changed")

// RoleSource answers which roles exist and which roles a caller may hand out.
type RoleSource interface {
	// Known reports whether the role exists.
	Known(role string) bool

	// Outranks reports whether the caller carries every capability the target carries.
	Outranks(caller, target string) bool
}

// declared reports whether the source declares the role, every role passing without a source.
func declared(source RoleSource, role string) bool {
	return source == nil || (role != "" && source.Known(role))
}

// editable reports whether a reader holding reader may edit an account holding held, every account without a source.
func (a *AdminHandlers) editable(reader, held string, heldDeclared bool) bool {
	return a.roles == nil || !heldDeclared || a.roles.Outranks(reader, held)
}

// refuseHandout reports whether it refused handing out an undeclared role or one the actor may not give.
func (a *AdminHandlers) refuseHandout(w http.ResponseWriter, r *http.Request, role string) bool {
	if a.roles == nil {
		return false
	}
	if !declared(a.roles, role) {
		respondAuthError(w, ErrUnknownRole)
		return true
	}
	if a.roles.Outranks(IdentityFromContext(r.Context()).Role, role) {
		return false
	}
	respondAuthError(w, ErrRoleNotGrantable)
	return true
}

// heldEditable returns the role the account holds and whether the actor may edit the account.
func (a *AdminHandlers) heldEditable(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	reenable bool,
) (string, bool) {
	if a.roles == nil {
		return "", true
	}
	held, err := a.heldRole(r.Context(), id)
	if err != nil {
		respondAuthError(w, err)
		return "", false
	}
	if err := a.heldRefusal(IdentityFromContext(r.Context()).Role, held, reenable); err != nil {
		respondAuthError(w, err)
		return held, false
	}
	return held, true
}

// heldRefusal returns why the actor may not write to an account holding held, or nil when it may.
func (a *AdminHandlers) heldRefusal(actor, held string, reenable bool) error {
	heldDeclared := declared(a.roles, held)
	if !a.editable(actor, held, heldDeclared) {
		return ErrUserNotEditable
	}
	if reenable && !heldDeclared {
		return ErrRoleNotGrantable
	}
	return nil
}

// roleChangeHeld returns the role the account holds and whether the actor may move the account to role.
func (a *AdminHandlers) roleChangeHeld(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	role string,
) (string, bool) {
	held, ok := a.heldEditable(w, r, id, false)
	return held, ok && !a.refuseHandout(w, r, role)
}

// accountReader is a store that reads one account by id.
type accountReader interface {
	UserByID(ctx context.Context, id uuid.UUID) (gouncer.User, error)
}

// heldRole returns the role the account holds, or gouncer.ErrUserNotFound when no account has the id.
func (a *AdminHandlers) heldRole(ctx context.Context, id uuid.UUID) (string, error) {
	if reader, ok := a.store.(accountReader); ok {
		u, err := reader.UserByID(ctx, id)
		return u.Role, err
	}
	users, err := a.store.ListUsers(ctx)
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if u.ID == id {
			return u.Role, nil
		}
	}
	return "", gouncer.ErrUserNotFound
}
