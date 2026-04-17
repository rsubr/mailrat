# Refactor Plan

This document turns the code-quality findings into an execution plan. The goal is to reduce hotspot complexity, isolate business logic from UI wiring, and create better seams for testing without changing product behavior.

## Objectives

- Reduce complexity in TUI event handling and CLI command registration.
- Move business logic out of Cobra/Bubble Tea handlers into reusable services.
- Clarify package boundaries between transport, storage, sync, and mail actions.
- Remove duplicated prompting, app bootstrap, and query-building logic.
- Increase testability by introducing narrower interfaces and smaller pure helpers.

## Constraints

- Preserve current CLI and TUI behavior.
- Keep refactors incremental and commit in coherent batches.
- Avoid destructive changes to untracked user files.
- Prefer structural refactors over feature work.

## Workstreams

### 1. TUI Decomposition

Files:
- `internal/tui/tui.go`
- `internal/tui/cmds.go`
- `internal/tui/inbox.go`
- `internal/tui/thread.go`
- `internal/tui/message.go`
- `internal/tui/compose.go`
- `internal/tui/accounts.go`
- `internal/tui/config.go`

Problems:
- Root `model` owns all screen state.
- `Update` is the largest hotspot and acts as a global reducer.
- UI commands mix domain logic, I/O, optimistic state updates, and offline queuing.
- Screen-specific transitions and keymaps are spread across several files.

Refactors:
- Split root event handling into root-level message handlers plus screen handlers.
- Group `model` fields into nested state structs:
  - `navState`
  - `inboxState`
  - `threadState`
  - `messageState`
  - `composeState`
  - `configState`
  - `uiState`
- Extract root message handlers from `Update` into dedicated methods:
  - window resize
  - account load
  - inbox load
  - thread load
  - message load
  - mark-read completion
  - sync completion
  - send completion
  - config load/save completion
- Move TUI command concerns into separate files by responsibility:
  - `cmds_load.go`
  - `cmds_sync.go`
  - `cmds_mail.go`
  - `cmds_config.go`
- Move optimistic mail actions and pending-op replay behind small helper functions so Bubble Tea commands orchestrate instead of own behavior.
- Standardize screen transition helpers:
  - open inbox
  - open thread
  - open message
  - open compose
  - open accounts
  - open config
- Keep rendering methods separate from state mutation.

Definition of done:
- `tui.go` no longer contains a monolithic `Update`.
- TUI commands are grouped by responsibility.
- The TUI `model` is easier to reason about and can be tested screen by screen.

### 2. CLI Command Construction And Prompting

Files:
- `internal/cli/root.go`
- `internal/cli/app.go`
- `internal/cli/accounts_cmd.go`
- `internal/cli/contacts_cmd.go`
- `internal/cli/inbox_cmd.go`
- `internal/cli/labels_cmd.go`
- `internal/cli/message_cmd.go`
- `internal/cli/send_cmd.go`
- `internal/cli/sync_cmd.go`
- `internal/cli/tui_launch.go`

Problems:
- Large Cobra `init()` functions build several commands inline.
- Command handlers repeat app setup, context creation, account resolution, and confirmation logic.
- Interactive prompting is duplicated.
- Bootstrap logic is partly centralized but not exposed as reusable command helpers.

Refactors:
- Replace large `init()` builders with command factory functions:
  - `newAccountsCmd()`
  - `newContactsCmd()`
  - other command constructors as needed
- Split subcommands into smaller helpers rather than anonymous `RunE` blocks.
- Extract reusable prompt utilities for:
  - string prompt
  - prompt with default
  - yes/no confirmation
  - typed confirmation
- Add command-scoped helpers for:
  - opening app
  - closing app
  - resolving account ID
  - creating service instances
- Keep `root.go` as the assembly point for all top-level commands.

Definition of done:
- The main CLI command files do not use giant inline `init()` functions.
- Repeated prompt and bootstrap patterns are centralized.
- Each command handler is small and focused.

### 3. Sync Pipeline And Query Planning

Files:
- `internal/sync/manager.go`

Problems:
- Sync methods combine policy, fetch loops, persistence, and progress printing.
- Query building is embedded in the manager.
- Backfill logic mutates config defaults inline.
- Logging is coupled to stdout.

Refactors:
- Split sync logic into focused helpers or files:
  - label sync
  - initial fetch planning
  - incremental history reconciliation
  - backfill planning
  - body/attachment fetch
- Extract Gmail query construction into dedicated helpers.
- Extract sync-config defaulting into a small policy helper rather than mutating inline inside a major workflow.
- Replace direct `fmt.Printf` reporting with an optional logger/progress sink interface.
- Make fetch/store behavior reusable across initial sync and backfill.

Definition of done:
- `manager.go` reads as orchestration rather than a single implementation blob.
- Query policy and output/reporting are isolated.

### 4. OAuth Flow Separation

Files:
- `internal/accounts/oauth.go`

Problems:
- OAuth flow mixes callback server setup, browser open, manual fallback, timeout, code parsing, HTML response, and token exchange.
- State and output details are embedded inline.

Refactors:
- Split flow into dedicated helpers:
  - callback listener setup
  - auth URL generation
  - browser-mode wait
  - manual-mode fallback
  - token exchange
- Hoist hard-coded values into named constants.
- Isolate terminal messaging from the mechanics of OAuth.

Definition of done:
- `runOAuthFlow` becomes a coordinator over smaller functions.
- Browser and manual paths are explicit and testable.

### 5. Storage Query And Merge Cleanup

Files:
- `internal/storage/messages.go`
- `internal/storage/threads.go`
- `internal/storage/pending_ops.go`
- `internal/storage/sync_config.go`
- related storage files if needed

Problems:
- Message repository methods are specialized and duplicated.
- `UpsertMessage` embeds merge semantics inline.
- Storage packages expose low-level patterns without higher-level composition helpers.

Refactors:
- Extract message merge rules into named helper logic.
- Consolidate repeated list/query behavior around shared query builders or common scan paths.
- Keep SQL close to the repository methods but remove avoidable duplication.
- Ensure pending-op and sync-config repositories have clearer, smaller APIs where possible.

Definition of done:
- Storage methods are easier to scan and maintain.
- Merge semantics are explicit and testable.

### 6. Package Boundaries And Interfaces

Files:
- `internal/mail/*`
- `internal/cli/app.go`
- `internal/tui/*`
- `internal/sync/manager.go`
- `internal/gmail/*`

Problems:
- UI code reaches directly into storage and managers.
- Domain operations are spread across UI and infrastructure code.

Refactors:
- Introduce narrow interfaces where they improve testability and decouple UI from infrastructure:
  - mail reader
  - mail actions
  - contact service
  - sync runner
- Keep `gmail` as a transport adapter and `storage` as persistence.
- Move user-facing mail actions toward `internal/mail` services where practical.

Definition of done:
- CLI/TUI depend on smaller abstractions instead of broad concrete services where that materially helps.
- Package responsibilities are clearer than they are today.

## Execution Order

1. Land the TUI decomposition because it is the largest hotspot.
2. Refactor CLI command construction and prompt helpers.
3. Refactor sync pipeline and OAuth helpers.
4. Clean up storage merge/query behavior.
5. Tighten interfaces and package boundaries.
6. Run tests and fix integration regressions.

## Commit Plan

- Commit 1: add refactor plan document
- Commit 2: TUI structural refactor
- Commit 3: CLI command and prompt refactor
- Commit 4: sync/OAuth/storage refactor
- Commit 5: integration cleanup and test fixes

## Verification

- `go test ./...`
- Smoke-check CLI command construction via `mailrat --help` and selected subcommand help.
- Build the binary if the refactor touches assembly or package wiring.
