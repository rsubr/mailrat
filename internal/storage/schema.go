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
    PRIMARY KEY (id, account_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS contacts (
    id         TEXT NOT NULL,
    account_id TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    email      TEXT NOT NULL,
    PRIMARY KEY (id, account_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_messages_account_date
    ON messages(account_id, internal_date DESC);

CREATE INDEX IF NOT EXISTS idx_message_labels_label
    ON message_labels(label_id, message_id);

CREATE INDEX IF NOT EXISTS idx_contacts_email
    ON contacts(account_id, email);
`
