# Path Forward: `tau/runtime` + Kernel Refactor Before Container Resumes

## Context

You asked me to evaluate whether the container library genuinely fits the TAU ecosystem, prompted by the herald precedent: ImageMagick was baked into an Alpine image and consumed via a typed Go wrapper, forcing every new capability through install + typed wrapper + recompile. Your intent was to generalize that pattern so an agent could operate inside an environment it discovers and actuates against.

Through this conversation a sharper frame emerged. Container is not a distinct architectural layer; it's one implementation of a kernel-runtime contract that must also cover direct execution on physical hosts (development machines, edge devices, embedded/IoT, remote targets). The library is currently OCI-shaped ahead of the contract it should conform to. The corrective move is to:

1. Finish the kernel's post-extraction refactor and clean up its project/GH state.
2. Design the kernel interface and the runtime contract together, in one motion, with a minimal `native` implementation that gives the kernel something to talk to.
3. Leave `tau/container` alone until the contract + `native` are proven. Then initialize `tau/runtime/container` *fresh*, using the finished contract and the existing container context + code as reference material — not as a migration.

## Verdict

Not a tangent — the pain is real and no other TAU module claims this territory. But container is being shaped ahead of its contract. Pause container Phase 2, pivot to kernel + runtime contract design, validate against a `native` implementation, then bootstrap a clean `tau/runtime/container` sub-module that inherits the contract's shape rather than inventing its own.

## Module Layout: `tau/runtime`

`tau/runtime` is a new umbrella repo/module (`github.com/tailored-agentic-units/runtime`); `tau/container` remains untouched apart from contextual-documentation updates. This mirrors `tau/provider` + `tau/provider/bedrock` — root module exposes the universal contract, sub-modules provide implementations. Sub-module nesting is legitimate because each level has its own `go.mod`.

```
tau/runtime/                     # root module — universal Runtime contract + registry
tau/runtime/native/              # sub-module — direct execution on physical hosts (dev, edge, embedded) — populated in Phase 3
tau/runtime/container/           # sub-module — container family — NOT created until Phase 6
tau/runtime/container/docker/    # sub-sub-module — Docker backend — NOT created until Phase 6
```

- `tau/runtime` root: the universal `Runtime` interface (perception, actuation, capability declaration, attach/detach lifecycle) + factory registry. Zero heavy dependencies. **Repo scaffolding created in Phase 0; contract populated in Phase 3.**
- `tau/runtime/native`: reference implementation. Platform variation (dev machine, edge, embedded) is an internal concern — same interface, no premature sub-substrate split. **Created in Phase 3 with minimal pass-through impl.**
- `tau/runtime/container` (Phase 6): bootstrapped fresh against the proven contract, using `tau/container`'s current context artifacts and implementation code as design references. Not a code move; a documentation-led initialization followed by clean-slate implementation.
- `tau/runtime/container/docker` (Phase 6): Docker backend, built fresh. Current `tau/container/docker` code is reference material for design decisions (labels, manifest pathing, exec plumbing, PTY sentinel framing) but not a direct port.

`tau/container` stays in place throughout Phases 1–5, touched only for contextual-documentation updates in Phase 0. Its `v0.1.0` release remains intact. No repo moves, no git-history reshuffling.

## Refined Execution Sequence

| Phase | Work |
|-------|------|
| 0 | **Preliminary setup.** (a) Initialize `tau/runtime` directory at `~/tau/runtime/` with Go module scaffolding (`go.mod` with module path `github.com/tailored-agentic-units/runtime`, `doc.go`, `README.md`, `_project/README.md`, `.claude/CLAUDE.md`, `.gitignore`, `CHANGELOG.md`). No contract code yet — scaffolding only. (b) Draft the initial runtime-contract concept doc in `tau/runtime/_project/` capturing the contract's intended shape (perception, actuation, capability declaration, attach/detach lifecycle) and what stays below the contract per-substrate. Phase 3 later evaluates the post-Phase-2 kernel architecture against this doc to establish implementation scope. (c) Create GitHub repo `tailored-agentic-units/runtime`, push initial commit. **No project board** — deferred to Phase 3. (d) Update `tau/container` contextual documentation to reflect current state and its position in the revised architectural vision: `_project/README.md` (position as a pre-contract reference artifact; Phase 2 paused; scheduled for deletion after Phase 6 transition), `.claude/CLAUDE.md` (note that the library is paused and its role in the broader direction), any concept docs under `.claude/context/concepts/` that need alignment. **Code untouched.** |
| 1 | **Kernel post-extraction refactor** — swap local packages for extracted-library imports (protocol, format, provider, agent, orchestrate); migrate response model; remove ConnectRPC; update `go.mod`; refresh kernel README + `_project/` docs to reflect the post-extraction baseline. |
| 2 | **Kernel context + GH cleanup** — close orchestrate-shaped issues #5–9 on kernel and create equivalents on `tau/orchestrate`; close #10 and create equivalent on `tau/agent`; archive `_project/library-extraction.md`; review kernel's GitHub project board, milestones, and objectives (#2, #3, #4) for alignment with the new direction. |
| 3 | **Joint design: Kernel Interface + `tau/runtime` contract + minimal `tau/runtime/native`.** Evaluate the Phase 2 kernel architecture against the Phase 0 concept doc in `tau/runtime/_project/`; refine the doc based on what the cleaned-up kernel actually needs; establish implementation scope. Deliverables: (a) refined contract concept doc; (b) populate `tau/runtime` root with the `Runtime` interface + registry; (c) create `tau/runtime/native` sub-module with a minimal implementation sufficient to run an end-to-end kernel loop (pass-through to `tau/agent` acceptable initially); (d) kernel's public interface wired through the contract; (e) initialize GitHub project board for `tau/runtime` now that implementation scope is known. |
| 4 | **Kernel Interface completion** (Objective #2, issues #26/#27/#28) — completed against the contract from Phase 3. Open issues may need revision to be contract-aware. |
| 5 | **Native maturation + kernel internals reshaping.** `tau/runtime/native` becomes the production-grade reference. Kernel internals reshape to handle capabilities through the contract. Objective #4 "Local Development Mode" likely rescopes or retires — local dev is now a config of `native`, not a separate deployment mode. |
| 6 | **Bootstrap `tau/runtime/container` from references.** With contract + native proven, initialize the container sub-modules. Steps: (a) derive a container spec doc from the finished `Runtime` interface — what does an OCI implementation need to satisfy the contract, what container-family additions (images, manifest, labels) belong above it; (b) review `tau/container`'s `_project/`, `.claude/context/concepts/`, guides, and sessions as design references; (c) review `tau/container`'s current implementation (`runtime.go`, `exec.go`, `shell.go`, `docker/*`) for specific design decisions worth preserving (label conventions, manifest pathing, PTY sentinel framing, exec plumbing); (d) implement `tau/runtime/container` + `tau/runtime/container/docker` fresh against the contract. Existing `tau/container` remains as a reference artifact; archive-or-keep decision deferred. |
| 7 | **Skills and MCP Integration** (Objective #3) — proceeds against the matured kernel + runtime stack. |
| 8 | **Embedded container mode** — kernel binary baked into container images. Requires kernel + contract + `tau/runtime/container` all complete. |

Dependency graph: Phases 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. Phases are largely sequential; 3–5 are the load-bearing block.

## EXECUTION-PLAN.md Redesign

The current `EXECUTION-PLAN.md` accumulates completed work (Group A done, Group B1 shipped as `v0.1.0`) alongside live work, and its dependency graph treats kernel and container as parallel. Proposed redesign: discard completed/shipped phases entirely and reorient to the current state.

Proposed structure for the new file:

```markdown
# TAU Platform Execution Plan

Strategic execution sequence for current TAU platform development initiatives.
Revised 2026-04-17.

## Context

Post-extraction state: five standalone libraries (protocol, format, provider, agent, orchestrate) at v0.1.0 stable. Kernel still pre-extraction-refactor; needs to swap local packages for extracted-library imports. A container library exists at v0.1.0 with Phase 2 partially underway, but is paused pending a universal kernel-runtime contract that will re-shape it as one of several runtime implementations. A new `tau/runtime` umbrella module hosts that contract and its implementations.

Completed work (marketplace refactor, iterative-dev skill, container Phase 1) is archival and not reflected here.

## Execution Sequence

| Phase | Initiative | Target |
|-------|-----------|--------|
| 0 | Initialize tau/runtime repo scaffolding; update tau/container contextual docs | tau/runtime (new, scaffolding only), tau/container (docs only) |
| 1 | Kernel post-extraction refactor | tau/kernel |
| 2 | Kernel context + GH cleanup | tau/kernel, tau/orchestrate, tau/agent |
| 3 | Kernel Interface + tau/runtime contract concept + tau/runtime/native minimal impl; initialize tau/runtime GH project board | tau/kernel, tau/runtime, tau/runtime/native (new) |
| 4 | Kernel Interface completion (Obj #2 / #26-#28) | tau/kernel v0.1.0 |
| 5 | Native implementation maturation + kernel internals reshaping | tau/runtime/native, tau/kernel |
| 6 | Bootstrap tau/runtime/container + tau/runtime/container/docker from tau/container references | tau/runtime/container (new), tau/runtime/container/docker (new) |
| 7 | Skills and MCP Integration (Obj #3) | tau/kernel |
| 8 | Embedded container mode | tau/runtime/container, tau/kernel |

## Dependency Graph

Phase 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. Phases 3–5 are the load-bearing block; contract design and native reference must land before container work resumes.

## Repositories

| Repository | Status |
|-----------|--------|
| tau/protocol | v0.1.0 stable |
| tau/format | v0.1.0 stable |
| tau/provider | v0.1.0 stable |
| tau/agent | v0.1.0 stable |
| tau/orchestrate | v0.1.0 stable |
| tau/kernel | Pre-extraction; Phase 1 refactor pending |
| tau/container | v0.1.0 shipped; Phase 2 paused; contextual docs updated in Phase 0; reference artifact for Phase 6 bootstrap |
| tau/runtime | Scaffolding created in Phase 0; contract populated in Phase 3 |

## Session Init Artifacts

| Phase | Artifact |
|------|----------|
| 0 | tau/runtime/_project/README.md + initial runtime-contract concept doc (created here); tau/container/_project/README.md (updated here) |
| 1 | tau/kernel/_project/post-extraction.md |
| 2 | tau/kernel/_project/post-extraction.md (Project Management Updates section) |
| 3 | New concept doc for kernel-runtime contract (location TBD) |
| 4 | Kernel GH issues #26/#27/#28 + tau/kernel/_project/README.md |
| 5 | tau/runtime/native/_project/README.md (created in Phase 3) |
| 6 | tau/container/_project/ + tau/container/.claude/context/ as reference material; new tau/runtime/container/_project/README.md |
| 7 | Kernel GH Objective #3 |
| 8 | tau/runtime/container/_project/README.md (Embedded Mode section) |
```

This reframe drops Group A/B1/tooling-foundation artifacts from the live plan and archives them implicitly.

## What Changes in Which Repos

- **`tau-platform/EXECUTION-PLAN.md`** — holistic rewrite per the structure above. Discards completed work; reorients to current state.
- **`tau/runtime/` (new)** — created in Phase 0 as repo scaffolding (`github.com/tailored-agentic-units/runtime`). Contract populated in Phase 3; `native/` sub-module added in Phase 3.
- **`tau/container/`** — Phase 0 documentation-only updates (`_project/README.md`, `.claude/CLAUDE.md`, concept docs as needed). Code untouched through Phases 1–5. Used as reference material in Phase 6.
- **`tau/kernel/`** — Phase 1 refactor; Phase 2 orphan reassignment + context cleanup; GH project board/milestones/objectives reviewed and relabeled; Phase 3 public interface wired through the new runtime contract.
- **`tau/runtime/container/` (new, Phase 6)** — fresh sub-module initialized against the contract using `tau/container` artifacts as reference. Not a code migration.
- **`tau/runtime/container/docker/` (new, Phase 6)** — fresh Docker backend sub-module, same treatment.
- **Kernel GH project board** — milestones realigned; objectives reviewed; issues relabeled/reassigned as needed.
- **`tau/runtime` GH project board** — deferred to Phase 3 (not created in Phase 0).
- **`tau/container` after Phase 6** — deleted locally and on GitHub once the transition into `tau/runtime/container` is complete.

## Critical Files / Artifacts

- `/home/jaime/tau/tau-platform/EXECUTION-PLAN.md` — target of redesign.
- `/home/jaime/tau/kernel/_project/post-extraction.md` — Phase 1/2 scope reference.
- `/home/jaime/tau/kernel/README.md`, `doc.go` — kernel's current shape pre-refactor.
- `/home/jaime/tau/kernel/` tools package — the static registry being superseded by contract-driven capabilities.
- `/home/jaime/tau/container/runtime.go`, `exec.go`, `shell.go`, `docker/` — Phase 6 reference material.
- `/home/jaime/tau/container/_project/`, `/home/jaime/tau/container/.claude/context/` — Phase 6 documentation reference material.

## Verification

- Phase 0: `~/tau/runtime/` directory exists with valid Go module scaffolding; initial runtime-contract concept doc exists in `tau/runtime/_project/`; `github.com/tailored-agentic-units/runtime` GH repo exists with initial commit; `tau/container` `_project/README.md` + `.claude/CLAUDE.md` reflect paused state, alignment with revised vision, and scheduled deletion after Phase 6; `EXECUTION-PLAN.md` rewrite is pushed.
- Phase 1: kernel compiles against extracted-library imports with no local duplicates; tests pass; `go.mod` reflects the new dependency graph.
- Phase 2: no orphaned issues remain on kernel GH; `_project/` docs reflect post-extraction baseline; GH project board aligned with revised direction.
- Phase 3: concept doc exists; `tau/runtime` root populated with contract + registry; `tau/runtime/native` sub-module exists and builds; kernel's public interface routes through the contract; a minimal end-to-end loop runs on `native`; `tau/runtime` GH project board initialized.
- Phase 4–5: kernel runs a nontrivial agentic loop end-to-end via `native`; Objective #4 rescoped or retired.
- Phase 6: `tau/runtime/container` + `tau/runtime/container/docker` exist, compile, satisfy the contract, and replicate the essential capabilities of the current `tau/container` (lifecycle, exec, shell, file ops, manifest) through the contract's abstractions.

## Context Snapshot — 2026-04-17 (pause for travel)

### Phase 0 status

Phase 0 is **functionally complete but uncommitted in two of three repos**. Nothing is broken. Everything can be picked up cleanly.

### Files modified

**`tau/runtime/` (new repo — committed + pushed):**
- `go.mod` — module `github.com/tailored-agentic-units/runtime`, Go 1.26
- `doc.go` — package comment
- `README.md` — top-level README
- `CHANGELOG.md` — initial scaffolding entry
- `_project/README.md` — project overview + phases table + planned architecture
- `_project/runtime-contract.md` — initial concept doc (four responsibility areas, substrate boundary, open Phase 3 questions)
- `.claude/CLAUDE.md` — project instructions, planned modules, scaffolding-state guidance
- Initial commit `efe38d6` on `main`, pushed to `https://github.com/tailored-agentic-units/runtime`
- Added `./runtime` to `~/tau/go.work` (uncommitted — `~/tau/go.work` lives in the `~/tau/` workspace, not a specific repo)

**`tau/container/` (uncommitted edits):**
- `_project/README.md` — added Status section at top; reframed "Vision" as historical; updated Phases table (Phase 2 paused, Phase 3 cancelled); marked dependency-position diagram as superseded with revised graph showing `runtime` umbrella
- `.claude/CLAUDE.md` — added Status section noting pause, reference-artifact role, scheduled deletion after Phase 6, don't-add-code guidance

**`tau/tau-platform/` (uncommitted edit):**
- `EXECUTION-PLAN.md` — holistic rewrite per the structure in this plan file. Flat Phase 0–8 sequence. Dropped completed Group A/B1 content. Updated repositories table and session init artifacts.

### Next steps when resuming

1. **Decide on commits.** Three separate repos have uncommitted edits:
   - `~/tau/tau-platform/` (EXECUTION-PLAN rewrite) — suggest separate commit
   - `~/tau/container/` (doc-only pause-framing updates) — suggest separate commit
   - `~/tau/go.work` (added `./runtime`) — this is in the `~/tau/` workspace root; whoever owns that repo decides
2. **Optional polish of `tau/runtime` docs.** The concept doc is deliberately first-draft; fine to leave as-is for Phase 3 refinement. No commit needed unless you want polish now.
3. **Phase 1 (kernel post-extraction refactor)** is the next phase of work. Entry artifact is `~/tau/kernel/_project/post-extraction.md`. Phase 1 is not trivial — expect a dedicated session.

### Key decisions locked in this session

- Container library is not a tangent but was OCI-shaped ahead of its contract.
- Created a new umbrella repo `tau/runtime` at `github.com/tailored-agentic-units/runtime`.
- Layout: `tau/runtime/` root + `native/`, `container/`, `container/docker/` sub-modules. `native` is the term for direct-physical-host execution (not "bare-metal").
- `tau/container` stays paused and untouched (code-wise) through Phases 1–5, then deleted after Phase 6 transition.
- `tau/runtime/container` is initialized fresh in Phase 6 using `tau/container` as reference material — not a code migration.
- `tau/runtime` GitHub project board deferred to Phase 3.
- Concept doc lives in `tau/runtime/_project/runtime-contract.md`.

### Blockers

None. Phase 1 work is unblocked and can begin any time after resumption.

### Task state

All four Phase 0 tasks complete (scaffold, GH repo, container docs, EXECUTION-PLAN rewrite). No tasks for Phase 1 yet — create fresh tasks when that session begins.

