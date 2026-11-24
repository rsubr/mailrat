# RAM notes

Measure: `MAILRAT_MEMLOG=1` logs Go heap stats every 5s; `MAILRAT_HEAPPROF=/path` also writes a heap profile.
Setup: Xvfb 1470x956, FYNE_SCALE=1, synthetic profile of 2000 inbox threads with ~90 KB bodies (216 MB db), no network, idle.
Software GL (llvmpipe) inflates RSS under Xvfb (libLLVM/gallium ~35 MB file-backed, ~30 MB anon); a Mac does not pay it.

| | Before | After |
|---|---|---|
| Go heapInuse | 25.2 MB | 24.8 MB |
| Go sys | 40.4 MB | 40.4 MB |
| Heap released to OS (75 s) | 2.9 MB | 6.9 MB |
| Anonymous RSS (75 s) | 84.1 MB | 80.4 MB |
| Total RSS | ~143 MB (mostly Mesa) | 86-144 MB (file-backed Mesa pages vary run to run) |

Heap profile: ~80% of the Go heap is Fyne text-face loading (go-text/typesetting font tables for the bundled Noto fonts),
not mail data. Lists are paged (50-60 rows) and bodies live only in SQLite, so message count barely moves RSS.

Changes: SQLite keeps at most 1 idle connection (30 s idle timeout, frees the 2 MB page cache per extra conn);
`debug.FreeOSMemory()` every minute. Existing: GC 25%, 100 MB soft limit, cache_size 2 MB, mmap 0, temp_store FILE.

Ideas not done: custom theme font set without bold-italic/emoji fallbacks (~5-10 MB, visible regression risk);
smaller `maxRemoteImageWidth` (1200 -> 900); measuring a real thread-open/image-heavy session on macOS.
