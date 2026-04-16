# Phase 2 Planning — Agent Tool Bridge

## Context

Phase 1 (Runtime Foundation, v0.1.0) shipped: the `Runtime` interface, the strict image capability manifest (`/etc/tau/manifest.json`, `version: "1"`), and the Docker sub-module with all 8 OCI-aligned methods. Tags `v0.1.0` and `docker/v0.1.0` are live; the `docker-hello` example merged in `tailored-agentic-units/examples` against the published tags.

Phase 2 builds the agent-facing surface on top of that foundation. Per `_project/README.md`:

> **Phase 2 — Agent Tool Bridge** (target `v0.2.0`): Tool-wrapped persistent shell, structured file/process tools, dynamic tool generation from manifest, agent integration surface.

The lynchpin is the persistent **Shell** type that wraps `Runtime.Exec` into a long-running, state-preserving session (cwd, env, shell history). Around it sit structured built-in tools (`file_*`, `process_*`) and a tool-definition surface that aggregates built-ins with manifest-declared CLIs into the `[]format.ToolDefinition` slice that `agent.Tools(ctx, msgs, tools, opts...)` consumes.

This session covers two distinct workflow steps:
1. **Step 0: Phase 1 transition closeout** (lightweight — most artifacts already shipped)
2. **Steps 1–6: Phase 2 decomposition** (3 Objectives, milestone already exists as #2)

> **Numbering convention.** Objectives and Sub-issues are referenced by their resulting GitHub issue number (e.g., `Obj #18`, `18A`), not by phase-local sequential index. See `feedback_objective_task_numbering.md` in user memory for the rationale.

---

## Step 0 — Phase 1 Closeout (lightweight)

Most closeout artifacts already shipped during the Phase 1 release session. Remaining work:

| Item | State | Action |
|------|-------|--------|
| CHANGELOG consolidation | Already clean (no `v0.1.0-dev.*` sections existed — user deferred dev tags during scaffolding) | None |
| Dev tags cleanup | None to clean | None |
| Phase release tagging | `v0.1.0` + `docker/v0.1.0` already live | None |
| Objective #3 | Open on GitHub; all 4 sub-issues closed | Close issue #3 |
| Phase 1 milestone | Open (9 closed, 1 open — Obj #3) | Close after #3 closes |
| `_project/README.md` Phases table | Missing Status column | Add column; mark Phase 1 "Complete" |
| `_project/phase.md` | Phase 1 content | Replace with Phase 2 content (Step 5) |
| `_project/objective.md` | Phase 1 Obj #3 content | Delete the file — the next `/dev-workflow objective` session re-initializes it for Obj #18 |

No transitioning sub-issues — Phase 1 is fully resolved, nothing to carry forward to Phase 2 backlog.

---

## Step 1–4 — Phase 2 Decomposition (3 Objectives)

The decomposition mirrors Phase 1's 3-Objective shape. Phase 1 split along **types → contract → implementation**; Phase 2 splits along **session foundation → tool primitives → agent surface + integration**.

Each Objective is one parent issue on `tailored-agentic-units/container` with the `objective` label and `Objective` issue type, assigned to the existing `Phase 2 - Agent Tool Bridge` milestone (#2). Issues created during this session: **#18, #19, #20**.

### Objective #18 — Persistent Shell Foundation

**Scope.** Add a new `Runtime.ExecStream` method that returns an `*ExecSession` handle (`Stdin io.WriteCloser`, `Stdout io.Reader`, `Stderr io.Reader`, `Wait() (int, error)`, `Close() error`), and build the `Shell` type that wraps it into a long-lived, state-preserving session. The README package layout already reserves `shell.go` at the root for this work; Phase 1's `phase.md` explicitly defers it to Phase 2.

**Sub-issues:**

| # | Title | Repo | Depends |
|---|-------|------|---------|
| 18A | `Runtime.ExecStream` + `ExecSession` type — root interface + Docker impl | container | — |
| 18B | `Shell` type wrapping `ExecSession`; cwd/env/history preserved across `Run` calls | container | 18A |

**Streaming primitive — design rationale.** Three approaches were considered:

1. **`Runtime.ExecStream` returning `*ExecSession`** *(chosen)* — explicit method, separate signature from `Exec`. Keeps `Exec`'s one-shot contract (`*ExecResult` of `[]byte` buffers) clean and matches OCI conventions where attach/exec are distinct verbs (`docker exec` vs. `docker attach`). Costs one extra method on the interface (now 9 methods) and a new exported type (`ExecSession`). The session handle lifecycle is unambiguous: `Wait` returns when the process exits; `Close` kills it early.

2. **`ExecOptions.Stream bool` flag overloading `Exec`** — single method, smaller surface (8 methods), but `Exec` would have to return `any` (or a result-union type) since one-shot returns `*ExecResult` and streaming returns a session handle. Callers lose compile-time type safety on the return — they must type-assert based on a flag they themselves set. The polymorphism saves one method but costs the cleanest property of the current interface: every method has a single, statically-typed return shape.

3. **Defer streaming entirely; build `Shell` on top of one-shot `Exec`** — `Shell.Run` wraps each command as `cd <cwd> && export <env...> && <cmd>` and tracks state on the host. Zero Runtime change. Loses true shell semantics: no shell history, no sourced rc files, no background jobs, no stateful tools that depend on a live shell process (e.g., `ssh-agent`, `direnv`, mise's shell hook). The vision statement — "an agent has access to the same tools and files that a human user would" — is exactly the statement this approach undercuts. Acceptable as a stopgap if streaming were technically blocked, but Docker's exec API supports streaming natively (`Tty: true`, `AttachStdin/Stdout/Stderr` with hijacked TCP), so there's no infrastructure reason to compromise.

The choice prioritizes interface clarity and true shell semantics over surface minimalism.

**Other design questions** (resolve in Obj #18 planning session):
- Shell command framing: PTY with prompt-sentinel parsing, or non-PTY with explicit stdout/stderr delimiters per command?
- `Shell.Close()` semantics: kill underlying exec instance + drain streams; container itself untouched.
- Does `ExecStream` accept `ExecOptions` directly (reusing `Cmd`, `Env`, `WorkingDir`) and ignore the `Attach*` flags, or take a new `ExecStreamOptions`?

**Why first.** The agent surface (Obj #20) consumes `Shell` to expose a `shell` built-in tool — the centerpiece of the README's "Built-in tools" list. Without `Shell` it regresses to per-call `Exec`, losing the "agent operates as a user" property the vision rests on. (Obj #19's file/process built-ins do not depend on `Shell`; they sit on existing primitives — see Obj #19 note.)

### Objective #19 — Structured Built-in Tools

**Scope.** Extract the manifest types into their own sub-package, then add the set of built-in `format.ToolDefinition`s every tau-managed container exposes regardless of manifest contents. Built directly on existing Runtime primitives (`CopyTo`/`CopyFrom` for files; `Exec` for processes — `Shell` is not a hard dependency for the file/process built-ins).

**Sub-issues:**

| # | Title | Repo | Depends |
|---|-------|------|---------|
| 19A | Extract manifest to `container/manifest` sub-package (mechanical rename + import updates) | container | — |
| 19B | Top-level `container.Tool` type + file built-ins (`file_read`, `file_write`, `file_list`) | container | 19A |
| 19C | Process built-ins (`process_list`, `process_kill`) | container | 19B |

**Manifest sub-package extraction (19A) — resolved naming approach.** The introduction of a top-level execution unit named `Tool` collides with today's `container.Tool` (the CLI-metadata struct in `manifest.go`). Rather than renaming one of them and accepting stuttery names like `container.ContainerTool`, extract the manifest concern into its own sub-package:

- New package: `github.com/tailored-agentic-units/container/manifest` (sub-package within the root module — single `go.mod`, no module split; manifest depends only on stdlib `encoding/json`)
- Move from `container/manifest.go` → `container/manifest/manifest.go`:
  - Types: `Manifest`, `Tool`, `Service`
  - Constants: `ManifestVersion`, `ManifestPath`
  - Functions: `Parse`, `Validate`, `Fallback`
- Move corresponding errors from `container/errors.go` → `container/manifest/errors.go`: `ErrManifestInvalid`, `ErrManifestVersion`
- Update `container.ContainerInfo.Manifest` field from `*Manifest` → `*manifest.Manifest`
- Update docker sub-module call sites: `container.Parse` → `manifest.Parse`, `container.ManifestPath` → `manifest.ManifestPath`, etc.
- Update `examples/container/docker-hello/main.go` to use new import path and `container.Fallback()` → `manifest.Fallback()`
- Update `_project/README.md` package layout diagram (root no longer shows `manifest.go`; lists `manifest/` sub-package alongside `docker/`)
- Update `.claude/CLAUDE.md` Project Structure block accordingly

After extraction the naming is clean: `manifest.Tool` describes the CLI metadata declared in an image manifest; `container.Tool` (introduced in 19B) is the runtime execution unit. No stutter, no collision, and the manifest format becomes a consumable primitive other tau modules can import without pulling in the Runtime interface.

This is a breaking change to the v0.1.0 surface, which is permissible pre-1.0. The change is purely structural — no behavior change in 19A.

**Design contract for the new top-level `Tool` (19B).** Each built-in is a `(format.ToolDefinition, Handler)` pair:

```go
type Tool struct {
    Definition format.ToolDefinition
    Handler    func(ctx context.Context, c *Container, args json.RawMessage) (any, error)
}
```

This is the integration unit that Obj #20 aggregates and that downstream consumers (agent in toolkit mode, kernel in eventual integration) dispatch against.

**Why partly serial.** 19A is a pure refactor — easy review, no semantic change. 19B and 19C build on it and could in principle land in either order, but 19B introduces the `Tool` type that 19C uses, so 19C waits for 19B.

**Why parallel-safe with Obj #18.** Obj #19 uses only Phase 1 primitives plus the new top-level `Tool` type. Obj #18's streaming work touches `runtime.go` and the docker sub-module's exec implementation — no overlap with manifest extraction or built-in tool definitions. Obj #18 and Obj #19 can land in either order.

### Objective #20 — Manifest-Driven Tool Surface & Agent Integration

**Scope.** `tools.go` at the root with manifest-driven generators, the aggregation helper that produces the `[]format.ToolDefinition` slice an agent consumes, and a runnable toolkit-mode example in `tailored-agentic-units/examples` that drives a real agent against a real container.

**Sub-issues:**

| # | Title | Repo | Depends |
|---|-------|------|---------|
| 20A | `ToolsFromManifest(*manifest.Manifest) []Tool` — synthesizes `format.ToolDefinition`s from manifest `Tools` map; description + version surfaced into the definition's description | container | Obj #18, Obj #19 |
| 20B | Aggregator — `Container.Tools()` (or `Tools(c *Container) []Tool`) merging built-ins + manifest tools, plus a stateless `Dispatch(ctx, c, name, args)` that routes tool calls to the right handler | container | 20A |
| 20C | Toolkit-mode example in `examples/container/agent-shell/` — agent loop using `provider/<x>` + container tools to perform a real task (e.g., "list workspace, write a file, run `gh --version`") | examples (tracked on container) | 20A, 20B + `v0.2.0` tags |

**Why the example is a sub-issue, not a phase deliverable.** Mirrors Phase 1's docker-hello sub-issue: tracked here so the Objective roll-up is honest, but the implementing PR opens against the examples repo and is gated on the Phase 2 release tags. Same disposition note applies — the example exercises the published contract that downstream consumers experience.

**Release-vs-closeout sequencing (important).** Sub-issue 20C breaks the standard "release immediately precedes closeout" cadence. The sequence is:

1. Sub-issues 20A and 20B merge (library work complete on `tailored-agentic-units/container`)
2. Run `/dev-workflow release v0.2.0` → tag `v0.2.0` and `docker/v0.2.0` cut from `main`
3. **Closeout does NOT immediately follow.** Phase 2 stays open with one sub-issue (20C) still in flight
4. Sub-issue 20C is implemented in `tailored-agentic-units/examples` consuming the published `v0.2.0` libraries (the dependency that necessitated the release)
5. Once 20C merges, Obj #20 closes, then `/dev-workflow phase` runs the Phase 2 closeout

This is the same pattern Phase 1 followed (`docker-hello`/sub-issue #14 was implemented after `v0.1.0`/`docker/v0.1.0` shipped, and Obj #3 stayed open until that merge — exactly the state this current planning session is closing out). Capturing it here so the eventual release session doesn't trigger closeout prematurely.

**Integration boundary.** "Agent integration surface" in this phase means consumption via `agent.Tools(ctx, msgs, []format.ToolDefinition, opts...)`. Kernel toolkit-mode integration (registering container tools into `kernel.tools.Registry`) is **out of scope** — that lives downstream and likely gets its own objective when the kernel team picks it up.

---

## Step 5 — `_project/phase.md` (Phase 2 content)

Replace the current Phase 1 content with:

- **Phase name + version target** — `Phase 2 — Agent Tool Bridge`, `v0.2.0`
- **Scope** — persistent shell wrapping the streaming primitive; built-in file/process tools; manifest-driven dynamic tool generation; `[]format.ToolDefinition` surface for `agent.Tools()` consumption
- **Objectives table** — 3 rows linked to issues #18, #19, #20 with status `Todo`
- **Constraints** — Obj #18 and Obj #19 may proceed in parallel; Obj #20 depends on both. Sub-issue 20C blocked on `v0.2.0` + `docker/v0.2.0` release tags. Root `go.mod` + docker sub-module both tag `v0.2.0` at phase close
- **Cross-cutting decisions** — to be filled in by Obj planning sessions, but seed with: streaming-exec cancellation semantics extend the existing context model; shell `Close()` is independent of container lifecycle; manifest extracted to `container/manifest` sub-package (`manifest.Tool` for CLI metadata, `container.Tool` for execution unit); `Dispatch` is stateless and concurrent-safe

---

## Step 6 — Project Board & Milestone Updates

- Each new Objective added to project #9 (TAU Container) and assigned to the `Phase 2 - Agent Tool Bridge` field option
- Milestone #2 (`Phase 2 - Agent Tool Bridge`) already exists — assign Objective issues to it on creation
- Close milestone #1 once Objective #3 closes
- No carry-forward objectives (no `Step 6` phase reassignment work)

---

## Critical Files

**Read during planning:**
- `/home/jaime/tau/container/_project/README.md` — vision, Phases roadmap, package layout
- `/home/jaime/tau/container/_project/phase.md` — Phase 1 status (replaced in Step 5)
- `/home/jaime/tau/container/runtime.go` — current `Runtime` interface (extension point for streaming exec)
- `/home/jaime/tau/container/manifest.go` — `manifest.Tool` shape (drives 20A; relocated to sub-package in 19A)
- `/home/jaime/tau/format/data.go:37-42` — `ToolDefinition` shape consumed by Obj #20
- `/home/jaime/tau/agent/agent.go:199-218` — `Tools()` signature this phase targets

**Modified during this session:**
- `/home/jaime/tau/container/_project/README.md` — add Status column, mark Phase 1 Complete
- `/home/jaime/tau/container/_project/phase.md` — replace with Phase 2 content
- `/home/jaime/tau/container/_project/objective.md` — deleted (re-created during next Obj planning session)

**Created during this session:**
- 3 new GitHub Objective issues (#18, #19, #20) on `tailored-agentic-units/container`

---

## Verification

- `gh issue view 3` shows state `closed`
- `gh api repos/tailored-agentic-units/container/milestones/1 --jq '.state'` returns `closed`
- `gh issue list --label objective --state open` returns the 3 new Phase 2 Objectives (#18, #19, #20)
- Each new Objective has `Objective` issue type, `objective` label, and is assigned to milestone `Phase 2 - Agent Tool Bridge`
- `gh project item-list 9 --owner tailored-agentic-units --format json --jq '.items[] | select(.content.title | startswith("Objective:")) | {title, phase: .phase}'` shows all three Phase 2 Objectives mapped to the `Phase 2 - Agent Tool Bridge` field option
- `_project/phase.md` reflects Phase 2 scope and Objectives table
- `_project/README.md` Phases table has a Status column and Phase 1 marked Complete

---

## Resolved Decisions

1. **Streaming exec primitive** — new `Runtime.ExecStream` method returning `*ExecSession` (rationale documented in Obj #18 above).
2. **Decomposition granularity** — 3 Objectives, file + process built-ins bundled in Obj #19.
3. **Toolkit-mode example** — included as sub-issue 20C, with the release-vs-closeout sequencing note in Obj #20 above making the deferred-closeout dependency explicit.
4. **Numbering convention** — Objectives and sub-issues referenced by GitHub issue number (Obj #18, sub-issue 18A) rather than phase-local sequence; recorded as user feedback memory.
