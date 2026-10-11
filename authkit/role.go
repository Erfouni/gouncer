// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

// ErrUnknownRole reports a role the role source does not declare.
var ErrUnknownRole = errors.New("authkit: unknown role")

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

// refuseHandout reports whether it refused handing out an undeclared role or one the actor does not outrank.
func (a *AdminHandlers) refuseHandout(w http.ResponseWriter, r *http.Request, role string) bool {
	if a.roles == nil {
		return false
	}
	if !declared(a.roles, role) {
		respondAuthError(w, ErrUnknownRole)
		return true
	}
	return a.refuseOutranking(w, r, role)
}

// refuseAccountBeyondReach reports whether it refused an actor not outranking the declared role the account holds.
func (a *AdminHandlers) refuseAccountBeyondReach(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	reenable bool,
) bool {
	if a.roles == nil {
		return false
	}
	held, err := a.heldRole(r.Context(), id)
	if err != nil {
		respondAuthError(w, err)
		return true
	}
	if declared(a.roles, held) {
		return a.refuseOutranking(w, r, held)
	}
	if reenable {
		respondBeyondReach(w)
	}
	return reenable
}

// refuseRoleChange reports whether it refused moving the account to the role.
func (a *AdminHandlers) refuseRoleChange(w http.ResponseWriter, r *http.Request, id uuid.UUID, role string) bool {
	return a.refuseAccountBeyondReach(w, r, id, false) || a.refuseHandout(w, r, role)
}

// refuseOutranking reports whether it refused an actor that does not outrank the role.
func (a *AdminHandlers) refuseOutranking(w http.ResponseWriter, r *http.Request, role string) bool {
	if a.roles.Outranks(IdentityFromContext(r.Context()).Role, role) {
		return false
	}
	respondBeyondReach(w)
	return true
}

// respondBeyondReach answers that the role lies beyond the actor's reach.
func respondBeyondReach(w http.ResponseWriter) {
	RespondError(w, http.StatusForbidden, ErrorResponse{Message: "role beyond reach", Code: "role_beyond_reach"})
}

// heldRole returns the role the account holds, or the empty role when no account has the id.
func (a *AdminHandlers) heldRole(ctx context.Context, id uuid.UUID) (string, error) {
	users, err := a.store.ListUsers(ctx)
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if u.ID == id {
			return u.Role, nil
		}
	}
	return "", nil
}
