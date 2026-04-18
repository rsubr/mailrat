# mailrat — Project Progress

## What it is

Single Go binary, four interfaces: Wails desktop app, Bubbletea TUI, HTTP web server (`mailrat web`), CLI. Alpine.js + Tailwind SPA frontend embedded via `//go:embed`. SQLite is the source of truth; Gmail API used only for sync and mutations.

## Completed features

- Multi-account unified inbox (SQLite-backed, offline-first)
- Full Gmail label support
- Google Contacts sync
- Rich text compose: contenteditable editor, formatting toolbar (bold/italic/underline/strikethrough/lists/links), `multipart/alternative` HTML+plain emails
- Gmail-style reply/forward with quoted original (attribution + blockquote)
- RFC 2822 reply threading (`In-Reply-To` + `References` headers)
- Inline compose at bottom of thread for reply/forward; full-pane compose for new mail
- Auto mark-as-read after 5 seconds (cancelled on navigation away)
- Trash thread (all messages moved to Gmail Trash, removed from local inbox)
- Mark message as unread
- Offline queuing: mark-read, mark-unread, trash queue to `pending_ops` when offline; replayed on next sync
- Alpine.js and Tailwind vendored — no CDN requests

## Key files

| What | Where |
|------|-------|
| Frontend HTML | `cmd/mailrat/frontend/index.html` |
| Frontend JS | `cmd/mailrat/frontend/js/app.js` |
| Web/Wails bridge shim | `cmd/mailrat/frontend/js/wails-compat.js` |
| HTTP web server | `cmd/mailrat/main.go` |
| Desktop App struct (Wails bindings) | `internal/desktop/app.go` |
| Email send | `internal/mail/send.go` |
| Label ops (read/unread/trash) | `internal/mail/labels.go` |
| Gmail API client | `internal/gmail/client.go` |
| SQLite store | `internal/storage/` |
| Pending ops queue | `internal/storage/pending_ops.go` |
| TUI | `internal/tui/` |

## Architecture notes

**Wails bridge pattern** — `Wails.call(method, ...args)` in `app.js` checks `window.go` availability. `wails-compat.js` polyfills `window.go.desktop.App.*` with fetch calls for web mode. Every new Go method on `App` must also get an HTTP route in `cmd/mailrat/main.go` and an entry in `wails-compat.js` to work in web mode.

**Offline-first mutations** — `App` methods update SQLite optimistically before calling Gmail. On failure, they enqueue a `pending_ops` row. `DrainPendingOps()` is called after every successful `/api/sync`. TUI uses the same table via `drainPendingOpsCmd`.

**Alpine.js store** — all shared state in `Alpine.store('mail')`. Components (`sidebar`, `emailList`, `readingPane`) are thin wrappers.

**Compose modes** — `composeMode` (`'new'` | `'reply'` | `'forward'`) controls full-pane vs inline compose. Two `x-ref="editor"` in mutually exclusive `x-if` blocks.

## Known gaps / TODO

- No mail signature management
- Labels not shown in sidebar beyond system labels
- No keyboard shortcuts in web/desktop SPA
- Inbox capped at 60 threads, no pagination
- Search is local SQLite only (no Gmail server-side search)
- No CC/BCC fields in compose
- No send validation (blank To is allowed)
