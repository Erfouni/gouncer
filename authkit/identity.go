// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// Identity is the authenticated user exposed to handlers, deliberately
// excluding credentials such as the password hash.
type Identity struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Capabilities []string  `json:"capabilities,omitzero"`
	Grantable    []string  `json:"grantable,omitzero"`
}

// CapabilitySource answers the capabilities a role carries.
type CapabilitySource interface {
	// CapabilitiesOf returns the capabilities the role carries.
	CapabilitiesOf(role string) []string
}

// GrantableSource answers the roles a caller may hand out.
type GrantableSource interface {
	// Grantable returns the roles the caller may hand out, the one carrying most capabilities first.
	Grantable(caller string) []string
}

// identityOf returns the identity a user is known by, with what its role carries and without its credentials.
func (h *Handlers) identityOf(u gouncer.User) Identity {
	identity := Identity{ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role}
	if h.capabilities == nil {
		return identity
	}
	identity.Capabilities = append([]string{}, h.capabilities.CapabilitiesOf(u.Role)...)
	if grantable, ok := h.capabilities.(GrantableSource); ok {
		identity.Grantable = append([]string{}, grantable.Grantable(u.Role)...)
	}
	return identity
}

type contextKey int

const identityContextKey contextKey = 0

// WithIdentity returns a context carrying the authenticated user's identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityContextKey, id)
}

// IdentityFromContext returns the identity stored by the RequireSession
// middleware, or the zero identity outside of it.
func IdentityFromContext(ctx context.Context) Identity {
	id, _ := ctx.Value(identityContextKey).(Identity)
	return id
}
