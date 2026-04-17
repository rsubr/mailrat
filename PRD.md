
Product Requirements Document (PRD)

Native Gmail Client (Go + Wails)

⸻

1. Product Overview

1.1 Product Name

Native Gmail Desktop Client (Working name)

1.2 Summary

A native macOS desktop email client built using Go and Wails that integrates directly with the Gmail API (not IMAP). The application provides a fast, offline-capable, multi-account unified inbox with a backend-driven architecture where Go manages all application state and synchronization.

1.3 Core Philosophy
    •    Go backend owns all application state
    •    SQLite is the single source of truth
    •    Frontend is a thin rendering layer
    •    Gmail API is used strictly as a sync layer

⸻

2. Goals & Non-Goals

2.1 Goals
    •    Unified inbox across multiple Gmail accounts
    •    Offline-first experience with local caching
    •    Full support for Gmail labels and threads
    •    Lightweight frontend with minimal state
    •    Deterministic and debuggable backend logic

2.2 Non-Goals
    •    Support for non-Gmail providers
    •    Full feature parity with Gmail web UI
    •    Server-side infrastructure (initially)
    •    Complex frontend state management

⸻

3. Phased Development Strategy

Phase 1: CLI Backend (Headless Mail Engine)

Purpose:
    •    Validate Gmail API integration
    •    Build sync engine
    •    Implement storage layer
    •    Test all functionality without UI

Deliverables:
    •    CLI application with full mail functionality
    •    SQLite-based storage
    •    OAuth account management

⸻

Phase 2: TUI (Bubbletea)

Purpose:
    •    Build a terminal-based UI using the Bubbletea library
    •    Provide a full interactive mail experience in the terminal
    •    Reuse all Phase 1 backend logic directly (no new services)

Deliverables:
    •    Bubbletea TUI application
    •    Pane-based layout: inbox list, thread view, message reader, compose
    •    Keyboard-driven navigation
    •    Real-time sync status display

⸻

Phase 3: Desktop App (Wails + Frontend)

Purpose:
    •    Add UI layer
    •    Reuse backend logic
    •    Maintain thin frontend architecture

Deliverables:
    •    Single Page Application (SPA)
    •    Event-driven UI
    •    No business logic in frontend

⸻

4. Functional Requirements

⸻

4.1 Account Management

Features
    •    Add Gmail account via OAuth2
    •    Store tokens securely (macOS Keychain)
    •    Support multiple accounts

Requirements
    •    Use Gmail API OAuth scopes:
    •    gmail.readonly
    •    gmail.modify
    •    gmail.send

⸻

4.2 Mail Features

Supported
    •    View inbox
    •    View threads
    •    Read messages
    •    Send emails
    •    Drafts
    •    Labels (including spam, archive)

Not Supported (Must Implement)
    •    Schedule send (local implementation)

⸻

4.3 Labels
    •    Many-to-many relationship with messages
    •    Support:
    •    Add/remove labels
    •    Create/delete labels
    •    System labels (INBOX, SPAM, etc.)

⸻

4.4 Contacts
    •    Use Google People API
    •    Features:
    •    Contact sync
    •    Autocomplete for compose

⸻

4.5 Unified Inbox
    •    Merge messages across accounts
    •    Sort by internalDate
    •    Maintain account association

⸻

5. API Capability Constraints

Feature    Status
Read mail    Supported
Labels    Supported
Send mail    Supported
Drafts    Supported (limited)
Schedule send    Not supported
Contacts    Separate API
Push notifications    Complex


⸻

6. Architecture

⸻

6.1 High-Level Architecture

Gmail API ↔ Sync Engine ↔ SQLite ↔ Go Services ↔ UI (Wails)

⸻

6.2 Backend Components

Packages

/internal
  /accounts
  /gmail
  /sync
  /storage
  /mail
  /contacts
  /cli


⸻

Responsibilities

gmail/
    •    API wrapper

sync/
    •    Sync logic
    •    Incremental updates

storage/
    •    SQLite queries

mail/
    •    Business logic

contacts/
    •    People API integration

⸻

7. Database Design (SQLite)

⸻

7.1 Tables

accounts
    •    id
    •    email
    •    tokens

messages
    •    id
    •    thread_id
    •    account_id
    •    snippet
    •    body
    •    internal_date
    •    is_unread
    •    cache_state

threads
    •    id
    •    account_id
    •    last_message_date

labels
    •    id
    •    account_id
    •    name

message_labels
    •    message_id
    •    label_id

drafts
    •    id
    •    account_id
    •    content

⸻

7.2 Cache State

State    Description
metadata_only    headers only
body_cached    full message
attachments_cached    attachments


⸻

7.3 Indexes
    •    messages(account_id, internal_date DESC)
    •    message_labels(label_id, message_id)

⸻

7.4 SQLite Configuration
    •    WAL mode
    •    Foreign keys enabled
    •    Busy timeout set

⸻

8. Sync Engine

⸻

8.1 Sync Types

Initial Sync
    •    Query: newer_than:Xd
    •    Fetch messages + threads

Incremental Sync
    •    Use historyId
    •    Detect:
    •    new messages
    •    label changes

⸻

8.2 Sync Strategy
    1.    Fetch metadata
    2.    Store locally
    3.    Fetch bodies (lazy or batch)
    4.    Download attachments (on-demand)

⸻

8.3 Components
    •    Sync Manager
    •    Fetcher
    •    Processor
    •    Storage Writer
    •    Cache Manager

⸻

9. Cache Strategy

⸻

9.1 User Settings
    •    Sync window:
    •    7 / 30 / 90 days / all
    •    Attachments:
    •    never / on open / always
    •    Body:
    •    snippet / full

⸻

9.2 Storage Model

Always Cached
    •    metadata

Selectively Cached
    •    body
    •    attachments

⸻

9.3 Eviction Policy
    •    Time-based (older than sync window)
    •    Size-based (max cache size)

⸻

10. CLI Specification (Phase 1)

⸻

Commands

mailcli accounts add
mailcli sync --days=30
mailcli inbox list
mailcli message read <id>
mailcli send
mailcli labels list
mailcli contacts list


⸻

Purpose
    •    Integration testing
    •    Backend validation
    •    API testing

⸻

11. TUI Architecture (Phase 2)

⸻

11.0 TUI Stack
    •    Library: Bubbletea (github.com/charmbracelet/bubbletea)
    •    Styling: Lipgloss (github.com/charmbracelet/lipgloss)
    •    Components: Bubbles (github.com/charmbracelet/bubbles) — list, viewport, textinput, spinner
    •    Entry point: cmd/tui/main.go

⸻

11.0.1 TUI Views
    •    Inbox view: scrollable thread list with unread indicators
    •    Thread view: message list within a thread
    •    Message reader: scrollable body with header details
    •    Compose view: multi-field form (To, Subject, Body)
    •    Account switcher: select active account or unified inbox

⸻

11.0.2 TUI State Rules
    •    All mail state lives in Go services / SQLite — TUI models hold only view state (cursor position, selected item, input buffers)
    •    Sync runs in a background goroutine; progress sent to TUI via tea.Cmd / tea.Msg

⸻

11.0.3 Keyboard Navigation
    •    j/k or arrow keys: move cursor
    •    Enter: open thread / message
    •    c: compose new message
    •    r: reply
    •    s: sync now
    •    q / Esc: back / quit

⸻

12. Frontend Architecture (Phase 3)

⸻

12.1 SPA Model
    •    Single entry point (index.html)
    •    No full page reloads
    •    Partial DOM updates only

⸻

12.2 Frontend Stack

Recommended:
    •    Alpine.js (lightweight)
    •    Tailwind CSS (existing)

Alternative:
    •    React (with strict constraints)

⸻

12.3 Frontend Responsibilities
    •    Render UI
    •    Capture user input
    •    Call Go functions
    •    Listen to events

⸻

12.4 State Rules

Layer    State Ownership
Go backend    YES
SQLite    YES
Frontend    NO (only UI state)


⸻

12. Communication Model

⸻

Commands (Frontend → Go)
    •    GetInbox()
    •    ReadMessage(id)
    •    SendMessage(data)
    •    Sync()

⸻

Events (Go → Frontend)
    •    inbox_updated
    •    message_loaded
    •    sync_progress
    •    draft_saved

⸻

13. Rendering Strategy
    •    Incremental updates only
    •    No full re-render
    •    Component-level updates

⸻

14. Offline Behavior

⸻

Available Offline
    •    Inbox list
    •    Cached messages
    •    Cached attachments

⸻

Not Available Offline
    •    Unsynced messages
    •    Remote attachments

⸻

15. Performance Requirements
    •    Inbox load < 300ms
    •    Message open < 200ms (cached)
    •    Sync latency < 5s

⸻

16. Risks
    •    Gmail API rate limits
    •    No schedule send API
    •    Sync complexity
    •    Data consistency issues

⸻

17. Key Design Principles
    1.    Backend-driven architecture
    2.    SQLite as source of truth
    3.    No frontend business logic
    4.    Event-driven updates
    5.    Offline-first design

⸻

18. Definition of Done (Phase 1)
    •    Account authentication works
    •    Sync works for last X days
    •    Inbox query is fast
    •    Messages readable offline
    •    Sending emails works
    •    Contacts available

⸻

19. Definition of Done (Phase 2)
    •    TUI launches and renders inbox list
    •    Thread and message navigation works
    •    Compose and reply flows work
    •    Sync runs in background and updates UI
    •    Keyboard shortcuts functional

⸻

20. Definition of Done (Phase 3)
    •    UI renders inbox
    •    Messages open correctly
    •    Compose works
    •    Events update UI correctly
    •    No frontend state bugs

⸻

21. Final Statement

The application is a backend-driven Gmail client where Go manages all state, synchronization, and business logic. The frontend is a lightweight rendering layer that reacts to backend events, ensuring a fast, reliable, and maintainable system.

⸻
