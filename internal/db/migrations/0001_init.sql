-- 0001_init.sql — initial schema for Mederos & Associates CRM
-- Applied automatically on startup by the embedded migration runner.

PRAGMA foreign_keys = ON;

-- Staff accounts. Roles are scaffolded now (admin/staff) so we can add
-- per-role permissions later without a schema change.
CREATE TABLE users (
  id             INTEGER PRIMARY KEY,
  username       TEXT    NOT NULL UNIQUE COLLATE NOCASE,
  password_hash  TEXT    NOT NULL,
  role           TEXT    NOT NULL DEFAULT 'staff' CHECK (role IN ('admin','staff')),
  display_name   TEXT    NOT NULL DEFAULT '',
  is_active      INTEGER NOT NULL DEFAULT 1,
  must_change_pw INTEGER NOT NULL DEFAULT 0,
  created_at     TEXT    NOT NULL DEFAULT (datetime('now')),
  updated_at     TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Server-side sessions. The cookie holds only the opaque random id, so we can
-- revoke any session (or all of a user's sessions) instantly.
CREATE TABLE sessions (
  id         TEXT    PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT    NOT NULL DEFAULT (datetime('now')),
  expires_at TEXT    NOT NULL,
  ip         TEXT    NOT NULL DEFAULT '',
  user_agent TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- Clients — the core of the CRM. folder_path is RELATIVE to the configured
-- case-files root; deleted_at is a soft delete (we never hard-delete legal
-- records in v1).
CREATE TABLE clients (
  id          INTEGER PRIMARY KEY,
  name        TEXT    NOT NULL,
  address     TEXT    NOT NULL DEFAULT '',
  phone       TEXT    NOT NULL DEFAULT '',
  email       TEXT    NOT NULL DEFAULT '',
  birthday    TEXT    NOT NULL DEFAULT '',   -- ISO 'YYYY-MM-DD' as text
  case_type   TEXT    NOT NULL DEFAULT '',
  adversary   TEXT    NOT NULL DEFAULT '',
  notes       TEXT    NOT NULL DEFAULT '',
  folder_path TEXT    NOT NULL DEFAULT '',   -- relative to case-files root
  created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
  updated_at  TEXT    NOT NULL DEFAULT (datetime('now')),
  created_by  INTEGER REFERENCES users(id),
  deleted_at  TEXT
);
CREATE INDEX idx_clients_name    ON clients(name COLLATE NOCASE);
CREATE INDEX idx_clients_deleted ON clients(deleted_at);

-- Audit log. Written from day one — cheap insurance for a law firm.
CREATE TABLE audit_events (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER REFERENCES users(id),
  action      TEXT    NOT NULL,
  entity_type TEXT    NOT NULL DEFAULT '',
  entity_id   TEXT    NOT NULL DEFAULT '',
  detail      TEXT    NOT NULL DEFAULT '',
  ip          TEXT    NOT NULL DEFAULT '',
  created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_audit_created ON audit_events(created_at);
CREATE INDEX idx_audit_entity  ON audit_events(entity_type, entity_id);
