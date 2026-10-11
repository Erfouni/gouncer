// SPDX-License-Identifier: Apache-2.0

package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

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

// freshFixture returns the contract fixture of a store over a fresh copy of the migrated template.
func freshFixture(t *testing.T) storetest.Fixture {
	db := migrated(t)
	return storetest.Fixture{Store: sqlite.NewUserStore(db), Plant: plantOn(db)}
}

func TestUserStoreKeepsTheStoreContract(t *testing.T) {
	t.Parallel()

	storetest.Run(t, freshFixture)
}
