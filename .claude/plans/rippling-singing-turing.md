# Issue #22 — Runtime.ExecStream primitive and Docker implementation

## Context

First sub-issue of Objective #18 (Persistent Shell Foundation), Phase 2 (v0.2.0).

Adds the streaming exec primitive that `Runtime.Exec` cannot satisfy. `Runtime.Exec` is one-shot: it returns a `*ExecResult` with captured stdout/stderr bytes *after* the process exits. A persistent shell (sub-issue #23) needs live bidirectional streams — stdin to send commands into a long-lived `bash -i`, stdout to read back output framed by a PS1 sentinel. That demands a session handle (`*ExecSession`) exposing `Stdin io.WriteCloser`, `Stdout io.Reader`, `Stderr io.Reader`, `Wait()` and `Close()`.

The architecture decision in `_project/objective.md` chose a separate `ExecStream` method over flagging `ExecOptions` — keeps `Exec`'s `*ExecResult` contract untouched and matches OCI's `exec` vs. `attach` verb separation.

## Files to modify / add

### Root module (`github.com/tailored-agentic-units/container`)

- `runtime.go` — add `ExecStream(ctx, id, opts) (*ExecSession, error)` to the `Runtime` interface (9 methods total). Godoc: `ctx` aborts the create/attach API calls only; once live, `Close` kills the session early, `Wait` blocks until natural exit.
- `exec_stream.go` — new file. Defines `ExecStreamOptions` and `ExecSession`. Godoc on `ExecSession.Stderr` documents the TTY-mode contract (merged into stdout, `Stderr` yields EOF).
- `tests/exec_stream_test.go` — new file. Zero-value and type-presence assertions only (no daemon).

### Docker sub-module (`github.com/tailored-agentic-units/container/docker`)

- `docker/exec_stream.go` — new file. Implements `ExecStream` on `dockerRuntime`, plus `halfCloser` (wraps `HijackedResponse` so `Stdin.Close` calls `CloseWrite`, not full conn close) and `eofReader` (yields EOF for TTY-mode `Stderr`).
- `docker/tests/exec_stream_test.go` — new file. Integration tests gated on `skipIfNoDaemon` (mirrors `docker/tests/io_test.go`).

## Design

### `ExecStreamOptions` (root)

```go
type ExecStreamOptions struct {
    Cmd        []string
    Env        map[string]string
    WorkingDir string
    Tty        bool
}
```

Kept distinct from `ExecOptions` — `AttachStdin/Stdout/Stderr` flags are meaningless when streams are always exposed via the session handle.

### `ExecSession` (root)

```go
type ExecSession struct {
    Stdin  io.WriteCloser
    Stdout io.Reader
    Stderr io.Reader

    wait  func() (int, error)
    close func() error
}

func (s *ExecSession) Wait() (int, error) { ... }
func (s *ExecSession) Close() error       { ... }
```

**Rationale for struct-with-callback-fields** (vs. an interface): the issue explicitly lists `Stdin`/`Stdout`/`Stderr` as fields on a struct. Callbacks let the Docker sub-module inject behavior without exporting runtime-specific types. Zero-value `Close` is a no-op (nil callback → return nil); zero-value `Wait` returns a descriptive error ("container: ExecSession not initialized") so tests can assert on it without panic.

### Docker `ExecStream` flow

1. `ContainerExecCreate` with `AttachStdin/Stdout/Stderr=true, Tty=opts.Tty` — honors `ctx`.
2. `ContainerExecAttach` — honors `ctx`. Returns `hr types.HijackedResponse`.
3. Build the `*ExecSession`:
   - **Stdin**: `&halfCloser{hr: hr}` — `Write` → `hr.Conn.Write`; `Close` → `hr.CloseWrite()`.
   - **TTY mode** (`opts.Tty=true`): Docker merges stderr onto stdout via the PTY.
     - `Stdout = hr.Reader`, `Stderr = eofReader{}`.
   - **Non-TTY mode**: spawn demux goroutine.
     - Create two `io.Pipe` pairs.
     - `Stdout`/`Stderr` = the read ends.
     - Goroutine: `stdcopy.StdCopy(stdoutPw, stderrPw, hr.Reader)`; on exit, `stdoutPw.CloseWithError(err) / stderrPw.CloseWithError(err)` (nil err → plain `Close`).
4. Shared state captured by `close` and `wait` closures:
   - `sync.Once` guarding close (idempotent).
   - `done chan struct{}` — closed on first Close; wakes polling loop.
   - `sync.WaitGroup` for the demux goroutine (non-TTY).
5. `close` closure: `closeOnce.Do` → `close(done)` → `hr.Conn.Close()` (terminates exec, unblocks StdCopy) → `wg.Wait()` for demux to exit → records `closeErr`.
6. `wait` closure: 50ms `time.Ticker`. Each tick: `ContainerExecInspect` — when `!Running`, return `ExitCode`. On `<-done`, do one final inspect and return the current exit code so `Close` short-circuits gracefully.

### Lifecycle semantics (per `runtime.go` godoc + `_project/objective.md` decision 5)

- `ctx` passed to `ExecStream`: aborts the `ContainerExecCreate`/`ContainerExecAttach` calls only. Returned error wraps `ctx.Err()` (matches the existing `Exec` pattern at `docker.go:178-184`).
- Once the session is returned, `ctx` has no further effect. Callers use `Close` for early termination.
- `Wait` blocks until natural process exit or until `Close` short-circuits it.

### Testing

Root module (`tests/exec_stream_test.go`):
- Compile-time interface assertion: `var _ interface { ExecStream(...) } = container.Runtime(nil)`.
- Zero-value `ExecSession{}`: `Stdin/Stdout/Stderr` nil; `Close()` returns nil; `Wait()` returns an error.
- Zero-value `ExecStreamOptions{}`: empty `Cmd/Env/WorkingDir`, `Tty=false`.

Docker integration (`docker/tests/exec_stream_test.go`) — reuses `skipIfNoDaemon`, `ensureImage`, `newRuntime`, `startSleeper` from existing helpers:
- `TestExecStream_StdinRoundTrip` — run `cat`, write `"hi\n"` to `Stdin`, call `Stdin.Close()`, read `"hi\n"` from `Stdout`, `Wait()==0`.
- `TestExecStream_ExitCodeZero` — run `true`; `Wait()` returns `(0, nil)`.
- `TestExecStream_ExitCodeNonZero` — run `sh -c "exit 7"`; `Wait()` returns `(7, nil)`.
- `TestExecStream_CloseTerminatesLongRunning` — run `sleep 30`, `Close()` returns under ~1s, subsequent `Wait()` returns promptly.
- `TestExecStream_CloseIdempotent` — two `Close()` calls in a row; no error.
- `TestExecStream_CtxCancelDuringStart` — cancelled `ctx` to `ExecStream`; error wraps `ctx.Err()` (match existing `TestExec_CtxCancel` pattern).
- `TestExecStream_NonTtyDemux` — `sh -c "echo out; echo err 1>&2"`; `Stdout` contains `out\n`, `Stderr` contains `err\n`.
- `TestExecStream_TtyMerged` — `Tty=true`, `sh -c "echo out; echo err 1>&2"`; `Stderr` yields EOF, `Stdout` contains both streams (ordering not asserted).

## Verification

End-of-task validation (matches the issue's final acceptance criterion and the project's gates):

```bash
go build ./...
go vet ./...
go test ./...
(cd docker && go build ./... && go vet ./... && go test ./...)
```

All must pass. Docker integration tests skip gracefully when the daemon is unreachable (`skipIfNoDaemon`).
