# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

## UX testing

To run and click through the app under Xvfb on Linux, see `docs/ux-tests/README.md` (`scripts/ux/`). All UI tests target one display only (MacBook Air 13" M4, 1470x918 usable); see "Display target" in that file and `docs/ux-tests/headless.md`; do not test other resolutions.

## Release procedure

When the user asks for a new release, run these steps in order. Stop and report on any failure.

1. Commit all intended changes with a conventional-style message (`feat:`, `fix(ui):`, ...) ending with the attribution lines from the session reminder.
2. Push to a branch named `release/vX.Y.Z` (as in past PRs, e.g. `release/v0.3.10`).
3. Open the PR against `main`: `gh pr create --base main`. The local git config shows `dev` as the main branch; the user wants release PRs to `main`, so always pass `--base main`. The PR build runs on PRs to `main`.
4. Merge it: `gh pr merge`.
5. Tag the merge commit on `main` as `vX.Y.Z` (plain `vX.Y.Z` style, see `git tag`) and push the tag. The tag must be reachable from `main` or CI rejects it.
   - Bugfix or minor release: bump Z.
   - Large feature release: bump Y and reset Z to 0.
   - If an X bump seems implied, ask the user first.
6. Wait for the tag-triggered workflow (`.github/workflows/build-macos.yml`: build, release, update-tap) to finish: `gh run list` / `gh run watch`. If any job fails, report and stop. Delete nothing.
7. Only after CI succeeds, delete OLD release artefacts, never the new release's:
   - Run `gh release delete <old-tag> --yes` for each old GitHub release. NEVER pass `--cleanup-tag`. Alternatively `gh release delete-asset <tag> <asset>` removes only the assets.
   - Never delete git tags. Verify with `git ls-remote --tags origin` that all old tags and the new tag remain.
   - The Homebrew cask in `rsubr/homebrew-mailrat` points at the newest release only, so deleting older releases is safe.
