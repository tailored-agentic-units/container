# 22 — Runtime.ExecStream primitive and Docker implementation

## Summary

Added the streaming exec primitive that `Runtime.Exec` cannot satisfy: `Runtime.ExecStream(ctx, id, opts) (*ExecSession, error)` is now on the interface (9 methods total), with a Docker sub-module implementation. `ExecSession` exposes live `Stdin io.WriteCloser`, `Stdout io.Reader`, `Stderr io.Reader` plus `Wait() (int, error)` and `Close() error`. First of two sub-issues on Objective #18 (Persistent Shell Foundation); unblocks sub-issue #23.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| `ExecSession` shape | Struct with exported I/O fields + exported `WaitFn`/`CloseFn` function-value fields, plus `Wait`/`Close` methods that nil-check the callbacks | Avoids a 5-parameter constructor while keeping the user-facing API method-shaped; zero value is safe (Close → nil, Wait → descriptive error) |
| Docker state layout | Unexported `execStream` struct owning `cli`, `execID`, `hr`, and lifecycle primitives (`wg`, `closeOnce`, `closeErr`, `done`); `wait`/`close`/`wireStreams`/`inspect` are methods on the struct | Eliminated five pieces of closure-captured state; `Runtime.ExecStream` reads as a short high-level sequence (create → attach → build session → return) |
| Stdin wrapper | `execStdin` type mapping `Write` → `hr.Conn.Write`, `Close` → `hr.CloseWrite()` | `Stdin.Close` signals EOF to the container process without tearing down stdout/stderr (TCP half-close) |
| TTY-mode stderr | `eofReader` yields `io.EOF` immediately; `Stdout = hr.Reader` | PTY merges stderr onto stdout; documented on `ExecStreamOptions.Tty` and `ExecSession.Stderr` godoc |
| Non-TTY demux | `wg.Go(func)` spawns a goroutine running `stdcopy.StdCopy` against `hr.Reader` into two `io.Pipe` pairs | Go 1.25 `WaitGroup.Go` idiom (global Go principles); `CloseWithError` propagates demux errors to the reader |
| Poll interval | File-level `const execInspectPollInterval = 50 * time.Millisecond` | Named constant over magic literal; no current need for per-call tuning (see tradeoff below) |
| `Wait` short-circuit | `done chan struct{}` closed by `close()`; `select` picks between `<-done` and `<-ticker.C`; either path returns via `inspect()` helper | `Close` wakes `Wait` immediately; `inspect()` dedupes the `ContainerExecInspect` call across both branches |
| CHANGELOG entry | Skipped | Memory `feedback_dev_prerelease_timing.md`: defer dev pre-release tags until library is end-to-end runnable. Obj #18 isn't complete until #23 lands. |

## Files Modified

- `runtime.go` — added `ExecStream` to the `Runtime` interface (9 methods total).
- `exec.go` (new) — `ExecStreamOptions` struct and `ExecSession` struct with `Wait`/`Close` methods and exported `WaitFn`/`CloseFn` callback fields.
- `docker/exec.go` (new) — `ExecStream` method on `dockerRuntime`, `execStream` struct, `execStdin` / `eofReader` helpers, and the `execInspectPollInterval` constant.
- `tests/exec_test.go` (new) — compile-time interface assertion + zero-value and callback-invocation tests.
- `tests/registry_test.go` — extended `stubRuntime` with an `ExecStream` method to satisfy the expanded `Runtime` interface.
- `docker/tests/exec_test.go` (new) — 8 integration tests (stdin round-trip, exit code zero/non-zero, Close terminates / idempotent, ctx cancel during start, non-TTY demux, TTY merged).
- `_project/README.md` — updated the `Runtime` interface code block.
- `_project/objective.md` — marked sub-issue #22 Done.
- `.claude/CLAUDE.md` — added `exec.go` entries to the project-structure block; updated the Phase 2 files note.

## Patterns Established

- **Unexported struct for runtime-specific session state.** When a runtime implementation needs to hand back a user-facing handle with lifecycle callbacks, own the state on an unexported struct with methods (not closures). The callbacks on the user-facing struct become method values (`es.wait`, `es.close`).
- **TCP half-close as the Stdin.Close contract.** When a runtime exposes a hijacked bidirectional conn as the stdin handle, wrap it so `Close` performs a write-side shutdown only. Lets callers signal EOF without tearing down the output streams.
- **Per-file named `const` for tunable-but-not-configurable values.** Over a magic literal for self-documenting intent; over a package-level `var` when no caller needs runtime tuning (tests can be updated to monkey-patch a `var` later if a real need emerges).

## Validation Results

- `go build ./...`, `go vet ./...`, `go test ./...` clean on the root module (`tests/` package, all existing + 5 new tests pass).
- `(cd docker && go build ./... && go vet ./... && go test ./...)` clean (`docker/tests/` package, all existing + 8 new `TestExecStream_*` integration tests pass).
- `go mod tidy` is a no-op in both modules.
- One flake observed on the unrelated pre-existing `TestExec_NonZeroExit` during the first full run; re-ran 3x in isolation and full-suite once, all green.
