-- SPDX-License-Identifier: Apache-2.0

-- name: CreateUser :exec
INSERT INTO auth_users (id, email, name, password_hash, disabled, created_at, role, confirmed)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetUserByEmail :one
SELECT id, email, name, password_hash, disabled, created_at, role, confirmed
FROM auth_users
WHERE email = ?;

-- name: GetUserByID :one
SELECT id, email, name, password_hash, disabled, created_at, role, confirmed
FROM auth_users
WHERE id = ?;

-- name: ListUsers :many
SELECT id, email, name, disabled, confirmed, created_at, role
FROM auth_users
ORDER BY name COLLATE NOCASE, id;

-- name: ListUsersByIDs :many
SELECT id, email, name, disabled, confirmed, created_at, role
FROM auth_users
WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: SetUserDisabled :execrows
UPDATE auth_users
SET disabled = ?
WHERE id = ?;

-- name: DeleteUserSessions :exec
DELETE FROM auth_sessions
WHERE user_id = ?;

-- name: CreateSession :exec
INSERT INTO auth_sessions (token_hash, user_id, created_at, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetUserBySession :one
SELECT u.id, u.email, u.name, u.password_hash, u.disabled, u.created_at, u.role, u.confirmed
FROM auth_sessions s
JOIN auth_users u ON u.id = s.user_id
WHERE s.token_hash = ? AND s.expires_at > ? AND NOT u.disabled;

-- name: DeleteSession :exec
DELETE FROM auth_sessions
WHERE token_hash = ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM auth_sessions
WHERE expires_at <= ?;

-- name: SetUserRole :execrows
UPDATE auth_users
SET role = ?
WHERE id = ?;

-- name: GrantRoleToRoleless :execrows
UPDATE auth_users
SET role = ?
WHERE role = '';

-- name: LockEnabledUser :one
SELECT id
FROM auth_users
WHERE id = ? AND NOT disabled;

-- name: LockUnactivatedUser :one
SELECT id
FROM auth_users
WHERE id = ? AND NOT disabled AND NOT confirmed;

-- name: CreateToken :exec
INSERT INTO auth_tokens (token_hash, user_id, purpose, created_at, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: CountLiveTokens :one
SELECT count(*)
FROM auth_tokens
WHERE user_id = ? AND purpose = ? AND expires_at > ?;

-- name: DeleteUserTokensForPurpose :exec
DELETE FROM auth_tokens
WHERE user_id = ? AND purpose = ?;

-- name: DeleteUserTokens :exec
DELETE FROM auth_tokens
WHERE user_id = ?;

-- name: FindLiveToken :one
SELECT user_id
FROM auth_tokens
WHERE token_hash = ? AND purpose = ? AND expires_at > ?;

-- name: ActivateUser :execrows
UPDATE auth_users
SET password_hash = ?, confirmed = TRUE
WHERE id = ? AND NOT disabled AND NOT confirmed;

-- name: SetUserPassword :execrows
UPDATE auth_users
SET password_hash = ?
WHERE id = ? AND NOT disabled;

-- name: DeleteToken :execrows
DELETE FROM auth_tokens
WHERE token_hash = ?;

-- name: ExpiredTokens :many
SELECT user_id, purpose
FROM auth_tokens
WHERE expires_at <= ?;

-- name: DeleteExpiredTokens :execrows
DELETE FROM auth_tokens
WHERE expires_at <= ?;

-- name: LockUnconfirmedAccounts :many
SELECT id
FROM auth_users
WHERE NOT confirmed AND id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY id;

-- name: DeleteUnconfirmedAccounts :exec
DELETE FROM auth_users
WHERE NOT confirmed
  AND id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
  AND NOT EXISTS (
    SELECT 1
    FROM auth_tokens t
    WHERE t.user_id = auth_users.id AND t.purpose = sqlc.arg(purpose) AND t.expires_at > sqlc.arg(expires_at)
  );

-- name: CountCover :one
SELECT count(*)
FROM auth_users
WHERE role IN (SELECT value FROM json_each(CAST(sqlc.arg(roles) AS TEXT))) AND NOT disabled AND id <> sqlc.arg(id);

-- name: HoldsPrivilege :one
SELECT count(*)
FROM auth_users
WHERE id = sqlc.arg(id) AND role IN (SELECT value FROM json_each(CAST(sqlc.arg(roles) AS TEXT))) AND NOT disabled;
