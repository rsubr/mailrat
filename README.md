# mailrat

> **Alpha software.** mailrat is under active development: expect bugs, breaking changes and possible data issues. Use at your own risk, and back up any important mail before connecting an account.

A native Gmail desktop client built with Go and Fyne. Go owns all state, SQLite is the source of truth, and the UI is a thin rendering layer.

---

## Features

- **Desktop app** - native Fyne UI laid out after Mimestream: accounts and mailboxes on the left, thread list in the middle, conversation on the right; follows the system light/dark appearance
- **Multi-account** - several Gmail accounts in one window, with a unified inbox
- **Offline-first** - mail and contacts are read from local SQLite; archive, trash, spam and read-state changes made offline are queued and replayed
- **Gmail API sync** - no IMAP; initial sync by date range, incremental sync via `historyId`, bodies fetched on demand; server-side deletions are removed locally
- **Sync window** - choose per account (all time, past year, 90 or 30 days) and per label how much mail stays synced
- **Labels** - system and user labels, label unread counts, create labels, apply labels to threads
- **Search** - search the local mailbox
- **Contacts** - sync, search and manage Google contacts, with address suggestions in compose
- **Rich text compose** - bold, italic, underline, strikethrough, lists and links; sent as `multipart/alternative` with a plain-text fallback; attachments; drafts synced to Gmail
- **Reply / forward** - quoted original with Gmail-style attribution, correct `In-Reply-To` / `References` threading, send-as aliases and signatures
- **Attachments** - metadata loads lazily; open on click or save from the context menu
- **Reading** - threads mark read after a short delay; trash with undo, archive, spam, star, mark unread

---

## Requirements

- Go 1.27.4 or newer, plus cgo (Xcode Command Line Tools on macOS; gcc, `libgl1-mesa-dev` and X11 dev packages on Linux)
- A Google Cloud OAuth 2.0 client (`credentials.json`) with the Gmail (`gmail.readonly`, `gmail.modify`, `gmail.send`) and Contacts (People API) scopes. The app walks you through importing it on first run; it is copied to `~/.mailrat/credentials.json`.

---

## Install and run

Homebrew (macOS, Apple silicon):

```bash
brew tap rsubr/mailrat
brew install --cask mailrat
```

From source:

```bash
git clone https://github.com/rsubr/mailrat.git
cd mailrat
go run ./cmd/mailrat      # or ./build.sh for a binary, ./build.sh app for a macOS .app
```

Flags and environment: `--data-dir <dir>` (or `MAILRAT_DATA_DIR`) selects the profile directory (default `~/.mailrat`); `MAILRAT_OAUTH_PORT` fixes the OAuth callback port; `MAILRAT_OAUTH_HEADLESS` skips opening a browser.

---

## Using the app

Launches the native Fyne app, laid out after Mimestream: account tiles and mailboxes on the left, thread list in the middle, conversation and toolbar on the right. Follows the system light/dark appearance.

Keyboard shortcuts follow Gmail on the web. Press `?` in the app for the list.

| Keys | Action |
|---|---|
| `g` then `i` `s` `t` `d` `a` `l` | Go to Inbox, Starred, Sent, Drafts, All Mail, or a label |
| `j` / `k` | Open next (older) / previous (newer) conversation |
| `n` / `p` | Next / previous message in the conversation |
| `o` or `Enter` | Open conversation; expand or collapse the current message |
| `Esc` | Back to the conversation list |
| `c` or `d` | Compose |
| `/` | Search (`Esc` leaves the field) |
| `e` or `y` | Archive / remove from view |
| `#` / `!` | Trash / report spam |
| `s` | Star or unstar |
| `+` `=` / `-` | Mark important / not important |
| `r` `a` `f` (or capitals) | Reply, reply all, forward |
| `⇧I` / `⇧U` or `_` | Mark read / unread |
| `u` / `⇧N` | Check for new mail |
| `⌘Enter` | Send (in a message window) |
| `⌘N` `⌘R` `⇧⌘R` `⇧⌘F` `⌘⌫` `⌘,` `⌘G` | New, reply, reply all, forward, trash, settings, check mail |

Not implemented: multi-select (`x`, `*` sequences), mute (`m`), move (`v`), label (`l`), undo (`z`).

#### Building

Fyne needs cgo, so build natively on the target OS (no Linux to macOS cross-compile without an Apple SDK):

```bash
./build.sh        # ./mailrat binary
./build.sh app    # ./MailRat.app bundle (macOS; installs the Fyne packager if missing)
```

CI (`.github/workflows/build-macos.yml`) builds an Apple-silicon `.app` on PRs to `main` and on `v*` tags. A tag on `main` publishes `mailrat-macos-arm64.zip` (plus `.sha256`) as a GitHub release and updates the Homebrew cask in [rsubr/homebrew-mailrat](https://github.com/rsubr/homebrew-mailrat) (needs repo secret `HOMEBREW_TAP_DEPLOY_KEY`). Intel builds are not produced.

#### Signing in

Mail access goes through your own Google Cloud OAuth client. On first run choose **Add Account…**; if no client is installed the app walks you through creating one and lets you pick the downloaded `credentials.json` (it is copied to `~/.mailrat/`). Sign-in opens your browser; the local callback is state-checked, and if the browser can't reach the app (remote machine) you can paste the redirect address instead. Tokens refresh automatically and are stored in the local database.

#### HTML mail

Messages render as styled text (links, lists, quotes, headings, bold/italic) in your system light or dark appearance. Author text colours are kept only when readable on the current background, so mail that ignores dark mode never shows black on black. Remote images are blocked until you choose *Load remote images* for that message.

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
Gmail API ↔ Sync Engine ↔ SQLite ↔ Go Services ↔ UI (Fyne)
```

### Design principles

- **Go backend owns all state** — no business logic in the UI layer
- **SQLite is the single source of truth** — queries run locally; offline reads are instant
- **Gmail API as sync layer only** — initial sync via date range, incremental sync via `historyId`
- **Lazy body fetch** — metadata is synced first; full bodies are fetched on demand

### Package layout

```
cmd/mailrat/              single binary entry point
  main.go                 desktop launch
  FyneApp.toml, icon.png  app bundle metadata for `fyne package`

internal/
  accounts/               OAuth2 flow and token management
  contacts/               Google People API integration
  desktop/                backend facade (App) and UI-facing item types
  ui/                     Fyne desktop UI (sidebar, thread list, reader, compose, settings)
  gmail/                  Gmail API client wrapper with retry / backoff
  mail/                   Business logic — inbox, send, labels, drafts
  storage/                SQLite schema and CRUD
  sync/                   Initial and incremental sync engine
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
