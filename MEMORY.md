# mailrat — Project Progress

## What it is

Single Go binary, four interfaces: Wails desktop app, Bubbletea TUI, HTTP web server (`mailrat web`), CLI. Alpine.js + Tailwind SPA frontend embedded via `//go:embed`. SQLite is the source of truth; Gmail API used only for sync and mutations.

## Completed features

- Multi-account unified inbox (SQLite-backed, offline-first)
- Full Gmail label support with real labels in sidebar (collapsible "More" section)
- Label chips per thread in the thread list
- Star indicator in thread list (SVG icon, detects `STARRED` label ID)
- Google Contacts sync
- Rich text compose: contenteditable editor, formatting toolbar (bold/italic/underline/strikethrough/lists/links), `multipart/alternative` HTML+plain emails
- Gmail-style reply/forward with quoted original (attribution + blockquote)
- RFC 2822 reply threading (`In-Reply-To` + `References` headers)
- Inline compose at bottom of thread for reply/forward; full-pane compose for new mail
- CC/BCC fields in compose (new message and reply/forward), shown/hidden via Cc/Bcc buttons or `Cmd/Ctrl+Shift+C` / `Cmd/Ctrl+Shift+B`
- CC/BCC displayed in message view
- Address autocomplete for To/Cc/Bcc: searches local contacts DB as user types, arrow/Enter/Tab keyboard navigation, multi-address comma support
- Auto mark-as-read after 5 seconds (cancelled on navigation away)
- Trash thread (all messages moved to Gmail Trash, removed from local inbox)
- Mark message as unread
- Offline queuing: mark-read, mark-unread, trash queue to `pending_ops` when offline; replayed on next sync
- Light/dark theme toggle with localStorage persistence
- SVG sprite-based icon system
- Infinite scroll pagination: scrolling to bottom of email list fetches next 60 threads (OFFSET-based), spinner shown while loading, stops when end of cache reached
- Alpine.js and Tailwind vendored — no CDN requests
- Local multi-term search (`/` in TUI, `SearchInbox` in desktop/web): searches subject, body, from/to/cc/bcc, and attachment filenames; space-separated terms ranked by match count descending (`LocalSearchThreadSummaries` in `internal/storage/threads.go`)

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
| Thread list queries | `internal/storage/threads.go` |
| Contacts search | `internal/storage/contacts.go` |
| Pending ops queue | `internal/storage/pending_ops.go` |
| TUI | `internal/tui/` |

## Architecture notes

**Wails bridge pattern** — `Wails.call(method, ...args)` in `app.js` checks `window.go` availability. `wails-compat.js` polyfills `window.go.desktop.App.*` with fetch calls for web mode. Every new Go method on `App` must also get an HTTP route in `cmd/mailrat/main.go` and an entry in `wails-compat.js` to work in web mode.

**Offline-first mutations** — `App` methods update SQLite optimistically before calling Gmail. On failure, they enqueue a `pending_ops` row. `DrainPendingOps()` is called after every successful `/api/sync`. TUI uses the same table via `drainPendingOpsCmd`.

**Alpine.js store** — all shared state in `Alpine.store('mail')`. Components (`sidebar`, `emailList`, `readingPane`) are thin wrappers.

**Compose modes** — `composeMode` (`'new'` | `'reply'` | `'forward'`) controls full-pane vs inline compose. Two `x-ref="editor"` in mutually exclusive `x-if` blocks.

**`@keydown.window.capture`** — keyboard shortcuts that conflict with Chrome built-ins (e.g. `Ctrl+Shift+C` for DevTools, `Ctrl+Shift+B` for bookmarks) require capture-phase listeners, not bubble-phase. Same technique Gmail uses.

**`addressField` Alpine component** — registered via `Alpine.data('addressField', (field) => ...)`. Reads/writes `$store.mail.composeData[field]` via getter/setter. Autocompletes the last comma-separated token. Tab accepts first suggestion; Enter requires explicit arrow-key selection.

**Pagination** — `GetInbox` and `GetLabelThreads` accept `offset int`. Frontend tracks `loadingMore` / `allLoaded`. `onListScroll()` on the list container triggers `loadMore()` when within 200px of bottom. Search results disable pagination (`allLoaded = true`).

## Known gaps / TODO

- No mail signature management
- No send validation (blank To is allowed)
- No server-side Gmail search (local SQLite only)
