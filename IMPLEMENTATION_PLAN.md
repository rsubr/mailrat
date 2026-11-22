# Implementation Plan: mailrat (Native Gmail Client)

This plan follows the three-phase strategy from the PRD. Each step has a clear deliverable and a verification check.

---

## Phase 1: CLI Backend (Headless Mail Engine)

### Step 1 — Project Scaffold

1. `go mod init github.com/rajasubramanian/mailrat` (or chosen module path)
2. Create directory structure:
   ```
   cmd/
     cli/
       main.go
   internal/
     accounts/
     gmail/
     sync/
     storage/
     mail/
     contacts/
   ```
3. Add core dependencies:
   - `google.golang.org/api` (Gmail + People API)
   - `golang.org/x/oauth2`
   - `modernc.org/sqlite` (CGO-free SQLite driver)
   - `github.com/spf13/cobra` (CLI framework)

**Verify:** `go build ./...` succeeds with empty packages.

---

### Step 2 — SQLite Storage Layer (`internal/storage`)

1. Define schema migrations (run on startup):
   - `accounts` table
   - `messages` table
   - `threads` table
   - `labels` table
   - `message_labels` join table
   - `drafts` table
2. Apply SQLite configuration: WAL mode, foreign keys enabled, busy timeout.
3. Create indexes:
   - `messages(account_id, internal_date DESC)`
   - `message_labels(label_id, message_id)`
4. Implement `Store` struct with methods:
   - `UpsertAccount`, `GetAccounts`, `DeleteAccount`
   - `UpsertMessage`, `GetMessage`, `ListMessages`
   - `UpsertThread`, `GetThread`
   - `UpsertLabel`, `ListLabels`
   - `SetMessageLabels`
   - `UpsertDraft`, `GetDraft`, `DeleteDraft`

**Verify:** Unit tests covering each CRUD operation against an in-memory SQLite DB.

---

### Step 3 — OAuth & Account Management (`internal/accounts`)

1. Define OAuth2 config using Gmail API scopes:
   - `gmail.readonly`, `gmail.modify`, `gmail.send`
2. Implement authorization flow:
   - Open browser to consent URL
   - Start local redirect listener
   - Exchange code for token
3. Store tokens in `accounts` table (encrypted at rest or via OS keychain on macOS using `github.com/zalando/go-keyring`).
4. Implement token refresh on expiry.
5. Expose `AccountManager`:
   - `AddAccount()` — OAuth flow
   - `ListAccounts()` — read from storage
   - `GetClient(accountID)` — returns authenticated `*http.Client`

**Verify:** `mailcli accounts add` completes OAuth flow and stores account. `mailcli accounts list` prints the account email.

---

### Step 4 — Gmail API Wrapper (`internal/gmail`)

1. Wrap `google.golang.org/api/gmail/v1` with a typed client:
   - `ListMessages(query string, maxResults int64) ([]MessageMeta, error)`
   - `GetMessage(id string, format string) (*Message, error)`
   - `GetThread(id string) (*Thread, error)`
   - `ListLabels() ([]Label, error)`
   - `ModifyLabels(msgID string, add, remove []string) error`
   - `SendMessage(raw string) error`
   - `CreateDraft(raw string) (string, error)`
   - `GetHistoryList(startHistoryID uint64) (*HistoryResponse, error)`
2. Define domain types: `MessageMeta`, `Message`, `Thread`, `Label`.
3. Handle rate limit errors (429) with exponential backoff.

**Verify:** Integration test (with real credentials in CI secret or manually) that lists inbox messages.

---

### Step 5 — Sync Engine (`internal/sync`)

1. Implement **Initial Sync**:
   - Query Gmail with `newer_than:Xd` (configurable: 7/30/90 days)
   - Paginate through all results
   - Fetch message metadata (headers, snippet, labels)
   - Write to storage with `cache_state = metadata_only`
2. Implement **Incremental Sync**:
   - Store `historyId` per account in `accounts` table
   - On sync, call `history.list` from last `historyId`
   - Process: `messagesAdded`, `labelsAdded`, `labelsRemoved`
   - Update storage accordingly
3. Implement **Body Fetch** (lazy):
   - Given message IDs, fetch full body from Gmail API
   - Update `body` field and set `cache_state = body_cached`
4. Expose `SyncManager`:
   - `InitialSync(accountID, days int) error`
   - `IncrementalSync(accountID string) error`
   - `FetchBodies(accountIDs []string) error`

**Verify:** After `mailcli sync --days=30`, messages appear in SQLite. Second sync completes faster via historyId.

---

### Step 6 — Mail Business Logic (`internal/mail`)

1. `InboxService`:
   - `GetUnifiedInbox(limit int) ([]Thread, error)` — merges across all accounts, sorted by `internal_date DESC`
   - `GetThread(accountID, threadID string) ([]Message, error)`
   - `GetMessage(accountID, messageID string) (*Message, error)` — fetches body if not cached
2. `SendService`:
   - `Send(accountID string, msg OutboundMessage) error` — build RFC 2822, base64url-encode, call Gmail send
   - `Reply(accountID, threadID string, msg OutboundMessage) error`
3. `LabelService`:
   - `Archive(accountID, messageID string) error`
   - `MarkRead(accountID, messageID string) error`
   - `MarkUnread(accountID, messageID string) error`
   - `ApplyLabel(accountID, messageID, labelID string) error`
4. `DraftService`:
   - `SaveDraft(accountID string, msg OutboundMessage) (string, error)`
   - `GetDraft(accountID, draftID string) (*Draft, error)`
   - `DeleteDraft(accountID, draftID string) error`

**Verify:** Unit tests for inbox merge logic. Integration test for send.

---

### Step 7 — Contacts (`internal/contacts`)

1. Wrap Google People API (`people.googleapis.com`):
   - Sync contacts to local SQLite `contacts` table (name, emails)
   - `SearchContacts(query string) ([]Contact, error)` — local SQLite FTS or LIKE query
2. Add `contacts` table to schema migration.

**Verify:** `mailcli contacts list` shows synced contacts.

---

### Step 8 — CLI Commands (`cmd/cli`)

Wire up Cobra commands using the services above:

| Command | Action |
|---|---|
| `mailcli accounts add` | OAuth flow |
| `mailcli accounts list` | Print accounts |
| `mailcli sync --days=N` | Initial or incremental sync |
| `mailcli inbox list` | Print unified inbox threads |
| `mailcli message read <id>` | Print message body |
| `mailcli send` | Interactive send (prompts To/Subject/Body) |
| `mailcli labels list` | Print labels for account |
| `mailcli contacts list` | Print synced contacts |

**Verify:** All Phase 1 Definition of Done criteria met:
- Account auth works
- Sync works for last X days
- Inbox query is fast (< 300ms from SQLite)
- Messages readable offline
- Sending emails works
- Contacts available

---

## Phase 2: TUI (Bubbletea)

### Step 9 — TUI Scaffold (`cmd/tui`)

1. Add dependencies:
   - `github.com/charmbracelet/bubbletea`
   - `github.com/charmbracelet/lipgloss`
   - `github.com/charmbracelet/bubbles`
2. Create `cmd/tui/main.go` entry point — initializes services (same as CLI), starts Bubbletea program.
3. Define top-level `Model` with an enum for active view: `InboxView`, `ThreadView`, `MessageView`, `ComposeView`.

**Verify:** `go run ./cmd/tui` launches without error and shows an empty frame.

---

### Step 10 — Inbox View

1. Use `bubbles/list` component for thread list.
2. Load threads from `InboxService.GetUnifiedInbox()` on startup.
3. Display: subject/snippet, sender, date, unread indicator.
4. Keyboard: `j/k` or arrows to move, `Enter` to open thread, `s` to sync, `q` to quit.

**Verify:** Inbox list renders and keyboard navigation works.

---

### Step 11 — Thread & Message Views

1. **Thread view:** list of messages in thread using `bubbles/list`.
2. **Message reader:** `bubbles/viewport` for scrollable body. Show From/To/Date header.
3. Navigation: `Enter` opens message from thread view, `Esc` goes back.

**Verify:** Can open a thread and read message body.

---

### Step 12 — Compose View

1. Multi-field form using `bubbles/textinput` for To, Subject and `bubbles/textarea` (or viewport) for Body.
2. `Tab` to move between fields, `Ctrl+Enter` to send, `Esc` to discard.
3. Calls `SendService.Send()` on submit.
4. Triggered by `c` (compose) or `r` (reply) from inbox/message views.

**Verify:** Can compose and send an email from the TUI.

---

### Step 13 — Background Sync in TUI

1. Run `SyncManager.IncrementalSync()` in a goroutine on startup and on `s` keypress.
2. Send sync progress as `tea.Msg` to update a status bar.
3. On sync complete, reload inbox list.

**Verify:** Sync runs in background, status bar updates, inbox refreshes automatically.

---

### Step 14 — Account Switcher

1. Add account switcher overlay (triggered by `a`) — list of accounts + "Unified Inbox".
2. Selecting an account filters inbox to that account only.

**Verify:** Phase 2 Definition of Done met — all TUI views functional, keyboard shortcuts work, background sync updates UI.

---

## Phase 3: Desktop App (Fyne)

Native Fyne v2 UI in `internal/ui`, driven by the `desktop.App` backend facade through the `ui.Backend` interface. Mimestream-style three-pane layout, Gmail web keyboard shortcuts, HTML mail rendered as styled text in light and dark.

**Verify:** `go test ./internal/ui` renders the UI headlessly; `./build.sh` builds the app on macOS.

---

## Cross-Cutting Concerns

### Error Handling
- All Gmail API errors surface to user with actionable messages.
- Network errors during sync are non-fatal — app works offline with cached data.

### Security
- OAuth tokens never logged.
- On macOS, use Keychain for token storage.
- HTML email bodies sanitized before rendering (remote images blocked, only http/https/mailto links active).

### Testing Strategy
- Unit tests: storage layer, mail business logic, sync processing.
- Integration tests: Gmail API calls (run manually or with CI secrets).
- TUI tests: Bubbletea model update functions are pure — test without rendering.

---

## Dependency Summary

| Package | Purpose |
|---|---|
| `google.golang.org/api` | Gmail + People API |
| `golang.org/x/oauth2` | OAuth2 flow |
| `modernc.org/sqlite` | CGO-free SQLite |
| `github.com/spf13/cobra` | CLI commands |
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/charmbracelet/bubbles` | TUI components |
| `github.com/zalando/go-keyring` | macOS Keychain |
| `fyne.io/fyne/v2` | Desktop app framework |
