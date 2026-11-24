# Headless UI tests

Fyne `test` driver tests in `internal/ui/*_test.go`. No display, no network, no real backend.

## Display target

All UI tests run only on the MacBook Air 13" M4 default scaled display: 2560x1664 native, 2x,
so 1470x956 logical points; usable area 1470x918 (minus the ~38pt macOS menu bar).
Assumption: 13-inch, default scaling (not the 15-inch 1710x1107).
Headless fyne/test windows MUST use this size: Go constants `uxWindowW`/`uxWindowH` = 1470 x 918 (planned, to be added
in `internal/ui`; `withSize` defaults to them). Scale 1 in tests (see `docs/ux-tests/README.md` for the Xvfb choice).

Rule: do not test other resolutions.

## Run

    env -u DISPLAY go test ./internal/ui/ -count=1
    UPDATE_GOLDEN=1 env -u DISPLAY go test ./internal/ui/ -run TestName   # regenerate goldens
    env -u DISPLAY go test -race ./internal/ui/ -count=2                  # must report zero races (slow, ~3-5 min)

Goldens live in `internal/ui/testdata/*.png`. A mismatch writes the actual image to
`testdata/failed/` (untracked, delete it). Review regenerated PNGs by eye before committing.

## Helpers (`helpers_test.go`)

- `newTestUI(t, opts...)` returns `(*UI, *fakeBackend)`; options `withDark`, `withSize`,
  `withBackend`, `withEmpty`, `withNoAccounts`, `withNoCreds`.
- `fakeBackend`: in-memory `Backend` with canned data and per-method error injection (`fail`, `fire`).
  Guarded by `mu`: read autosave results through `savedDrafts()` (drafts and the ids passed to
  `SaveDraft`), `draftCount()` and `callLog()` rather than the raw fields. `SetAccountSettings`,
  `SetGlobalSettings` and `SetSyncConfig` arguments are recorded in `savedAccount`, `savedGlobal`,
  `savedSync`.
- `compflowTicks(t)`: replaces the autosave ticker (`newDraftTicker` in compose.go) with a manual one
  and returns `tick()`. The Fyne test driver runs `fyne.Do` inline on the calling goroutine, so a
  free-running ticker would read editor state while the test edits it. `tick()` returns only after
  the autosave pass has finished. Use it for any test that edits a compose window and expects
  (or forbids) an autosave.
- `freezeNow(t, goldenNow)` + `msAt(h)`: fixed clock (and UTC `time.Local`) for date-stable goldens.
- `golden(t, name, img)`: compare a canvas capture with `testdata/name.png`.
- `newWindow`, `typeKeys`, `findTapRow`, `buttonLabels`, `contains`: drive and inspect widgets.

## Conventions

Tests drive the UI through real widgets (tap, type, key events) and assert on visible state, then
capture a golden only where layout matters. Always freeze the clock before a golden; never use
`time.Now` in fixtures (use `msAt`). Goldens must not depend on host timezone or fonts beyond
Fyne's bundled theme. Keep one golden per distinct visual state; name it `area_state.png`.
Tests that document an app gap use `t.Skip` with the reason, so they flip on when fixed.

## Known skipped tests

- `TestComposeFlowSendShortcut`, `TestKeysCmdShortcuts`: test canvas cannot deliver canvas shortcuts.
- `TestComposeFlowOutboundAttachments`: `OutboundMsg` has no attachment field and compose has no attach UI.
- `TestScreenshot`: manual; set `MAILRAT_SHOT=/path/out.png`.
