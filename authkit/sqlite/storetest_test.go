// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/sqlite"
	"github.com/gopherium/gouncer/authkit/storetest"
)

// plantToken stores a token row as it stands.
const plantToken = "INSERT INTO auth_tokens (token_hash, user_id, purpose, created_at, expires_at) " +
	"VALUES (?, ?, ?, ?, ?)"

// plantOn returns a hook that stores a token in db past the store's own checks.
func plantOn(db *sql.DB) func(ctx context.Context, token gouncer.Token) error {
	return func(ctx context.Context, token gouncer.Token) error {
		_, err := db.ExecContext(ctx, plantToken, token.TokenHash, token.UserID, string(token.Purpose),
			token.CreatedAt, token.ExpiresAt)
		return err
	}
}

// heldAccount is an open write transaction that holds the database file for one account.
type heldAccount struct {
	// tx is the transaction that holds the file.
	tx *sql.Tx
}

// Renew deletes the token expired and stores renewed inside the transaction.
func (h heldAccount) Renew(ctx context.Context, expired, renewed gouncer.Token) error {
	if _, err := h.tx.ExecContext(ctx, "DELETE FROM auth_tokens WHERE token_hash = ?", expired.TokenHash); err != nil {
		return err
	}
	_, err := h.tx.ExecContext(ctx, plantToken, renewed.TokenHash, renewed.UserID, string(renewed.Purpose),
		renewed.CreatedAt, renewed.ExpiresAt)
	return err
}

// Commit ends the hold and keeps what it wrote.
func (h heldAccount) Commit(context.Context) error { return h.tx.Commit() }

// Rollback ends the hold and drops what it wrote.
func (h heldAccount) Rollback(context.Context) error { return h.tx.Rollback() }

// holdOn returns a hook that holds an account of db in an open write transaction on a second connection.
func holdOn(db *sql.DB) func(ctx context.Context, id uuid.UUID) (storetest.Held, error) {
	return func(ctx context.Context, id uuid.UUID) (storetest.Held, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		written, err := tx.ExecContext(ctx, "UPDATE auth_users SET disabled = disabled WHERE id = ?", id)
		if err == nil {
			if rows, _ := written.RowsAffected(); rows != 1 {
				err = gouncer.ErrUserNotFound
			}
		}
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		return heldAccount{tx: tx}, nil
	}
}

// freshFixture returns the contract fixture of a store over a fresh copy of the migrated template.
func freshFixture(t *testing.T) storetest.Fixture {
	db := migrated(t)
	return storetest.Fixture{Store: sqlite.NewUserStore(db), Plant: plantOn(db), Hold: holdOn(db)}
}

func TestUserStoreKeepsTheStoreContract(t *testing.T) {
	t.Parallel()

	storetest.Run(t, freshFixture)
}

func TestUserStoreKeepsTheHeldContract(t *testing.T) {
	t.Parallel()

	storetest.RunHeld(t, freshFixture)
}
