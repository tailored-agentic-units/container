# Objective #18 Planning — Persistent Shell Foundation

## Context

Phase 2 (Agent Tool Bridge, target `v0.2.0`) is active; Obj #18 is the lynchpin. The phase-planning session (`.claude/plans/deep-mixing-snowglobe.md`) already committed the shape:

- New `Runtime.ExecStream` method returning an `*ExecSession` handle
- `Shell` type wrapping the session into a cwd/env/history-preserving interactive surface
- Two sub-issues on `tailored-agentic-units/container`, assigned to milestone `Phase 2 - Agent Tool Bridge` (#2)

The persistent shell is the piece the agent-surface Objective (#20) plugs into as the `shell` built-in tool — without it, "agent operates as a user" degrades back to per-call `Exec`. Obj #18 and Obj #19 are parallel-safe (no file overlap).

No `_project/objective.md` exists — this is a fresh objective planning session, no Step 0 transition closeout.

## Resolved Design Decisions

1. **Shell framing: PTY + prompt sentinel.** Run `bash -i` under a PTY; inject a unique sentinel (UUID) as `PS1` / `PS2`; each `Shell.Run` emits the command followed by `; printf '<sentinel>\n%s\n' "$?"`; the framing layer reads stdout until the sentinel, extracts the trailing exit code. Preserves shell history, sourced rc files, interactive prompts, and the vision's agent-as-user posture. Chosen over non-PTY + output delimiters despite implementation complexity — framing is standard art (Ansible, Fabric, paramiko all use this pattern).

2. **Streaming options type: new `ExecStreamOptions`.** Separate struct with `Cmd []string`, `Env map[string]string`, `WorkingDir string`, `Tty bool`. The `AttachStdin/Stdout/Stderr` flags on the current `ExecOptions` are meaningless in streaming mode — all three streams are always exposed via the session handle. Keeping the types distinct keeps call-site semantics obvious.

3. **TTY-mode stderr contract.** When `ExecStreamOptions.Tty=true`, the container process's stderr is merged onto stdout by the PTY (standard Unix behavior). `ExecSession.Stderr` returns a reader that yields EOF immediately in TTY mode. Document this explicitly on both types. Shell uses `Tty=true` and folds everything into its framing layer.

4. **`Shell.Close` semantics.** Kill the underlying `ExecSession` (which closes the hijacked conn, causing the PTY-attached bash to receive SIGHUP), then drain the streams. The container itself is untouched. Multiple concurrent `Shell` instances per container are allowed — each owns its own `ExecSession`.

5. **`ExecSession.Close` vs `ctx` cancellation.** Cancelling the `ctx` passed to `Runtime.ExecStream` aborts the attach/create API calls only (mirrors existing `Exec` semantics). Once the session is live, `Close()` is the only way to kill early; `Wait()` blocks until natural exit. This matches the `io.ReadCloser` pattern already used by `CopyFrom`.

## Sub-Issue Decomposition

Two sub-issues. Both on `tailored-agentic-units/container`, labeled `feature`, issue type `Task`, milestone `Phase 2 - Agent Tool Bridge`.

### 18A — `Runtime.ExecStream` primitive + Docker implementation

**Depends on:** none (sits on Phase 1 foundation).

**Scope:**

- **Root module:**
  - New `exec_stream.go`: `ExecStreamOptions` struct and `ExecSession` struct with `Stdin io.WriteCloser`, `Stdout io.Reader`, `Stderr io.Reader`, `Wait() (int, error)`, `Close() error`. Document TTY-mode stderr contract on both types.
  - `runtime.go`: add `ExecStream(ctx context.Context, id string, opts ExecStreamOptions) (*ExecSession, error)` to the `Runtime` interface (expands to 9 methods). Godoc: document cancellation semantics (ctx aborts create/attach only; `Close` kills live session; `Wait` blocks until natural exit).
  - `tests/exec_stream_test.go`: type-level tests (zero-value behavior, method presence) — integration coverage lives in the docker module.

- **Docker sub-module:**
  - Implementation on `dockerRuntime`: `ContainerExecCreate` with `AttachStdin/Stdout/Stderr=true` and `Tty=opts.Tty` → `ContainerExecAttach` → wrap the hijacked `HijackedResponse` into an `ExecSession`.
    - TTY mode: `Stdout = hr.Reader`, `Stderr = io.LimitReader(eof, 0)` equivalent (reader that EOFs immediately).
    - Non-TTY mode: spawn a demux goroutine; `Stdout` and `Stderr` are the read ends of `io.Pipe` pairs fed by `stdcopy.StdCopy(stdoutW, stderrW, hr.Reader)`.
  - `Wait`: poll `ContainerExecInspect` (or drain until EOF then inspect once) to return the exit code. Poll interval: 50ms — matches Docker SDK idioms.
  - `Close`: close the hijacked conn (terminates the attached process), drain remaining goroutines, close pipe writers.
  - Place in `docker/exec_stream.go` to keep `docker/docker.go` from growing further.
  - `docker/tests/exec_stream_test.go`: integration tests gated on `skipIfNoDaemon` — basic echo round-trip via stdin/stdout, exit code via `Wait`, `Close` kills early, `ctx` cancel aborts attach.

**Critical files modified:**
- `/home/jaime/tau/container/runtime.go` — interface extension
- `/home/jaime/tau/container/exec_stream.go` — new file (types)
- `/home/jaime/tau/container/docker/docker.go` — no changes needed; impl lives in new file
- `/home/jaime/tau/container/docker/exec_stream.go` — new file (impl)
- `/home/jaime/tau/container/tests/exec_stream_test.go` — new file (type tests)
- `/home/jaime/tau/container/docker/tests/exec_stream_test.go` — new file (integration)

**Acceptance criteria:**
- [ ] `Runtime` interface includes `ExecStream`; all implementations compile
- [ ] `ExecSession` exposes all five documented members; godoc documents TTY-mode stderr contract
- [ ] Docker impl round-trips stdin → stdout (e.g., pipe `echo hi` via stdin, read `hi\n` on stdout)
- [ ] `Wait` returns correct exit code for exit 0 and non-zero exit
- [ ] `Close` terminates a long-running process and is safe to call multiple times
- [ ] `ctx` cancellation during `ExecStream(...)` returns an error wrapping `ctx.Err()`
- [ ] Non-TTY mode demultiplexes stdout/stderr via `stdcopy`; TTY mode merges to stdout
- [ ] `go build ./... && go vet ./... && go test ./... && (cd docker && go test ./...)` all pass

### 18B — `Shell` type at root

**Depends on:** 18A merged (needs `ExecStream` and `ExecSession`).

**Scope:**

- **Root module:**
  - New `shell.go`: `Shell` type, `NewShell(ctx, rt, containerID, opts ShellOptions) (*Shell, error)`, `Shell.Run(ctx, cmd string) (stdout []byte, exitCode int, err error)`, `Shell.Close() error`.
    - `ShellOptions`: `WorkingDir string`, `Env map[string]string`, `ShellPath string` (default `/bin/bash`).
    - Constructor: calls `rt.ExecStream` with `Tty=true`, `Cmd=[shellPath, "-i"]`, `Env`, `WorkingDir`; primes the session by writing `PS1=<sentinel>\nPS2=<sentinel>\n` and reading until first sentinel to synchronize.
    - `Run`: writes `<cmd>; printf '\n<sentinel>\n%s\n' "$?"\n` to `ExecSession.Stdin`; reads stdout until sentinel; parses trailing exit-code line; returns everything before the sentinel as stdout.
    - `Close`: calls `ExecSession.Close()`, drains any remaining bytes from stdout.
  - Sentinel: a UUID generated per `Shell` via `crypto/rand` to avoid collisions with legitimate output.

- **Tests:**
  - `tests/shell_test.go`: unit tests using a fake `ExecSession` (backed by `io.Pipe`s) to exercise the framing/parsing logic without a container. Cover: command output parsing, exit code extraction, multi-line output, output containing near-sentinel strings, `Close` idempotency.
  - `docker/tests/shell_test.go`: integration tests — cwd persists across `Run` (`cd /tmp; pwd` → two Runs), env persists (`export X=1; echo $X`), history (sanity-check `history` command returns prior commands), `Close` terminates session without affecting container.

**Critical files modified:**
- `/home/jaime/tau/container/shell.go` — new file
- `/home/jaime/tau/container/tests/shell_test.go` — new file (unit, framing logic)
- `/home/jaime/tau/container/docker/tests/shell_test.go` — new file (integration)

**Acceptance criteria:**
- [ ] `Shell.Run` returns stdout, exit code, and error for both success and non-zero exits
- [ ] cwd persists across successive `Run` calls (e.g., `cd /tmp` then `pwd` returns `/tmp`)
- [ ] environment variables set via `export` persist across `Run` calls
- [ ] `Shell.Close` is idempotent and terminates the underlying `ExecSession`
- [ ] Sentinel collision is extremely unlikely (UUID-based); tests cover output that partially matches the sentinel
- [ ] Concurrent `Shell` instances on one container are independent (integration test)
- [ ] `go build ./... && go vet ./... && go test ./... && (cd docker && go test ./...)` all pass

## Sub-Issue Creation Commands

Pre-resolved IDs:

| Item | Value |
|------|-------|
| Repo | `tailored-agentic-units/container` |
| Parent Objective (#18) node id | resolve at exec time via `gh issue view 18 --json id --jq '.id'` |
| Task issue type id | `IT_kwDOD155C84B2CKc` |
| Milestone | `Phase 2 - Agent Tool Bridge` |
| Project | #9 TAU Container (phase field: `Phase 2 - Agent Tool Bridge`) |

For each sub-issue:

1. `gh issue create --repo tailored-agentic-units/container --title "<title>" --label feature --milestone "Phase 2 - Agent Tool Bridge" --body "$(cat <<'EOF' ... EOF)"`
2. Fetch child node id: `gh issue view <url> --json id --jq '.id'`
3. Assign `Task` issue type via GraphQL `updateIssueIssueType` with `typeId=IT_kwDOD155C84B2CKc`
4. Link to parent #18 via GraphQL `addSubIssue`
5. Add item to project #9, set phase field to `Phase 2 - Agent Tool Bridge`

### Sub-issue body — 18A

Title: `Runtime.ExecStream primitive and Docker implementation`

Body content source: the "Scope / Approach / Acceptance Criteria" blocks from Sub-Issue 18A above, formatted per the convention in `.claude/plans/deep-mixing-snowglobe.md` (Context → Scope → Approach → Acceptance Criteria).

### Sub-issue body — 18B

Title: `Shell type wrapping ExecSession with PTY + prompt sentinel framing`

Body content source: the "Scope / Approach / Acceptance Criteria" blocks from Sub-Issue 18B above. Must include the dependency-on-18A line in Context.

## `_project/objective.md` Content

Create after both sub-issues exist. Structure:

- Title: `Objective #18 — Persistent Shell Foundation`
- Phase reference: `Phase 2 — Agent Tool Bridge (v0.2.0)`
- Parent issue link
- Scope: one-paragraph summary (from the objective body)
- Sub-issues table: `#`, `Title`, `Repo`, `Depends`, `Status`
- Architecture decisions: the five resolved design decisions above (condensed)
- Acceptance criteria: objective-level (both sub-issues merged, `Runtime.ExecStream` live, `Shell` usable from a Docker container against a real daemon)

## Project Board & Milestone Updates

- Add both new sub-issues to project #9 (TAU Container)
- Set their phase field to `Phase 2 - Agent Tool Bridge` (same as parent Obj #18)
- Verify milestone `Phase 2 - Agent Tool Bridge` is set on each sub-issue at creation (via `--milestone` flag on `gh issue create`)

## Critical Files (Read-Only During Planning)

- `/home/jaime/tau/container/runtime.go` — `Runtime` interface (extension point)
- `/home/jaime/tau/container/container.go` — `ExecOptions`, `ExecResult` (shape conventions for new types)
- `/home/jaime/tau/container/docker/docker.go` — existing `Exec` impl (template for `ExecStream` impl)
- `/home/jaime/tau/container/docker/tests/io_test.go` — test patterns (`skipIfNoDaemon`, `startSleeper`)
- `/home/jaime/tau/container/_project/phase.md` — Phase 2 objective list + cross-cutting decisions
- `/home/jaime/tau/container/.claude/plans/deep-mixing-snowglobe.md` — phase planning decisions this objective inherits

## Verification

After objective planning completes:

- `gh issue list --repo tailored-agentic-units/container --search "parent-issue:18" --json number,title,state` returns the two new sub-issues
- Each sub-issue has: `feature` label, `Task` issue type, milestone `Phase 2 - Agent Tool Bridge`, is linked to parent #18, is on project #9 in the `Phase 2 - Agent Tool Bridge` phase
- `_project/objective.md` exists and matches the structure above
- `_project/phase.md` objectives table still shows Obj #18 row (no status change yet — status flips to "In Progress" when the first sub-issue branch lands)
