# 23 — Shell type wrapping ExecSession with PTY sentinel framing

## Summary

Landed the persistent `Shell` type at the container root, built on top of `Runtime.ExecStream` (from sub-issue #22). A `Shell` wraps a PTY-attached `bash -i` (or any shell the caller passes via `ShellOptions.ShellPath`) and exposes `Run(ctx, cmd) ([]byte, int, error)` + `Close() error`. State — cwd, env, shell history, sourced rc files — persists across `Run` calls. Completes Objective #18 (Persistent Shell Foundation); unblocks Obj #20's `shell` built-in tool.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Framing strategy | Silent-prompt sentinel, not prompt-as-sentinel | The issue's literal "PS1=<sentinel>" framing produces two sentinels per Run (prompt after cmd + printf's own), breaking the "read until sentinel; next line is exit" contract. Setting `PS1=''` eliminates the prompt-side sentinel, leaving only the printf-emitted one. Revisited mid-planning after tracing byte streams. |
| PTY configuration | `stty -echo -opost` at prime | Echo off prevents stdin bounce-back contaminating stdout. `-opost` keeps `\n` from being transformed to `\r\n` on the slave, so line-based parsing is unambiguous without needing CR-tolerant matching. |
| Sentinel format | `tau.<uuid-v7>` via `github.com/google/uuid` | Matches the TAU ecosystem convention (`agent`, `kernel`, `orchestrate` all use `uuid.Must(uuid.NewV7()).String()`). v7's time-ordered prefix is nice for log correlation; the `tau.` prefix makes sentinels visually distinct. First direct external dep on the root container module — root's previous "zero deps" posture was incidental, not principled. |
| Concurrency contract | Serialize Run on `*Shell` via `sync.Mutex`; independent `*Shell` instances parallel | Interleaving stdin writes would scramble the framing protocol; serializing is the only safe option for a single bash process. Multiple bash processes (multiple `*Shell` instances) run in true parallel because each owns its own `ExecSession`. |
| Close-during-Run safety | `session.Close()` runs before `mu.Lock()` in `Close` | Original ordering (mu.Lock first, then session.Close) would deadlock when a concurrent `Close` hit an in-flight `Run` blocked on `readUntilSentinel`. Reversing the order lets `session.Close` unblock the pending read, letting `Run` release `mu`, letting `Close` proceed. Caught during test design and exercised by `TestShell_Close_DuringRun_NoDeadlock`. |
| ctx cancel semantics | Goroutine watches `ctx.Done()` and closes the session on cancel | Simple and correct: cancelling mid-Run tears down the shell. Callers needing finer-grained cancel per-Run should compose shorter-lived shells around a longer-lived context. Documented on `Run`'s godoc. |
| Output trim rule | Strip at most one trailing `\n` from Run output | printf's leading `\n` always inserts one framing blank line. Stripping exactly one removes the artifact; stripping at-most-one preserves caller-authored blank lines. Verified by `TestShell_Run_PreservesLegitimateBlankLines`. |
| CHANGELOG entry | Skipped (memory `feedback_dev_prerelease_timing.md`) | Obj #18 completes with this sub-issue, but the library isn't end-to-end runnable until Obj #19 + #20 land. Dev pre-release CHANGELOG bump deferred. |

## Files Modified

- `shell.go` (new) — `DefaultShellPath`, `ShellOptions`, `Shell`, `NewShell`, `(*Shell).Run`, `(*Shell).Close`, `prime`, `readUntilSentinel`, `generateSentinel`. Fully godoc'd.
- `go.mod` / `go.sum` (root) — added `github.com/google/uuid v1.6.0`.
- `tests/shell_test.go` (new) — 15 black-box unit tests using a fake `ExecSession` over `io.Pipe` pairs and a fake-bash goroutine that parses the sentinel from the prime payload. Covers defaults, option forwarding, basic output, non-zero exit, empty output, no-trailing-newline output, blank-line preservation, embedded sentinel substring, sequential runs, close idempotency, run-after-close, exit parse failure, and close-during-run deadlock safety.
- `docker/tests/shell_test.go` (new) — 10 integration tests against alpine:3.21's busybox ash (`ShellPath: "/bin/sh"`). Covers basic echo, exit code, cwd persistence, env persistence, PTY stderr merging, close idempotency, run-after-close, custom working dir, custom env, concurrent independent shells.
- `_project/README.md` — Phase 2 status note now "In Progress (Obj #18 Done)".
- `_project/phase.md` — Objective #18 flipped to Done.
- `_project/objective.md` — sub-issue #23 flipped to Done; architecture decision #3 rewritten to describe the silent-prompt approach and the reasoning behind the revision; acceptance criteria all checked.
- `.claude/CLAUDE.md` — `shell.go` added to the project-structure block; status note updated.

## Patterns Established

- **Silent-prompt PTY framing.** For programmatic wrappers of `bash -i` under PTY, `stty -echo -opost; PS1=''; PS2=''` is the canonical setup. Use printf with a UUID sentinel on the output side for frame boundaries. Don't try to use the shell's prompt as a frame boundary — it sits at the wrong granularity (once per command, not once per Run).
- **Ecosystem-consistent UUID convention.** Use `github.com/google/uuid` with `NewV7()` for any TAU ID or sentinel. Matches `agent`, `kernel`, `orchestrate`. Time-ordered IDs are valuable for logs and debugging; the trivial dep cost is well within the TAU ecosystem norm.
- **Close-before-Lock for concurrent teardown.** When a long-lived handle's `Close` needs to interrupt an operation holding the handle's mutex, do the teardown (the thing that unblocks the operation) BEFORE acquiring the mutex. Then the mutex acquisition is a quick post-interrupt cleanup, not the interrupt itself.

## Validation Results

- Root module: `go build ./... && go vet ./... && go test ./... -timeout 60s` → clean. 15 new unit tests + all prior tests pass.
- Docker sub-module: `go build ./... && go vet ./... && go test ./... -timeout 120s` → clean. 10 new integration tests + all prior tests pass (~12s total against live Docker daemon).
- `go mod tidy` is a no-op in both modules after the root module's `go get github.com/google/uuid@v1.6.0`.
- Acceptance criteria from issue #23: all checked ✓.
- Objective #18 acceptance criteria: all checked ✓.
