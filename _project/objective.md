# Objective #18 — Persistent Shell Foundation

**Parent:** [container#18](https://github.com/tailored-agentic-units/container/issues/18)
**Phase:** [Phase 2 — Agent Tool Bridge](phase.md) (`v0.2.0`)

## Scope

Add the streaming exec primitive that `Runtime.Exec` cannot satisfy, and build the `Shell` type that wraps it into a long-lived, state-preserving session. Every downstream consumer that needs an "agent operating as a user" experience — Obj #20's `shell` built-in tool, tools that depend on a persistent shell process (`mise`, `direnv`, sourced rc files) — goes through `Shell`, not through repeated one-shot `Exec` calls.

## Sub-issues

| # | Title | Repo | Depends | Status |
|---|-------|------|---------|--------|
| [#22](https://github.com/tailored-agentic-units/container/issues/22) | Runtime.ExecStream primitive and Docker implementation | container | — | Todo |
| [#23](https://github.com/tailored-agentic-units/container/issues/23) | Shell type wrapping ExecSession with PTY prompt sentinel framing | container | #22 | Todo |

## Architecture decisions

1. **Separate streaming primitive.** `Runtime.ExecStream(ctx, id, ExecStreamOptions) (*ExecSession, error)` — a distinct method on the Runtime interface (9 methods total). Returns a session handle exposing `Stdin`, `Stdout`, `Stderr`, `Wait`, `Close`. Keeps `Exec`'s one-shot `*ExecResult` contract untouched and matches OCI's `exec` vs. `attach` verb separation.

2. **New `ExecStreamOptions` type.** `Cmd []string`, `Env map[string]string`, `WorkingDir string`, `Tty bool`. Kept distinct from `ExecOptions` because the `AttachStdin/Stdout/Stderr` flags are meaningless when streams are always exposed via the session handle.

3. **Shell framing: PTY + prompt sentinel.** Run `bash -i` under a PTY; inject a UUID sentinel as `PS1`/`PS2`; each `Shell.Run` emits the command followed by `printf '\n<sentinel>\n%s\n' "$?"`; the framing layer reads stdout until the sentinel and parses the trailing exit code. Preserves shell history, sourced rc files, interactive prompts, and the vision's agent-as-user posture.

4. **TTY-mode stderr contract.** When `ExecStreamOptions.Tty=true`, the container process's stderr is merged onto stdout by the PTY. `ExecSession.Stderr` yields EOF immediately in TTY mode. Shell uses `Tty=true` and folds everything into its framing layer.

5. **Lifecycle semantics.** `ctx` passed to `Runtime.ExecStream` aborts the create/attach API calls only; once the session is live, `Close` is the only way to kill early and `Wait` blocks until natural exit. `Shell.Close` terminates the underlying `ExecSession` (SIGHUP via hijacked conn close) without touching the container. Multiple concurrent `Shell` instances per container are allowed — each owns its own `ExecSession`.

## Acceptance criteria (objective-level)

- [ ] Both sub-issues (#22, #23) merged
- [ ] `Runtime.ExecStream` live on the interface and implemented by the Docker sub-module
- [ ] `Shell` usable against a real Docker daemon: cwd persists, env persists, multiple instances can coexist on one container
- [ ] All unit and integration tests pass (`go build ./... && go vet ./... && go test ./... && (cd docker && go test ./...)`)
