-- SPDX-License-Identifier: Apache-2.0

-- +goose Up
CREATE TABLE auth_users (
    id TEXT NOT NULL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    disabled BOOLEAN NOT NULL CHECK (disabled IN (0, 1)),
    created_at INTEGER NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    confirmed BOOLEAN NOT NULL DEFAULT 1 CHECK (confirmed IN (0, 1))
);

CREATE INDEX auth_users_role_idx ON auth_users (role) WHERE role <> '';

CREATE TABLE auth_sessions (
    token_hash BLOB NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES auth_users (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX auth_sessions_user_id_idx ON auth_sessions (user_id);

CREATE INDEX auth_sessions_expires_at_idx ON auth_sessions (expires_at);

CREATE TABLE auth_tokens (
    token_hash BLOB NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES auth_users (id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX auth_tokens_user_id_purpose_idx ON auth_tokens (user_id, purpose);

CREATE INDEX auth_tokens_expires_at_idx ON auth_tokens (expires_at);

-- +goose Down
DROP TABLE auth_tokens;

DROP TABLE auth_sessions;

DROP TABLE auth_users;
