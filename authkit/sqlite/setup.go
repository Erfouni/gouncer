// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/sqlite/internal/db"
)

// HasUsers reports whether the store holds any account, disabled and unconfirmed ones included.
func (s *UserStore) HasUsers(ctx context.Context) (bool, error) {
	held, err := s.queries.HasUsers(ctx)
	if err != nil {
		return false, failed("has users", err)
	}
	return held, nil
}

// CreateFirstUser stores u while the store holds no account, or answers authkit.ErrSetupDone and stores nothing.
func (s *UserStore) CreateFirstUser(ctx context.Context, u gouncer.User) error {
	return s.inTx(ctx, "create first user", func(queries *db.Queries) error {
		held, err := queries.HasUsers(ctx)
		if err != nil {
			return err
		}
		if held {
			return authkit.ErrSetupDone
		}
		return queries.CreateUser(ctx, newUserParams(u))
	})
}
