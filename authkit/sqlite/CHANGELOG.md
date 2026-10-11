# Changelog

All notable changes to the `authkit/sqlite` module are documented in
this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module
follows [Semantic Versioning](https://semver.org/). While at v0.x, minor
releases may contain breaking changes.

Releases of this module are tagged `authkit/sqlite/vX.Y.Z`.

## [Unreleased]

### Added

- `UserStore` implementing `gouncer.Store` plus the `authkit.AdminStore`,
  `authkit.InviteStore`, `authkit.SessionReaper` and
  `authkit.TokenReaper` contracts on the application's one handle from
  `dbkit/sqlite`. Requires authkit v0.19.0.
- `Migrate` with `MigrateOptions`, which runs the auth migrations under
  dbkit's file lock and records them in `auth_goose_db_version`.
- One baseline migration, `00004`, for `auth_users`, `auth_sessions` and
  `auth_tokens`, with ids as UUID text and times as UTC microseconds.
- A busy, locked or constraint failure carries dbkit's error class, such
  as `dbkit.ErrBusy`. The PostgreSQL store does not class its failures.
