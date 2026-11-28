# Manual test checklist

Tick each item. "Mac" marks items that need macOS verification.

## Reading
- [ ] Select text in a message body (drag, double/triple click); Ctrl/Cmd+C copies. Paste into a rich editor: formatting kept (Mac only; Linux/Windows copy plain text).
- [ ] Dark/light mail toggle: sun button top-right of the reader, and the `v` key. Check a dark-styled and a light newsletter.
- [ ] Mojibake / HTML entities (`&amp;`, `Â`, `â€™`) render as clean text in list, snippet and body.
- [ ] Tooltips show on toolbar, sidebar and compose buttons.

## List
- [ ] Bulk toolbar: tick several rows, archive/trash/mark act on all of them.
- [ ] Archive shows an undo toast; Undo restores the thread.
- [ ] Empty folder shows an empty state; kill network and refresh to see the error state.

## Compose
- [ ] Reply and Forward: quoted text is collapsed; expand it, edit, collapse again.
- [ ] Attachments: picker button, drag-drop a file onto the compose window, remove a chip.
- [ ] Forward carries the original attachments.
- [ ] Attach over 25MB total: a clear limit message appears and the file is not added.
- [ ] Reply-To is honoured on Reply. Check the sent mail's In-Reply-To/References headers (Show original in Gmail).
- [ ] Close compose with typed text: the draft is flushed (appears in Drafts, reopens with content).
- [ ] Contacts autocomplete in To/Cc/Bcc, including "other contacts" (people you only emailed).
- [ ] Editor responsiveness with a long body and long quote (typing, scrolling, selection).

## App
- [ ] Esc closes dialogs (settings, account, compose confirm).
- [ ] Settings: sync frequency changes and persists after restart.
- [ ] Add Account: complete OAuth in the browser, or paste the redirect URL when the browser is remote.
- [ ] Contacts page: search, open, edit.

## Added later
- [ ] Reader toolbar: Star and Labels buttons toggle star and open the label picker for the open conversation.
- [ ] List header filter button opens the filter menu; choosing an entry filters the list, clearing restores it.
- [ ] Author backgrounds (banners, coloured cells) are painted and readable in both mail modes (v toggles mail mode).
- [ ] Rich copy on Linux (X11) and Windows: copy from a mail body, paste into a rich editor; formatting and links survive.
- [ ] Rich paste in compose: Cmd/Ctrl+V keeps formatting, Shift+V pastes plain text.
- [ ] Reply-To and References/In-Reply-To are set on replies (Show original in Gmail).
- [ ] Accounts race fix: no visible change; adding, removing and switching accounts still works.

## Known limits
- Banner/background images and CSS backgrounds in mail are not painted.
- A selection cannot span multiple messages in a thread.
- No drag-over highlight when dragging a file onto compose.
- Side-by-side layout cells stack vertically; grid-table cells are not painted.
- No inline highlight or background-image painting.
- Wayland-only sessions copy plain text only.
- macOS clipboard read/write is untested; Windows clipboard has not been run.
- X11 rich copy over 256 KB falls back to plain text.
- Messages synced before the migration use the old reply headers until they are fetched again.

## Mac build
`./build.sh app` then `open MailRat.app` (see README, "Prerequisites (macOS)"). Needs cgo, so build natively on the Mac.
