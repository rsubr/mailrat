# mailrat

A fast, offline-capable Gmail client for the terminal. Built with Go, backed by SQLite.

mailrat syncs your Gmail directly via the Gmail API — not IMAP — and stores everything locally so your inbox is always available, even without a connection. All state lives in Go and SQLite; the CLI is a thin layer on top.

## Features

- **Multi-account** — manage multiple Gmail accounts from one tool
- **Offline-first** — read cached messages and contacts without a connection
- **Fast** — inbox loads from local SQLite, not the network
- **Unified inbox** — view messages across all accounts sorted by date
- **Full Gmail label support** — system and user-defined labels
- **Contacts** — sync, search, create, edit, and delete Google contacts
- **No IMAP** — uses the Gmail API directly for reliable sync via `historyId`

## Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Commands](#commands)
  - [accounts](#accounts)
  - [sync](#sync)
  - [inbox](#inbox)
  - [message](#message)
  - [send](#send)
  - [labels](#labels)
  - [contacts](#contacts)
- [Data Storage](#data-storage)
- [Architecture](#architecture)
- [Roadmap](#roadmap)

## Requirements

- Go 1.21 or later
- A Google account
- A Google Cloud project with the Gmail API and People API enabled (the `accounts add` command walks you through this)

## Installation

### Build from source

```bash
git clone https://github.com/mailrat/mailrat.git
cd mailrat
./build.sh --release
```

This produces a static binary at `./mailrat`. Copy it somewhere on your `$PATH`:

```bash
cp mailrat /usr/local/bin/
```

## Getting Started

### 1. Add your Gmail account

```bash
mailrat accounts add
```

This launches a step-by-step wizard that:
- Guides you through enabling the Gmail API and People API on Google Cloud
- Walks you through creating OAuth2 credentials (Desktop app type)
- Opens a browser for the OAuth consent flow — or, if you are on a headless/remote machine, prints a URL you can open on any device and paste the redirect URL back

### 2. Sync your mail

```bash
mailrat sync --days=30
```

### 3. Read your inbox

```bash
mailrat inbox list
```

---

## Commands

### accounts

Manage Gmail accounts.

```bash
# Add a new account (guided setup wizard)
mailrat accounts add

# List all authenticated accounts
mailrat accounts list

# Remove an account and delete all its local data
mailrat accounts remove <account-id|email>
mailrat accounts remove you@gmail.com --force   # skip confirmation
```

> **Note:** Removing an account permanently deletes all locally cached emails, threads, labels, drafts, and contacts for that account.

---

### sync

Sync mail from Gmail. On first run per account, performs a full sync. On subsequent runs, uses Gmail's `historyId` for fast incremental updates.

```bash
# Sync all accounts (auto-detects initial vs incremental)
mailrat sync

# Sync the last N days on initial sync (default: 30)
mailrat sync --days=7
mailrat sync --days=90

# Sync a specific account only
mailrat sync --account <account-id>
```

---

### inbox

Browse your inbox.

```bash
# Unified inbox across all accounts, newest first
mailrat inbox list

# Limit number of results (default: 50)
mailrat inbox list --limit 20

# Filter to a single account
mailrat inbox list --account <account-id>
```

Output columns: unread indicator (`*`), date, from, subject, snippet.

---

### message

Read individual messages.

```bash
# Read a message by ID (fetches body from Gmail if not cached)
mailrat message read <message-id>

# Specify account if the ID is ambiguous
mailrat message read <message-id> --account <account-id>
```

Message IDs are shown in `inbox list` output.

---

### send

Compose and send an email.

```bash
# Interactive prompts for To / Subject / Body
mailrat send

# Pass fields as flags
mailrat send --to recipient@example.com --subject "Hello" --body "Hi there"

# Send from a specific account (default: first account)
mailrat send --account <account-id> --to recipient@example.com
```

When using interactive mode, end the message body with a line containing only `.`.

---

### labels

Manage Gmail labels.

```bash
# List all labels for the default account
mailrat labels list

# List labels for a specific account
mailrat labels list --account <account-id>
```

---

### contacts

Sync and manage Google contacts.

```bash
# List locally cached contacts
mailrat contacts list

# Sync from Google, then list
mailrat contacts list --sync

# Search by name or email
mailrat contacts list --search "alice"

# Add a new contact
mailrat contacts add
mailrat contacts add --name "Alice Smith" --email alice@example.com

# Edit a contact (prompts with current values as defaults)
mailrat contacts edit <contact-id>
mailrat contacts edit <contact-id> --name "Alice B. Smith"
mailrat contacts edit <contact-id> --email newemail@example.com

# Delete a contact (prompts for confirmation)
mailrat contacts delete <contact-id>
mailrat contacts delete <contact-id> --force   # skip confirmation
```

Contact IDs are shown in `contacts list` output.

---

## Data Storage

All data is stored under `~/.mailrat/`:

```
~/.mailrat/
├── credentials.json    # your Google OAuth client credentials (you provide this)
└── mailrat.db          # SQLite database — all mail and contact data
```

The SQLite database uses WAL mode and stores:

| Table | Contents |
|---|---|
| `accounts` | account IDs, emails, OAuth tokens, sync historyId |
| `messages` | headers, snippet, body (when cached), cache state |
| `threads` | thread IDs and last message date |
| `labels` | Gmail label definitions |
| `message_labels` | many-to-many message ↔ label mapping |
| `drafts` | draft content |
| `contacts` | synced Google contacts with resource name and etag |

You can inspect the database directly:

```bash
sqlite3 ~/.mailrat/mailrat.db ".tables"
sqlite3 ~/.mailrat/mailrat.db "SELECT email FROM accounts"
sqlite3 ~/.mailrat/mailrat.db \
  "SELECT subject, from_addr, datetime(internal_date/1000,'unixepoch') FROM messages LIMIT 10"
```

> **Note:** `credentials.json` and `mailrat.db` are excluded from version control via `.gitignore`. Never commit them.

---

## Architecture

```
Gmail API ──► Sync Engine ──► SQLite ──► Go Services ──► CLI
```

- **Go backend owns all state** — no business logic in the UI layer
- **SQLite is the single source of truth** — queries run locally, offline reads are instant
- **Gmail API as sync layer only** — initial sync via date range, incremental sync via `historyId`
- **Lazy body fetch** — message metadata is synced first; full bodies are fetched on demand

### Package layout

```
cmd/cli/                  entry point
internal/
  accounts/               OAuth2 flow and token management
  gmail/                  Gmail API wrapper with retry/backoff
  sync/                   initial and incremental sync engine
  storage/                SQLite schema and CRUD
  mail/                   inbox, send, label, and draft services
  contacts/               Google People API wrapper
  cli/                    Cobra commands
```

---

## Roadmap

- [ ] Attachment download and caching
- [ ] TUI (Bubbletea) — Phase 2
- [ ] Desktop app (Wails) — Phase 3
- [ ] Reply and forward commands
- [ ] Schedule send (local queue)
- [ ] Push notifications via Gmail API watch

---

## License

MIT
