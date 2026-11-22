# mailrat — Project Progress

## What it is

Single Go binary, three interfaces: native Fyne v2 desktop app (Mimestream-style), Bubbletea TUI, CLI. SQLite is the source of truth; Gmail API used only for sync and mutations.

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
- Local multi-term search (`/` in TUI, `SearchInbox` in desktop/web): searches subject, body, from/to/cc/bcc, and attachment filenames; space-separated terms ranked by match count descending (`LocalSearchThreadSummaries` in `internal/storage/threads.go`)

## Key files

| What | Where |
|------|-------|
| Fyne UI (state, actions, events) | `internal/ui/ui.go` |
| Sidebar / thread list / reader | `internal/ui/sidebar.go`, `threadlist.go`, `reader.go` |
| Compose / settings windows | `internal/ui/compose.go`, `settings.go` |
| Theme + icons | `internal/ui/theme.go`, `icons.go` |
| Backend facade used by UI | `internal/desktop/app.go` |
| Email send | `internal/mail/send.go` |
| Label ops (read/unread/trash) | `internal/mail/labels.go` |
| Gmail API client | `internal/gmail/client.go` |
| SQLite store | `internal/storage/` |
| Thread list queries | `internal/storage/threads.go` |
| Contacts search | `internal/storage/contacts.go` |
| Pending ops queue | `internal/storage/pending_ops.go` |
| TUI | `internal/tui/` |

## Architecture notes

**UI/backend seam** — `internal/ui` depends only on the `Backend` interface (`backend.go`), satisfied by `desktop.App`. Backend events (`inbox:updated`, `sync:complete`, …) arrive via `SetEventHandler`; UI work from goroutines must go through `fyne.Do`. `ui` imports no Fyne driver, so it compiles and tests headlessly (fake backend in `ui_test.go`); only `cmd/mailrat` imports `fyne.io/fyne/v2/app` (cgo).

**Offline-first mutations** — `App` methods update SQLite optimistically before calling Gmail. On failure, they enqueue a `pending_ops` row. `DrainPendingOps()` is called after every successful sync. TUI uses the same table via `drainPendingOpsCmd`.

**Compose modes** — `composeMode` (`'new'` | `'reply'` | `'forward'`) controls full-pane vs inline compose. Two `x-ref="editor"` in mutually exclusive `x-if` blocks.

**`@keydown.window.capture`** — keyboard shortcuts that conflict with Chrome built-ins (e.g. `Ctrl+Shift+C` for DevTools, `Ctrl+Shift+B` for bookmarks) require capture-phase listeners, not bubble-phase. Same technique Gmail uses.


**Pagination** — `GetInbox` and `GetLabelThreads` accept `offset int`. Frontend tracks `loadingMore` / `allLoaded`. `onListScroll()` on the list container triggers `loadMore()` when within 200px of bottom. Search results disable pagination (`allLoaded = true`).

## Known gaps / TODO

- No mail signature management
- No send validation (blank To is allowed)
- No server-side Gmail search (local SQLite only)
