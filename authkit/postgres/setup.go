// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/gouncer/authkit/postgres/internal/db"
)

// UserStore implements authkit.SetupStore.
var _ authkit.SetupStore = (*UserStore)(nil)

// lockUsers makes every other write on the accounts wait until the transaction ends, and lets reads go on.
const lockUsers = `LOCK TABLE auth.users IN SHARE ROW EXCLUSIVE MODE`

// HasUsers reports whether the store holds any account, disabled and unconfirmed ones included.
func (s *UserStore) HasUsers(ctx context.Context) (bool, error) {
	held, err := s.queries.HasUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("postgres: has users: %w", err)
	}
	return held, nil
}

// CreateFirstUser stores u while the store holds no account, or answers authkit.ErrSetupDone and stores nothing.
func (s *UserStore) CreateFirstUser(ctx context.Context, u gouncer.User) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, lockUsers); err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	queries := s.queries.WithTx(tx)
	held, err := queries.HasUsers(ctx)
	if err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	if held {
		return authkit.ErrSetupDone
	}
	err = queries.CreateUser(ctx, db.CreateUserParams{
		ID:           u.ID,
		Email:        u.Email,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Disabled:     u.Disabled,
		Confirmed:    u.Confirmed,
		Role:         u.Role,
		CreatedAt:    u.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	return nil
}
