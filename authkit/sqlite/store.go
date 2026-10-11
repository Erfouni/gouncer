// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gopherium/framework/dbkit"
	dbsqlite "github.com/gopherium/framework/dbkit/sqlite"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/sqlite/internal/db"
)

// UserStore persists users, their sessions and their tokens in the auth tables of an application's SQLite file.
type UserStore struct {
	// handle is the application's one handle, whose write transactions begin immediately.
	handle *sql.DB
	// queries runs the generated statements on handle.
	queries *db.Queries
}

// NewUserStore returns a UserStore on the handle dbkit/sqlite opened for the application.
func NewUserStore(handle *sql.DB) *UserStore {
	return &UserStore{handle: handle, queries: db.New(handle)}
}

// CreateUser stores a new user, or answers gouncer.ErrEmailTaken when the email is already in use.
func (s *UserStore) CreateUser(ctx context.Context, u gouncer.User) error {
	err := s.queries.CreateUser(ctx, db.CreateUserParams{
		ID:           u.ID,
		Email:        u.Email,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Disabled:     u.Disabled,
		CreatedAt:    dbkit.Time{Time: u.CreatedAt},
		Role:         u.Role,
		Confirmed:    u.Confirmed,
	})
	if errors.Is(dbsqlite.Classify(err), dbkit.ErrUnique) {
		return gouncer.ErrEmailTaken
	}
	return failed("create user", err)
}

// UserByEmail returns the user owning the email, or gouncer.ErrUserNotFound when none does.
func (s *UserStore) UserByEmail(ctx context.Context, email string) (gouncer.User, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	return userOrMissing("get user by email", row, err, gouncer.ErrUserNotFound)
}

// UserByID returns the user with the id, disabled or not, or gouncer.ErrUserNotFound.
func (s *UserStore) UserByID(ctx context.Context, id uuid.UUID) (gouncer.User, error) {
	row, err := s.queries.GetUserByID(ctx, id)
	return userOrMissing("get user by id", row, err, gouncer.ErrUserNotFound)
}

// CreateSession stores a login session.
func (s *UserStore) CreateSession(ctx context.Context, session gouncer.Session) error {
	return failed("create session", s.queries.CreateSession(ctx, db.CreateSessionParams{
		TokenHash: session.TokenHash,
		UserID:    session.UserID,
		CreatedAt: dbkit.Time{Time: session.CreatedAt},
		ExpiresAt: dbkit.Time{Time: session.ExpiresAt},
	}))
}

// UserBySession returns the enabled user owning an unexpired session with the hash, or gouncer.ErrSessionNotFound.
func (s *UserStore) UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (gouncer.User, error) {
	row, err := s.queries.GetUserBySession(ctx, db.GetUserBySessionParams{
		TokenHash: tokenHash,
		ExpiresAt: dbkit.Time{Time: now},
	})
	return userOrMissing("get user by session", row, err, gouncer.ErrSessionNotFound)
}

// ListUsers returns every user ordered by name regardless of ASCII case, then by id, without password hashes.
func (s *UserStore) ListUsers(ctx context.Context) ([]gouncer.User, error) {
	rows, err := s.queries.ListUsers(ctx)
	if err != nil {
		return nil, failed("list users", err)
	}
	users := make([]gouncer.User, len(rows))
	for i, row := range rows {
		users[i] = gouncer.User{
			ID:        row.ID,
			Email:     row.Email,
			Name:      row.Name,
			Disabled:  row.Disabled,
			Confirmed: row.Confirmed,
			Role:      row.Role,
			CreatedAt: row.CreatedAt.Time,
		}
	}
	return users, nil
}

// SetUserDisabled writes whether the user may log in, ending its sessions and tokens on a disable.
func (s *UserStore) SetUserDisabled(ctx context.Context, id uuid.UUID, disabled bool) error {
	return s.inTx(ctx, "set user disabled", func(queries *db.Queries) error {
		return disable(ctx, queries, id, disabled)
	})
}

// DeleteExpiredSessions removes the sessions that expired at or before now and returns how many it removed.
func (s *UserStore) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	count, err := s.queries.DeleteExpiredSessions(ctx, dbkit.Time{Time: now})
	if err != nil {
		return 0, failed("delete expired sessions", err)
	}
	return count, nil
}

// DeleteSession removes the session with the token hash, if any.
func (s *UserStore) DeleteSession(ctx context.Context, tokenHash []byte) error {
	return failed("delete session", s.queries.DeleteSession(ctx, tokenHash))
}

// inTx runs body in a write transaction, committing it or answering its failure under action.
func (s *UserStore) inTx(ctx context.Context, action string, body func(*db.Queries) error) error {
	tx, err := s.handle.BeginTx(ctx, nil)
	if err != nil {
		return failed(action, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := body(s.queries.WithTx(tx)); err != nil {
		if isSentinel(err) {
			return err
		}
		return failed(action, err)
	}
	return failed(action, tx.Commit())
}

// disable writes an account's disabled state and ends its sessions and tokens when it closes.
func disable(ctx context.Context, queries *db.Queries, id uuid.UUID, disabled bool) error {
	count, err := queries.SetUserDisabled(ctx, db.SetUserDisabledParams{Disabled: disabled, ID: id})
	if err != nil {
		return err
	}
	if count == 0 {
		return gouncer.ErrUserNotFound
	}
	return revoke(ctx, queries, id, disabled)
}

// revoke deletes the sessions and tokens of an account that is now disabled.
func revoke(ctx context.Context, queries *db.Queries, id uuid.UUID, disabled bool) error {
	if !disabled {
		return nil
	}
	if err := queries.DeleteUserSessions(ctx, id); err != nil {
		return err
	}
	return queries.DeleteUserTokens(ctx, id)
}

// userOrMissing returns the user of row, missing when no row came back, or the failure under action.
func userOrMissing(action string, row db.AuthUser, err, missing error) (gouncer.User, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return gouncer.User{}, missing
	}
	if err != nil {
		return gouncer.User{}, failed(action, err)
	}
	return gouncer.User{
		ID:           row.ID,
		Email:        row.Email,
		Name:         row.Name,
		PasswordHash: row.PasswordHash,
		Disabled:     row.Disabled,
		Confirmed:    row.Confirmed,
		Role:         row.Role,
		CreatedAt:    row.CreatedAt.Time,
	}, nil
}

// failed returns nil for a nil err, or err marked with its dbkit class under action.
func failed(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("sqlite: %s: %w", action, dbsqlite.Classify(err))
}

// isSentinel reports whether err is a gouncer or authkit contract error the caller matches on.
func isSentinel(err error) bool {
	return errors.Is(err, gouncer.ErrTokenExists) ||
		errors.Is(err, gouncer.ErrTokenNotFound) ||
		errors.Is(err, gouncer.ErrUserNotFound) ||
		errors.Is(err, gouncer.ErrLastPrivileged) ||
		errors.Is(err, authkit.ErrRoleChanged)
}
