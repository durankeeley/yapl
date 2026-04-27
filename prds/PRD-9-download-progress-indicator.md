# PRD-9 - Download Progress Indicator

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** M
**Sprint:** Backlog
**Tags:** `feature`, `ux`, `download`, `archive`
**Created:** 2026-04-27

---

## 1. Problem & Context

Proton builds and the Steam Linux Runtime are multi-gigabyte downloads. During `setup`, YAPL prints `-> Acquiring Proton 'ge-proton-9'...` and then goes silent until the extraction finishes. On a slow connection this can take many minutes with no feedback, leaving users unsure whether the download is progressing or hung.

* **Current State:** `archive.go` downloads via `io.Copy` with no progress reporting. The user sees one line then silence.
* **Impact/Need:** Users cannot distinguish a slow download from a frozen process. Poor experience on LAN deployments where files are being pulled over the network.

## 2. Proposed Solution

* Wrap the `io.Copy` in `archive.go`'s download path with a progress-reporting reader that prints a running byte count to stdout.
* Display: `  12.4 MB / 650 MB (1.9%)` updated in-place using carriage return (`\r`), without printing newlines until complete.
* Show a final newline and total size when the download finishes.

## 3. Scope & Design Details

### Progress reader
* **Behavior/Rule:** A small `progressReader` struct wraps `io.Reader`, counts bytes, and writes a `\r`-prefixed line on each `Read` call.
* **Specifics:** Only update the display when at least 256 KB has been received since the last print — avoid flooding stdout when reads are tiny.

### Content-Length handling
* **Behavior/Rule:** If the HTTP response includes `Content-Length`, display `X MB / Y MB (Z%)`. If absent, display `X MB downloaded` with no percentage.
* **Specifics:** Parse `resp.ContentLength`; a value of `-1` means unknown.

### Non-TTY output
* **Behavior/Rule:** When stdout is not a terminal (e.g., piped to a log file), fall back to periodic line-based output (`\n` instead of `\r`), emitted every 10% or every 100 MB.
* **Specifics:** Detect with `term.IsTerminal(int(os.Stdout.Fd()))` from `golang.org/x/term` or a manual `syscall.Isatty` check.

### Extraction phase
* **Behavior/Rule:** After download, print `-> Extracting...` as a single line. No per-file progress for extraction (tar entry counts are not predictable for large archives).
* **Specifics:** The existing `ar.Extract` call is unchanged; only the download phase gets a progress indicator.

## 4. Execution & Milestones

- [ ] Write failing test: `TestProgressReader_CountsBytesCorrectly`
- [ ] Write failing test: `TestProgressReader_DoesNotPrintMoreFrequentlyThan256KB`
- [ ] Implement `progressReader` in `internal/archive/archive.go`
- [ ] Wire into the HTTP download path in `Archive.Extract`
- [ ] All tests pass (use a `bytes.Buffer` as the underlying reader in tests; no network required)
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/archive/archive.go` — `progressReader` type, wire into download path
* `internal/archive/archive_test.go` — new unit tests for the reader

### Risks & Considerations
* **CI log pollution:** `\r` in CI logs produces garbled output. The TTY detection guard prevents this.
* **External dependency:** `golang.org/x/term` is a lightweight stdlib-adjacent module. Acceptable to add. Alternatively, use `syscall.Isatty` to avoid a new module dependency.
* **Thread safety:** Progress printing is single-threaded (one download at a time); no mutex needed.
