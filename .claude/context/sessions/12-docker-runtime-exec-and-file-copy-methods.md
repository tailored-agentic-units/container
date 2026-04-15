# 12 - Docker runtime Exec and file copy methods

## Summary

Replaced the three "not implemented in sub-issue #11" stubs in `docker/docker.go` with real implementations of `Exec`, `CopyTo`, and `CopyFrom`. The Docker runtime now supports one-shot command execution (with cancellation, exit-code capture, and per-stream attach), tar-stream file uploads (with auto-mkdir of parent dirs), and tar-stream file downloads (returning raw bytes via a small adapter that respects ctx cancellation on Reads). Sub-issue C (`Inspect` + manifest) remains the last open piece of Objective 3's library work.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Exec cancellation primitive | Close the hijacked connection on ctx.Done; goroutine drain channel | Docker SDK v28.5.2 has no `ContainerExecKill`; closing the hijacked conn is the conventional abort. The drain goroutine + select avoids leaking on cancel. |
| Drain-error wrapping | Drain goroutine's error reported separately from inspect error | Lets a future caller distinguish "stream tore" from "inspect failed". Same `%w` wrap so `errors.Is` still walks. |
| AttachStdin without a Stdin reader | Pass flag through to ExecCreate, immediately CloseWrite() | Phase 1 `ExecOptions` has no stdin source field. Allocating the pipe and EOF-ing it lets the daemon stay happy without us needing data. The field will be added when a caller needs it. |
| CopyTo parent-dir creation | `mkdir -p` via `r.Exec` (recursive call into the new method) | Reuses the runtime's own surface — no separate helper for shell-out. Side effect: CopyTo now requires the container to be running and to ship a POSIX shell + mkdir. Documented on the godoc. |
| CopyTo body buffering | `io.ReadAll` upfront, then tar-write with explicit `Size` | tar headers require Size before body bytes. Phase 1 callers move manifest-sized payloads (kilobytes), so full buffering is acceptable. Streaming would need a temp-file or known-size source. |
| CopyFrom tar unwrapping | Custom `tarFileReader` that delegates Read to `tar.Reader`, ctx-checks per Read | Caller asked for raw file bytes; the SDK returns a tar stream. Adapter is ~10 lines, ctx check honors the Runtime interface contract for streaming cancellation without spawning a goroutine. |
| Not-found detection | Wrap original docker error with `%w`; document `cerrdefs.IsNotFound` | docker/errdefs `Is*` are deprecated thin aliases over containerd/errdefs; existing code already uses cerrdefs. No new domain error since the issue scopes that out. |
| docker/errdefs vs containerd/errdefs | Confirmed full pivot: `docker/errdefs/is.go` is `var Is* = cerrdefs.Is*` aliases marked Deprecated. Only the constructor helpers (`errdefs.NotFound(err)`, etc.) and marker interfaces remain in docker/errdefs. | Validates last session's discovery from #11; reinforces the "use cerrdefs for detection" convention. |

## Files Modified

- `docker/docker.go` — Exec, CopyTo, CopyFrom implementations + `tarFileReader` helper + godoc on the three methods; new imports: `archive/tar`, `bytes`, `path`, `github.com/docker/docker/pkg/stdcopy`
- `docker/tests/io_test.go` — new file, 8 black-box tests
- `.claude/context/guides/.archive/12-docker-runtime-exec-and-file-copy-methods.md` — implementation guide moved here at closeout

## Patterns Established

- **Goroutine + select for streaming cancellation**: drain goroutine writes to a buffered error channel; main goroutine selects on ctx.Done() and the drain channel; on ctx fire, close the underlying stream to unblock the drain, wait for the drain to report, return the ctx error. Reusable for any future Docker SDK call that returns a hijacked connection (e.g., logs follow, attach to running container).
- **Method-internal reuse via `r.Exec`**: `CopyTo` calls `r.Exec` for parent-dir creation rather than introducing a private `mkdir` helper. Lower surface area, exercises the same exec path the test suite covers, and any cancellation/timeout semantics inherit automatically.
- **`ctx.Err()` at the top of Read**: minimal-effort way to honor "Cancelling ctx aborts the copy stream" on a returned `io.ReadCloser` without goroutine plumbing. Pattern for future stream-returning methods.
- **Test gotcha — empty hijacked stream EOFs immediately**: `Exec` with no `AttachStdout`/`AttachStderr` against a long-running command does NOT block on the drain — there's nothing to multiplex, so StdCopy returns immediately. Cancellation tests must attach at least one stream to keep the drain blocked while ctx fires. Captured this in `TestExec_CtxCancel`.

## Validation Results

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./tests/... -count=1` — 13/13 pass (5 lifecycle from #11 + 8 new I/O); ~4.3s wall time when daemon is reachable
- `go test -coverpkg=.../docker ./tests/...` — **78.4%** of statements in `docker` package; the gap is the still-stubbed `Inspect` method (3 lines). Will cross 80% after #13 lands the real `Inspect`.
- All tests skip cleanly via `skipIfNoDaemon` when Docker is unreachable; no test-environment churn beyond what #11 established.
