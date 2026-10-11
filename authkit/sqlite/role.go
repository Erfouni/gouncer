// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/sqlite/internal/db"
)

// SetUserRole writes an account's role, refusing to leave no enabled account under a privileged one.
func (s *UserStore) SetUserRole(ctx context.Context, id uuid.UUID, role string, privileged gouncer.Roles) error {
	return s.underCover(ctx, "set user role", id, privileged, !privileged.Holds(role), func(queries *db.Queries) error {
		count, err := queries.SetUserRole(ctx, db.SetUserRoleParams{Role: role, ID: id})
		if err != nil {
			return err
		}
		if count == 0 {
			return gouncer.ErrUserNotFound
		}
		return nil
	})
}

// SetUserDisabledUnderCover disables an account, refusing to leave no enabled account privileged.
func (s *UserStore) SetUserDisabledUnderCover(
	ctx context.Context,
	id uuid.UUID,
	disabled bool,
	privileged gouncer.Roles,
) error {
	return s.underCover(ctx, "set user disabled", id, privileged, disabled, func(queries *db.Queries) error {
		return disable(ctx, queries, id, disabled)
	})
}

// SetUserRoleFrom writes the role of an account still holding held, refusing to leave no enabled account privileged.
func (s *UserStore) SetUserRoleFrom(
	ctx context.Context,
	id uuid.UUID,
	held, role string,
	privileged gouncer.Roles,
) error {
	return s.underCover(ctx, "set user role", id, privileged, !privileged.Holds(role), func(queries *db.Queries) error {
		count, err := queries.SetUserRoleFrom(ctx, db.SetUserRoleFromParams{Role: role, ID: id, Held: held})
		if err != nil {
			return err
		}
		if count == 0 {
			return missedOrMoved(ctx, queries, id)
		}
		return nil
	})
}

// SetUserDisabledFrom disables an account still holding held, refusing to leave no enabled account privileged.
func (s *UserStore) SetUserDisabledFrom(
	ctx context.Context,
	id uuid.UUID,
	held string,
	disabled bool,
	privileged gouncer.Roles,
) error {
	return s.underCover(ctx, "set user disabled", id, privileged, disabled, func(queries *db.Queries) error {
		params := db.SetUserDisabledFromParams{Disabled: disabled, ID: id, Held: held}
		count, err := queries.SetUserDisabledFrom(ctx, params)
		if err != nil {
			return err
		}
		if count == 0 {
			return missedOrMoved(ctx, queries, id)
		}
		return revoke(ctx, queries, id, disabled)
	})
}

// GrantRoleToRoleless gives a role to every account holding none, reporting how many took it.
func (s *UserStore) GrantRoleToRoleless(ctx context.Context, role string) (int64, error) {
	if role == "" {
		return 0, gouncer.ErrEmptyRole
	}
	granted, err := s.queries.GrantRoleToRoleless(ctx, role)
	if err != nil {
		return 0, failed("grant role", err)
	}
	return granted, nil
}

// underCover runs write in one write transaction, refusing it when it would remove the last cover.
func (s *UserStore) underCover(
	ctx context.Context,
	action string,
	id uuid.UUID,
	privileged gouncer.Roles,
	removesCover bool,
	write func(*db.Queries) error,
) error {
	return s.inTx(ctx, action, func(queries *db.Queries) error {
		if err := refuseUncovered(ctx, queries, id, privileged, removesCover); err != nil {
			return err
		}
		return write(queries)
	})
}

// refuseUncovered refuses a write that leaves no enabled account under a privileged role.
func refuseUncovered(
	ctx context.Context,
	queries *db.Queries,
	id uuid.UUID,
	privileged gouncer.Roles,
	removesCover bool,
) error {
	if len(privileged) == 0 || !removesCover {
		return nil
	}
	roles := jsonList([]string(privileged))
	held, err := queries.HoldsPrivilege(ctx, db.HoldsPrivilegeParams{ID: id, Roles: roles})
	if err != nil || held == 0 {
		return err
	}
	cover, err := queries.CountCover(ctx, db.CountCoverParams{Roles: roles, ID: id})
	if err != nil {
		return err
	}
	if cover == 0 {
		return gouncer.ErrLastPrivileged
	}
	return nil
}

// missedOrMoved returns gouncer.ErrUserNotFound when no account has the id, else authkit.ErrRoleChanged.
func missedOrMoved(ctx context.Context, queries *db.Queries, id uuid.UUID) error {
	_, err := queries.GetUserByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return gouncer.ErrUserNotFound
	}
	if err != nil {
		return err
	}
	return authkit.ErrRoleChanged
}

// jsonList returns values as the JSON array text that json_each reads.
func jsonList[T string | uuid.UUID](values []T) string {
	encoded, _ := json.Marshal(values)
	return string(encoded)
}
