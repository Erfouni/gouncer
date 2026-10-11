// SPDX-License-Identifier: Apache-2.0

// Package sqlite implements gouncer's user and session persistence in SQLite tables of its own.
package sqlite

import "embed"

// migrations holds the auth table migration files applied by goose.
//
//go:embed migrations/*.sql
var migrations embed.FS
