# mailrat

A native Gmail desktop client built with Go. Backend-driven architecture: Go owns all state, SQLite is the source of truth, and the UI is a thin rendering layer.

Three interfaces share the same backend:

- **CLI** — headless mail engine for scripting and backend testing
- **TUI** — full-featured terminal UI built with Bubbletea
- **Desktop / Web** — Wails desktop app or browser-accessible HTTP server

---

## Features

- **Multi-account** — manage multiple Gmail accounts from one binary
- **Offline-first** — read cached messages and contacts without a connection
- **Fast** — inbox loads from local SQLite, not the network
- **Unified inbox** — threads across all accounts sorted by date
- **Full Gmail label support** — system and user-defined labels
- **Contacts** — sync, search, create, edit, and delete Google contacts
- **No IMAP** — uses the Gmail API directly for reliable sync via `historyId`

---

## Requirements

- Go 1.21 or later
- A Google Cloud OAuth 2.0 credentials file (`credentials.json`) with the scopes:
  - `gmail.readonly`, `gmail.modify`, `gmail.send`
  - `contacts` (for the People API)

Place `credentials.json` in `~/.mailrat/credentials.json` before running any command.

---

## Installation

```bash
git clone https://github.com/mailrat/mailrat.git
cd mailrat
go build ./...
```

---

## Quickstart

```bash
# 1. Add a Gmail account (opens browser OAuth flow)
go run ./cmd/cli accounts add

# 2. Sync the last 30 days
go run ./cmd/cli sync --days=30

# 3. Pick an interface
go run ./cmd/cli tui          # terminal UI
go run ./cmd/webserver        # open http://localhost:8080 in a browser
```

---

## Interfaces

### CLI

```bash
go run ./cmd/cli <command>
```

| Command | Description |
|---|---|
| `accounts add` | OAuth flow to add a Gmail account |
| `accounts list` | List connected accounts |
| `accounts remove <id\|email>` | Remove an account and its local data |
| `sync [--days=N] [--account=id]` | Sync mail (initial or incremental) |
| `inbox list [--label=L] [--limit=N] [--account=id]` | Print unified inbox |
| `message read <id> [--account=id]` | Print a message body |
| `send [--to=] [--subject=] [--body=] [--account=id]` | Send an email |
| `labels list [--account=id]` | List Gmail labels |
| `contacts list [--sync] [--search=q]` | List / search contacts |
| `contacts add [--name=] [--email=]` | Add a contact |
| `contacts edit <id>` | Edit a contact |
| `contacts delete <id> [--force]` | Delete a contact |
| `tui` | Launch the terminal UI |

---

### TUI

```bash
go run ./cmd/cli tui
```

Full-screen terminal UI powered by [Bubbletea](https://github.com/charmbracelet/bubbletea).

#### Navigation

| Key | Action |
|---|---|
| `j` / `k` or arrows | Move cursor |
| `Enter` / `o` | Open thread / message |
| `u` / `Esc` | Back |

#### Actions

| Key | Action |
|---|---|
| `c` | Compose new message |
| `r` | Reply |
| `a` | Reply all |
| `f` | Forward |
| `e` / `y` | Archive |
| `I` | Mark unread |
| `s` | Sync now |
| `/` | Search |
| `q` | Quit |

#### Compose

| Key | Action |
|---|---|
| `Tab` | Next field |
| `Ctrl+S` / `Ctrl+Enter` | Send |
| `Esc` | Discard |

---

### Desktop app (Wails)

Requires [Wails v2](https://wails.io) installed (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```bash
wails build        # production binary
wails dev          # live-reload dev mode
```

The frontend is an Alpine.js + Tailwind CSS SPA embedded into the binary at build time. No npm or frontend build step required — CDN-loaded.

---

### Web server (remote browser access)

Serves the same UI and backend over plain HTTP — useful for testing on a remote machine or from a phone/tablet.

```bash
# Run from the project root
go run ./cmd/webserver

# Custom address
go run ./cmd/webserver -addr 0.0.0.0:9090

# All flags
go run ./cmd/webserver -addr 0.0.0.0:8080 -static cmd/desktop/frontend
```

| Flag | Default | Description |
|---|---|---|
| `-addr` | `0.0.0.0:8080` | Listen address (`host:port`) |
| `-static` | `cmd/desktop/frontend` | Path to frontend files (relative to cwd) |

Open `http://<host>:<port>` in any browser.

#### REST API

The web server exposes the backend as a JSON API used by the browser frontend:

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/accounts` | Connected accounts |
| `GET` | `/api/inbox?accountId=&limit=` | Inbox thread summaries |
| `GET` | `/api/search?q=&limit=` | Search threads |
| `GET` | `/api/thread?accountId=&threadId=` | Messages in a thread |
| `GET` | `/api/message?accountId=&messageId=` | Single message (lazy body fetch) |
| `POST` | `/api/send` | Send a message (JSON body) |
| `POST` | `/api/sync?accountId=` | Run incremental sync |
| `GET` | `/api/labels?accountId=` | Gmail labels |
| `GET` | `/api/events` | SSE stream — push `sync:progress`, `sync:complete`, `inbox:updated` |

The same `index.html` works inside the Wails WebView and in a plain browser. A small shim (`js/wails-compat.js`) maps `window.go.desktop.App.*` to `fetch()` calls when running outside Wails, and subscribes to the SSE stream for push events.

---

## Data directory

All data lives in `~/.mailrat/`:

| File | Contents |
|---|---|
| `credentials.json` | Google OAuth client credentials (you supply this) |
| `mailrat.db` | SQLite database — all mail, contacts, and tokens |

The database uses WAL mode and stores:

| Table | Contents |
|---|---|
| `accounts` | Account IDs, emails, OAuth tokens, sync `historyId` |
| `messages` | Headers, snippet, body (when cached), cache state |
| `threads` | Thread IDs and last-message date |
| `labels` | Gmail label definitions |
| `message_labels` | Many-to-many message ↔ label mapping |
| `drafts` | Draft content |
| `contacts` | Synced Google contacts with resource name and etag |

Inspect the database directly:

```bash
sqlite3 ~/.mailrat/mailrat.db ".tables"
sqlite3 ~/.mailrat/mailrat.db "SELECT email FROM accounts"
sqlite3 ~/.mailrat/mailrat.db \
  "SELECT subject, from_addr, datetime(internal_date/1000,'unixepoch')
   FROM messages ORDER BY internal_date DESC LIMIT 10"
```

---

## Architecture

```
Gmail API ↔ Sync Engine ↔ SQLite ↔ Go Services ↔ UI (CLI / TUI / Wails / HTTP)
```

### Design principles

- **Go backend owns all state** — no business logic in the UI layer
- **SQLite is the single source of truth** — queries run locally; offline reads are instant
- **Gmail API as sync layer only** — initial sync via date range, incremental sync via `historyId`
- **Lazy body fetch** — metadata is synced first; full bodies are fetched on demand

### Package layout

```
cmd/
  cli/              CLI entry point (also launches TUI)
  desktop/          Wails desktop entry point
    frontend/       Alpine.js + Tailwind SPA (embedded into binary at build time)
  webserver/        HTTP server for remote browser access

internal/
  accounts/         OAuth2 flow and token management
  cli/              Cobra commands
  contacts/         Google People API integration
  desktop/          Wails-bound App struct and frontend-facing types
  gmail/            Gmail API client wrapper with retry / backoff
  mail/             Business logic — inbox, send, labels, drafts
  storage/          SQLite schema and CRUD
  sync/             Initial and incremental sync engine
  tui/              Bubbletea models and views
```

---

## Development

```bash
go test ./...    # run tests
go build ./...   # build all binaries
go vet ./...     # vet
```

---

## License

MIT
