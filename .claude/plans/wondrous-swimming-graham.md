# Plan — Obj #18, sub-issue #23: Shell type with PTY sentinel framing

## Context

Sub-issue #23 is the second of two on Objective #18 (Persistent Shell Foundation). The first (#22) landed `Runtime.ExecStream` and `ExecSession`; this one builds a long-lived `Shell` on top that preserves cwd, env, and shell history across `Run` calls. Downstream, Obj #20's `shell` built-in tool wraps this type.

**Design posture.** The agent is never embedded in the container — it lives wherever its provider lives and reaches into the container through the tool API we're building. `Shell` is the native Go API that both the agent tool surface (Obj #20) and human callers drive directly. That means the public API must feel natural, and the framing internals must not leak into user-visible behavior.

### Framing strategy (the key decision)

The issue spec describes "prompt sentinel framing": set `PS1=<sentinel>` so every prompt is a frame boundary. That elegant idea breaks once you realize bash prints a prompt **after every command**, so a single `Run` (which issues `<cmd>` plus a `printf` to capture `$?`) emits two prompt-sentinels, only the second of which is the actual frame boundary. Papering over that with `$'\n'`-terminated prompts and compound-command tricks works but layers workarounds on top of an abstraction that doesn't fit our programmatic driver's needs.

The cleaner framing is: **use the PTY honestly, but stop using the prompt as a framing signal.** Concretely:

1. Keep `Tty: true` and `bash -i` — tools inside that check `isatty()` (colored `ls`, paging `git log`, etc.) continue to behave as they would for a human user. This is the vision-level "agent as user" requirement.
2. At startup, disable terminal echo (`stty -echo`) so what we write to stdin doesn't bounce back onto stdout.
3. At startup, set `PS1=''` and `PS2=''` so the interactive prompt emits zero bytes between commands. The interactive shell is still an interactive shell (rc files sourced, readline initialized, history written) — it just doesn't visually announce itself at every turn, which we don't need because we're not a human reader.
4. Frame each `Run` with ONLY the printf-emitted sentinel. The framing payload is exactly the issue spec's: `<cmd>\nprintf '\n<sentinel>\n%s\n' "$?"\n`. Parse by reading stdout until the sentinel line, then reading the exit code from the next line.

With `PS1=''` there is no competing sentinel in the stream — the prompt writes no bytes. The framing bytes produced per Run are exactly `<cmd_output>\n<sent>\n<exit>\n`, and the parse is the literal reading of the issue spec. No compound-command hack, no `$'...\n'` hack — the abstraction fits.

## Files to create / modify

| File | Kind | Phase | Purpose |
|------|------|-------|---------|
| `shell.go` | new (root) | Implementation guide (dev) | `Shell`, `ShellOptions`, `NewShell`, `(*Shell).Run`, `(*Shell).Close`, `DefaultShellPath` const |
| `tests/shell_test.go` | new (root) | Phase 5 (AI) | Black-box unit tests using a fake `ExecSession` over `io.Pipe` pairs |
| `docker/tests/shell_test.go` | new (docker) | Phase 5 (AI) | Integration tests gated on `skipIfNoDaemon` — cwd/env persistence, concurrent shells, close semantics |
| `_project/README.md` | update | Closeout (AI) | Note that `shell.go` now exists (the current README lists it as Phase 2 work) |
| `_project/objective.md` | update | Closeout (AI) | Flip sub-issue #23 status from Todo → Done |
| `.claude/CLAUDE.md` | update | Closeout (AI) | Add `shell.go` to the project-structure block |

No changes to `runtime.go`, `container.go`, `exec.go`, or either `go.mod`.

## API surface (root `shell.go`)

```go
// DefaultShellPath is used when ShellOptions.ShellPath is empty.
const DefaultShellPath = "/bin/bash"

// ShellOptions carries the parameters for NewShell.
type ShellOptions struct {
    WorkingDir string            // forwarded to ExecStream
    Env        map[string]string // forwarded to ExecStream
    ShellPath  string            // defaults to DefaultShellPath
}

// Shell wraps an ExecSession running an interactive shell, preserving cwd,
// env, and shell history across successive Run calls. The session is
// PTY-attached so tools inside see a real terminal; framing is handled by
// sentinel markers emitted via printf, not by the shell's prompt.
type Shell struct { /* unexported state */ }

func NewShell(ctx context.Context, rt Runtime, containerID string, opts ShellOptions) (*Shell, error)

// Run executes cmd in the shell and returns the captured stdout (stderr is
// merged onto stdout in PTY mode), the shell's exit code for cmd, and an
// error. Run serializes concurrent callers on one *Shell — interleaving
// stdin writes would scramble the framing protocol. Cancelling ctx during
// Run closes the shell; callers needing finer-grained cancellation should
// compose at a higher level.
func (s *Shell) Run(ctx context.Context, cmd string) (stdout []byte, exitCode int, err error)

// Close terminates the underlying ExecSession and releases resources.
// Idempotent. Safe to call concurrently with Run; the in-flight Run returns
// an error when the session tears down.
func (s *Shell) Close() error
```

## Internal state

```go
type Shell struct {
    sess     *ExecSession
    sentinel string
    reader   *bufio.Reader // wraps sess.Stdout

    mu        sync.Mutex
    closed    bool
    closeOnce sync.Once
    closeErr  error
}
```

## Constructor (`NewShell`)

1. Resolve `shellPath` — default `DefaultShellPath` if empty.
2. Generate sentinel: 16 bytes from `crypto/rand`, hex-encoded, wrapped as `__TAU_<hex>__` (32 hex chars = 128 bits of entropy).
3. Call `rt.ExecStream(ctx, containerID, ExecStreamOptions{Cmd: []string{shellPath, "-i"}, Env: opts.Env, WorkingDir: opts.WorkingDir, Tty: true})`.
4. Wrap `sess.Stdout` in `bufio.NewReader`.
5. Prime: write ONE compound line to `sess.Stdin`:
   ```
   stty -echo; PS1=''; PS2=''; printf '\n<sentinel>\n'
   ```
   (All four steps as a single bash command. `printf` emits the prime marker.)
6. `readUntilSentinel(io.Discard)` — consume any pre-prime welcome / default-prompt / echoed-command noise up to the first sentinel line.
7. On error anywhere in priming, `sess.Close()` and wrap the error with `fmt.Errorf("container: prime shell: %w", err)`.

## `Run(ctx, cmd)`

Holds `s.mu` for the entire call; checks `s.closed` first and returns a descriptive error if set.

1. Write ONE compound line: `<cmd>\nprintf '\n<sentinel>\n%s\n' "$?"\n`
   - Two logical bash commands separated by `\n`. Between them bash would have printed its prompt — but PS1='' so zero bytes are emitted. This is the exact framing the issue spec describes, and it now works because the prompt is silent.
2. `readUntilSentinel(&outBuf)` — reads `bufio.Reader.ReadString('\n')` in a loop; compares `strings.TrimRight(line, "\r\n")` to `s.sentinel`; on exact match, returns; otherwise appends the raw line (including its `\n`) to `outBuf`.
3. Strip at most one trailing `\n` from `outBuf` — printf's leading `\n` always introduces exactly one framing blank line (it forces the sentinel onto its own line even if `<cmd>` output had no trailing newline). Stripping one `\n` cleanly removes the framing artifact; stripping AT MOST one preserves legitimate blank lines in command output.
4. Read one more line for the exit code; `strconv.Atoi(strings.TrimRight(line, "\r\n"))`; on parse failure return `fmt.Errorf("container: parse exit code %q: %w", line, err)`.
5. Return `(outBuf.Bytes(), exitCode, nil)`.

Ctx handling: launch a small goroutine on entry that watches `ctx.Done()` and calls `s.sess.Close()` on cancel. That's the simplest correct semantics — cancellation tears down the shell, any in-flight read unblocks with EOF, Run surfaces the error. Caller can hold a longer-lived shell and gate individual Runs with short-lived `ctx`s by reconnecting on cancel if needed. We document this clearly on `Run`'s godoc.

## `Close()`

```go
func (s *Shell) Close() error {
    s.closeOnce.Do(func() {
        s.mu.Lock()
        s.closed = true
        s.mu.Unlock()
        s.closeErr = s.sess.Close()
    })
    return s.closeErr
}
```

`sync.Once` idempotency mirrors the pattern in `docker/exec.go`. Closing the underlying session tears down the hijacked conn, which closes the stdout pipe; any in-flight `ReadString` returns io.EOF, so `Run` unblocks without needing an explicit drain goroutine.

## Testing (Phase 5 — AI responsibility, NOT in the implementation guide)

The implementation guide the developer executes covers `shell.go` only. The two test files below are written by the AI in Phase 5 after the developer's Phase 4 implementation lands. They're documented here for design-alignment awareness, not as developer-executable steps.

### Unit tests (`tests/shell_test.go`)

Black-box, no Docker. A **fake runtime** and a **fake-bash goroutine** drive the protocol end-to-end over `io.Pipe` pairs.

```go
// Two pipes: stdin (shell writes → fake-bash reads) and stdout (fake-bash writes → shell reads).
stdinR, stdinW := io.Pipe()
stdoutR, stdoutW := io.Pipe()

sess := &container.ExecSession{
    Stdin:   stdinW,
    Stdout:  stdoutR,
    Stderr:  eofReader{},
    WaitFn:  func() (int, error) { <-done; return 0, nil },
    CloseFn: func() error { stdinW.Close(); stdoutW.Close(); closeOnce(&done); return nil },
}
```

The fake-bash goroutine reads the priming line from stdin, extracts the sentinel by regex-matching `printf '\\n(.+)\\n'` in the prime payload, writes `\n<sentinel>\n` to stdout. Then for each subsequent `<cmd>\nprintf '\n<sentinel>\n%s\n' "$?"\n` from stdin, it emits a test-scripted `<canned_output><sentinel>\n<canned_exit>\n` response.

Test cases:

| # | Name | Asserts |
|---|------|---------|
| 1 | `TestNewShell_DefaultsShellPath` | fake-runtime receives `Cmd: []string{"/bin/bash", "-i"}` when `ShellOptions.ShellPath` is empty |
| 2 | `TestNewShell_CustomShellPath` | explicit `ShellPath: "/bin/sh"` → fake-runtime receives `["/bin/sh", "-i"]` |
| 3 | `TestNewShell_ForwardsEnvAndWorkingDir` | `Env` and `WorkingDir` propagate to `ExecStreamOptions` |
| 4 | `TestShell_Run_BasicOutput` | canned `hello\n` + exit 0 → Run returns `[]byte("hello\n"), 0, nil` |
| 5 | `TestShell_Run_NonZeroExit` | canned exit 7 → returned |
| 6 | `TestShell_Run_EmptyOutput` | canned empty output + exit 0 → Run returns `[]byte{}, 0, nil` (printf's leading `\n` stripped) |
| 7 | `TestShell_Run_EmbeddedPartialSentinel` | canned output contains a substring of the sentinel on a line — parsing does NOT terminate early |
| 8 | `TestShell_Run_MultipleInSequence` | two Runs over one Shell — both return scripted results in order |
| 9 | `TestShell_Close_Idempotent` | two Close calls — second returns same error; fake CloseFn invoked once |
| 10 | `TestShell_Run_AfterClose` | Run after Close → descriptive error referencing "closed" |
| 11 | `TestShell_Run_ExitParseFailure` | fake-bash emits non-numeric exit line → Run returns parse error |

Helpers live inline in the test file; no separate testutil package.

### Integration tests (`docker/tests/shell_test.go`)

Same `newRuntime(t)` + `startSleeper(t, rt)` scaffolding as `exec_test.go`. Each test uses `t.Cleanup(func() { _ = sh.Close() })`.

| # | Name | Asserts |
|---|------|---------|
| 1 | `TestShell_Run_CwdPersists` | `cd /tmp` then `pwd` → `/tmp\n` |
| 2 | `TestShell_Run_EnvPersists` | `export FOO=bar` then `echo $FOO` → `bar\n` |
| 3 | `TestShell_Run_ExitCodeNonZero` | `false` → exit 1 |
| 4 | `TestShell_Run_StderrMergedOntoStdout` | `echo err 1>&2` captured in Run's stdout output (PTY merges stderr onto stdout) |
| 5 | `TestShell_Close_Idempotent` | two Close calls both return nil |
| 6 | `TestShell_Close_RunAfterClose` | Run post-Close returns error |
| 7 | `TestShell_Concurrent_IndependentShells` | two `Shell` instances on one container each set a distinct env var; each sees only its own value |
| 8 | `TestShell_CustomWorkingDir` | `NewShell` with `WorkingDir: "/tmp"` → first `pwd` yields `/tmp\n` |

**Image compatibility.** `alpine:3.21` ships busybox `ash`, not bash. Tests pass `ShellOptions{ShellPath: "/bin/sh"}` explicitly to stay on the existing image. Confirmed compatibility: busybox `sh -i` supports `stty -echo`, `PS1=''`, `printf '\n%s\n'`, and `$?` — all POSIX. The unit tests cover the `DefaultShellPath = "/bin/bash"` branch without needing an image.

## Validation

From the issue acceptance criteria:

```bash
go build ./... && go vet ./... && go test ./... && (cd docker && go test ./...)
```

Additionally: `go mod tidy` is a no-op in both modules.

## Closeout deltas

- `_project/README.md`: update the "shell.go appears in package layout but is Phase 2 work" note.
- `_project/objective.md`: flip sub-issue #23 to Done.
- `.claude/CLAUDE.md`: add `shell.go` entry.
- `CHANGELOG`: per `feedback_dev_prerelease_timing.md`, **skip** the dev pre-release CHANGELOG entry. Obj #18 isn't end-to-end runnable yet; the CHANGELOG bump is deferred.
- Session summary + guide archive per standard workflow.
- PR with `Closes #23`.
