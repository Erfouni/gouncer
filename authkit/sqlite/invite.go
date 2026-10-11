// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopherium/framework/dbkit"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/sqlite/internal/db"
)

// CreateToken stores t for an enabled account while fewer than live unexpired tokens stand for its purpose.
func (s *UserStore) CreateToken(ctx context.Context, t gouncer.Token, live int) error {
	return s.inTx(ctx, "create token", func(queries *db.Queries) error {
		if err := holdEnabled(ctx, queries, t.UserID); err != nil {
			return err
		}
		standing, err := queries.CountLiveTokens(ctx, db.CountLiveTokensParams{
			UserID:    t.UserID,
			Purpose:   string(t.Purpose),
			ExpiresAt: dbkit.Time{Time: time.Now().UTC()},
		})
		if err != nil {
			return err
		}
		if standing >= int64(live) {
			return gouncer.ErrTokenExists
		}
		return insertToken(ctx, queries, t)
	})
}

// ReplaceToken stores t for an enabled account in place of every token it holds for the same purpose.
func (s *UserStore) ReplaceToken(ctx context.Context, t gouncer.Token) error {
	return s.inTx(ctx, "replace token", func(queries *db.Queries) error {
		if err := holdReplaceable(ctx, queries, t.UserID, t.Purpose); err != nil {
			return err
		}
		err := queries.DeleteUserTokensForPurpose(ctx, db.DeleteUserTokensForPurposeParams{
			UserID:  t.UserID,
			Purpose: string(t.Purpose),
		})
		if err != nil {
			return err
		}
		return insertToken(ctx, queries, t)
	})
}

// ActivateByToken spends the invite token, stores the password hash and confirms the account it names.
func (s *UserStore) ActivateByToken(
	ctx context.Context,
	tokenHash []byte,
	now time.Time,
	passwordHash string,
) (uuid.UUID, error) {
	return s.redeem(ctx, "activate by token", tokenHash, gouncer.PurposeInvite, now,
		func(queries *db.Queries, id uuid.UUID) (int64, error) {
			return queries.ActivateUser(ctx, db.ActivateUserParams{PasswordHash: passwordHash, ID: id})
		})
}

// ResetByToken spends the reset token, stores the password hash and ends every session of the account it names.
func (s *UserStore) ResetByToken(
	ctx context.Context,
	tokenHash []byte,
	now time.Time,
	passwordHash string,
) (uuid.UUID, error) {
	return s.redeem(ctx, "reset by token", tokenHash, gouncer.PurposeReset, now,
		func(queries *db.Queries, id uuid.UUID) (int64, error) {
			count, err := queries.SetUserPassword(ctx, db.SetUserPasswordParams{PasswordHash: passwordHash, ID: id})
			if err != nil || count == 0 {
				return count, err
			}
			if err := queries.DeleteUserSessions(ctx, id); err != nil {
				return count, err
			}
			return count, queries.DeleteUserTokensForPurpose(ctx, db.DeleteUserTokensForPurposeParams{
				UserID:  id,
				Purpose: string(gouncer.PurposeReset),
			})
		})
}

// DeleteExpiredTokens removes the expired tokens and the unconfirmed accounts an expired invite leaves behind.
func (s *UserStore) DeleteExpiredTokens(ctx context.Context, now time.Time) (int64, error) {
	var swept int64
	err := s.inTx(ctx, "delete expired tokens", func(queries *db.Queries) error {
		expired, err := queries.ExpiredTokens(ctx, dbkit.Time{Time: now})
		if err != nil {
			return err
		}
		swept = int64(len(expired))
		stranded := make([]uuid.UUID, 0, len(expired))
		for _, row := range expired {
			if row.Purpose == string(gouncer.PurposeInvite) {
				stranded = append(stranded, row.UserID)
			}
		}
		if err := sweepStranded(ctx, queries, stranded, now); err != nil {
			return err
		}
		_, err = queries.DeleteExpiredTokens(ctx, dbkit.Time{Time: now})
		return err
	})
	if err != nil {
		return 0, err
	}
	return swept, nil
}

// redeem spends the live token behind the hash and purpose and applies write to the account it names.
func (s *UserStore) redeem(
	ctx context.Context,
	action string,
	tokenHash []byte,
	purpose gouncer.TokenPurpose,
	now time.Time,
	write func(*db.Queries, uuid.UUID) (int64, error),
) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.inTx(ctx, action, func(queries *db.Queries) error {
		held, err := queries.FindLiveToken(ctx, db.FindLiveTokenParams{
			TokenHash: tokenHash,
			Purpose:   string(purpose),
			ExpiresAt: dbkit.Time{Time: now},
		})
		if errors.Is(err, sql.ErrNoRows) {
			return gouncer.ErrTokenNotFound
		}
		if err != nil {
			return err
		}
		if err := holdEnabled(ctx, queries, held); err != nil {
			return err
		}
		if _, err := queries.DeleteToken(ctx, tokenHash); err != nil {
			return err
		}
		count, err := write(queries, held)
		if err != nil {
			return err
		}
		if count == 0 {
			return gouncer.ErrUserNotFound
		}
		id = held
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// sweepStranded deletes the unconfirmed accounts among stranded that hold no live invite.
func sweepStranded(ctx context.Context, queries *db.Queries, stranded []uuid.UUID, now time.Time) error {
	if len(stranded) == 0 {
		return nil
	}
	held, err := queries.LockUnconfirmedAccounts(ctx, jsonList(stranded))
	if err != nil || len(held) == 0 {
		return err
	}
	return queries.DeleteUnconfirmedAccounts(ctx, db.DeleteUnconfirmedAccountsParams{
		IDs:       jsonList(held),
		Purpose:   string(gouncer.PurposeInvite),
		ExpiresAt: dbkit.Time{Time: now},
	})
}

// holdReplaceable checks the account a replacement token may stand for, an unactivated one for an invite.
func holdReplaceable(ctx context.Context, queries *db.Queries, id uuid.UUID, purpose gouncer.TokenPurpose) error {
	if purpose != gouncer.PurposeInvite {
		return holdEnabled(ctx, queries, id)
	}
	_, err := queries.LockUnactivatedUser(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return gouncer.ErrUserNotFound
	}
	return err
}

// holdEnabled checks that the account is enabled, or answers gouncer.ErrUserNotFound.
func holdEnabled(ctx context.Context, queries *db.Queries, id uuid.UUID) error {
	_, err := queries.LockEnabledUser(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return gouncer.ErrUserNotFound
	}
	return err
}

// insertToken stores t for the account the transaction already checked.
func insertToken(ctx context.Context, queries *db.Queries, t gouncer.Token) error {
	return queries.CreateToken(ctx, db.CreateTokenParams{
		TokenHash: t.TokenHash,
		UserID:    t.UserID,
		Purpose:   string(t.Purpose),
		CreatedAt: dbkit.Time{Time: t.CreatedAt},
		ExpiresAt: dbkit.Time{Time: t.ExpiresAt},
	})
}
