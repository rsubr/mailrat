package storage

const schema = `
CREATE TABLE IF NOT EXISTS accounts (
    id         TEXT PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE,
    token_json TEXT NOT NULL,
    history_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS threads (
    id                TEXT NOT NULL,
    account_id        TEXT NOT NULL,
    last_message_date INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (id, account_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS messages (
    id           TEXT NOT NULL,
    thread_id    TEXT NOT NULL,
    account_id   TEXT NOT NULL,
    subject      TEXT NOT NULL DEFAULT '',
    from_addr    TEXT NOT NULL DEFAULT '',
    to_addr      TEXT NOT NULL DEFAULT '',
    snippet      TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL DEFAULT '',
    internal_date INTEGER NOT NULL DEFAULT 0,
    is_unread    INTEGER NOT NULL DEFAULT 1,
    cache_state  TEXT NOT NULL DEFAULT 'metadata_only',
    has_attachments INTEGER NOT NULL DEFAULT 0,
    message_id   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (id, account_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS labels (
    id         TEXT NOT NULL,
    account_id TEXT NOT NULL,
    name       TEXT NOT NULL,
    label_type TEXT NOT NULL DEFAULT 'user',
    PRIMARY KEY (id, account_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS message_labels (
    message_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    label_id   TEXT NOT NULL,
    PRIMARY KEY (message_id, account_id, label_id),
    FOREIGN KEY (message_id, account_id) REFERENCES messages(id, account_id) ON DELETE CASCADE,
    FOREIGN KEY (label_id, account_id) REFERENCES labels(id, account_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS drafts (
    id         TEXT NOT NULL,
    account_id TEXT NOT NULL,
    content    TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT 0,
    remote_id  TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (id, account_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS attachments (
    message_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    mime_type  TEXT NOT NULL DEFAULT '',
    size       INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (message_id, account_id) REFERENCES messages(id, account_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS pending_ops (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    op_type    TEXT NOT NULL,
    account_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT 0,
    attempts   INTEGER NOT NULL DEFAULT 0,
    UNIQUE (op_type, account_id, message_id)
);

CREATE TABLE IF NOT EXISTS contacts (
    id            TEXT NOT NULL,
    account_id    TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    email         TEXT NOT NULL,
    resource_name TEXT NOT NULL DEFAULT '',
    etag          TEXT NOT NULL DEFAULT '',
    phone         TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (id, account_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_messages_account_date
    ON messages(account_id, internal_date DESC);

CREATE INDEX IF NOT EXISTS idx_message_labels_label
    ON message_labels(label_id, message_id);

CREATE INDEX IF NOT EXISTS idx_contacts_email
    ON contacts(account_id, email);

CREATE TABLE IF NOT EXISTS sync_config (
    label_id          TEXT NOT NULL,
    account_id        TEXT NOT NULL,
    label_name        TEXT NOT NULL DEFAULT '',
    days_to_sync      INTEGER NOT NULL DEFAULT 30,
    backfilled        INTEGER NOT NULL DEFAULT 0,
    max_emails        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (label_id, account_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS account_settings (
    account_id        TEXT PRIMARY KEY,
    sync_period       TEXT NOT NULL DEFAULT '30d',
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS send_as (
    account_id     TEXT NOT NULL,
    email          TEXT NOT NULL,
    display_name   TEXT NOT NULL DEFAULT '',
    signature_html TEXT NOT NULL DEFAULT '',
    is_primary     INTEGER NOT NULL DEFAULT 0,
    is_default     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, email),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS app_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
`
