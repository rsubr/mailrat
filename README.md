# mailrat

A native Gmail desktop client built with Go. Backend-driven architecture: Go owns all state, SQLite is the source of truth, and the UI is a thin rendering layer.

All interfaces ship as a single binary:

| Invocation | Interface |
|---|---|
| `mailrat` | Native desktop app (Fyne) |
| `mailrat tui` | Terminal UI (Bubbletea) |
| `mailrat <command>` | CLI (accounts, sync, inbox, send, …) |

---

## Features

- **Multi-account** — manage multiple Gmail accounts from one binary
- **Offline-first** — read cached messages and contacts without a connection
- **Fast** — inbox loads from local SQLite, not the network
- **Unified inbox** — threads across all accounts sorted by date
- **Full Gmail label support** — system and user-defined labels
- **Contacts** — sync, search, create, edit, and delete Google contacts
- **No IMAP** — uses the Gmail API directly for reliable sync via `historyId`
- **Rich text compose** — Gmail-style editor with bold, italic, underline, strikethrough, bullet/numbered lists, and hyperlinks; HTML emails sent as `multipart/alternative` with plain-text fallback
- **Reply/forward with quoted original** — Gmail-style attribution (`On Mon, Jan 2, 2026 at 3:04 PM, sender wrote:`) with blockquote
- **Reply threading** — RFC 2822 `In-Reply-To` and `References` headers so replies thread correctly in Gmail
- **Inline compose** — reply and forward open at the bottom of the thread (new mail takes over the reading pane), no modal
- **Auto mark-as-read** — threads are marked read automatically after 5 seconds of viewing
- **Trash and mark-as-unread** — one-click delete (moves to Trash) and mark-as-unread from the toolbar

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
go build -o mailrat ./cmd/mailrat
```

Or run directly without installing:

```bash
go run ./cmd/mailrat <command>
```

---

## Quickstart

```bash
# 1. Add a Gmail account (opens browser OAuth flow)
mailrat accounts add

# 2. Sync the last 30 days
mailrat sync --days=30

# 3. Pick an interface — or run without args for the desktop app
mailrat tui          # terminal UI
mailrat              # native desktop app (Fyne)
```

---

## Interfaces

### Desktop app (default)

```bash
mailrat
```

Launches the native Fyne app, laid out after Mimestream: account tiles and mailboxes on the left, thread list in the middle, conversation and toolbar on the right. Follows the system light/dark appearance.

Keyboard shortcuts follow Gmail on the web. Press `?` in the app for the list.

| Keys | Action |
|---|---|
| `g` then `i` `s` `t` `d` `a` `l` | Go to Inbox, Starred, Sent, Drafts, All Mail, or a label |
| `j` / `k` | Open next (older) / previous (newer) conversation |
| `n` / `p` | Next / previous message in the conversation |
| `o` or `Enter` | Open conversation; expand or collapse the current message |
| `u` | Back to the conversation list |
| `c` or `d` | Compose |
| `/` | Search (`Esc` leaves the field) |
| `e` or `y` | Archive / remove from view |
| `#` / `!` | Trash / report spam |
| `s` | Star or unstar |
| `+` `=` / `-` | Mark important / not important |
| `r` `a` `f` (or capitals) | Reply, reply all, forward |
| `⇧I` / `⇧U` or `_` | Mark read / unread |
| `⇧N` | Check for new mail |
| `⌘Enter` | Send (in a message window) |
| `⌘N` `⌘R` `⇧⌘R` `⇧⌘F` `⌘⌫` `⌘,` `⌘G` | New, reply, reply all, forward, trash, settings, check mail |

Not implemented: multi-select (`x`, `*` sequences), mute (`m`), move (`v`), label (`l`), undo (`z`).

#### Building

Fyne needs cgo, so build natively on the target OS (there is no Linux→macOS cross-compile without an Apple SDK):

```bash
./build.sh        # ./mailrat binary
./build.sh app    # ./mailrat.app bundle (macOS)
```

**Prerequisites (macOS, Apple silicon):**

1. Xcode Command Line Tools — `xcode-select --install` (provides clang and the Cocoa/OpenGL headers Fyne needs).
2. Go 1.27.4 or newer — `brew install go`.
3. For `.app` bundles only: the Fyne packager — `go install fyne.io/tools/cmd/fyne@latest` (`build.sh app` installs it for you). Make sure `$(go env GOPATH)/bin` is on your `PATH`.

No Homebrew libraries or Node needed.

CI (`.github/workflows/build-macos.yml`) builds an Apple-silicon (`arm64`) `.app` bundle on a macOS runner and publishes it as `mailrat-macos-arm64.zip` (plus a `.sha256`). Intel builds are not produced.

The `.app` also contains the command-line binary at `mailrat.app/Contents/MacOS/mailrat`, so one zip serves the desktop app, `mailrat tui` and the CLI. This is the artifact for a Homebrew **cask** (`binary "mailrat.app/Contents/MacOS/mailrat"` puts `mailrat` on the `PATH`).

#### Signing in

Mail access goes through your own Google Cloud OAuth client. On first run choose **Add Account…**; if no client is installed the app walks you through creating one and lets you pick the downloaded `credentials.json` (it is copied to `~/.mailrat/`). Sign-in opens your browser; the local callback is state-checked, and if the browser can't reach the app (remote machine) you can paste the redirect address instead. `mailrat accounts add` offers the same flow in the terminal. Tokens refresh automatically and are stored in the local database.

#### HTML mail

Messages render as styled text (links, lists, quotes, headings, bold/italic) in your system light or dark appearance. Author text colours are kept only when readable on the current background, so mail that ignores dark mode never shows black on black. Remote images are blocked until you choose *Load remote images* for that message.

---

### CLI

```bash
mailrat <command> [flags]
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
Gmail API ↔ Sync Engine ↔ SQLite ↔ Go Services ↔ UI (CLI / TUI / Fyne)
```

### Design principles

- **Go backend owns all state** — no business logic in the UI layer
- **SQLite is the single source of truth** — queries run locally; offline reads are instant
- **Gmail API as sync layer only** — initial sync via date range, incremental sync via `historyId`
- **Lazy body fetch** — metadata is synced first; full bodies are fetched on demand

### Package layout

```
cmd/mailrat/              single binary entry point
  main.go                 root command + desktop launch
  FyneApp.toml, icon.png  app bundle metadata for `fyne package`

internal/
  accounts/               OAuth2 flow and token management
  cli/                    Cobra commands + TUI launch
  contacts/               Google People API integration
  desktop/                backend facade (App) and UI-facing item types
  ui/                     Fyne desktop UI (sidebar, thread list, reader, compose, settings)
  gmail/                  Gmail API client wrapper with retry / backoff
  mail/                   Business logic — inbox, send, labels, drafts
  storage/                SQLite schema and CRUD
  sync/                   Initial and incremental sync engine
  tui/                    Bubbletea models and views
```

---

## Development

```bash
go test ./...                          # run tests
./build.sh                             # build binary (needs cgo + OpenGL/X11 headers on Linux)
go vet ./internal/...                  # vet
MAILRAT_SHOT=out.png go test ./internal/ui   # render the UI headlessly to a PNG
```

---

## License

MIT
