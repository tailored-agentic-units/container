# Phase Planning: Container Phase 1 — Runtime Foundation

## Context

This is the inaugural phase planning session for the new `tau/container` package. The repository was initialized with a concept-development commit and exists at `tailored-agentic-units/container` with project board #9 and milestones for all three phases already created. No `_project/phase.md` exists yet, so no transition closeout is required — this is a clean Phase 1 kickoff.

Phase 1 establishes the runtime foundation for OCI-aligned container execution: the `Runtime` interface, a Docker sub-module implementing it, container lifecycle, one-shot exec, file copy, and the image capability manifest convention at `/etc/tau/manifest.json`. Target version is `v0.1.0`. Per `~/tau/tau-platform/EXECUTION-PLAN.md` step B1, this work runs concurrently with kernel post-extraction (Group C) and unblocks Phase 2 (toolkit-mode agent integration).

The phase decomposition needs to fit established TAU patterns: root module defines the interface and a thread-safe `Register()` registry, sub-modules implement and call back into the root (mirroring `provider/registry.go` and `provider/azure/`). Tests live in `tests/` at each module level. Sub-module `go.mod` files require both `protocol` and the root container module with no `replace` directives.

## Resolved decisions

- **3-objective decomposition** confirmed (sub-PR-sized splits handled in Objective Planning).
- **Phase-planning PR bundles the staged `_project/README.md` whitespace fix** with the new `_project/phase.md`.
- **`Register()` lives inline in `docker/docker.go`** (matches `provider/azure/azure.go`, `provider/bedrock/bedrock.go`). Deviates from the `register.go` filename in the README package layout; `phase.md` will note this deviation so the README can be updated later.

## Decomposition

Three objectives. Objectives 1 and 2 run in parallel (independent); Objective 3 depends on both.

| # | Objective | Files | Depends on |
|---|-----------|-------|------------|
| 1 | Runtime Interface & Core Types | `runtime.go`, `container.go`, `errors.go`, `registry.go` (mirroring `provider/registry.go`), `doc.go`, `tests/registry_test.go`, `tests/types_test.go` | — |
| 2 | Image Capability Manifest | `manifest.go`, `tests/manifest_test.go` (golden JSON fixtures) | — |
| 3 | Docker Runtime Implementation | `docker/go.mod`, `docker/docker.go` (client wiring, all 8 `Runtime` methods, `tau.*` label convention, `Register()`), `docker/tests/docker_test.go` (skip when Docker unreachable), `examples/docker-hello/main.go` | Obj 1 + Obj 2 |

**Why three, not more.** Docker-Engine API calls cluster naturally around a shared `client.Client`, container-ID lookups, error mapping, and label conventions. Splitting lifecycle from exec/copy at the *phase* level fragments the testing story (exec and copy both need a running container from `Create`+`Start`). Sub-PR-sized splitting belongs in Objective Planning, not phase planning.

**Why no separate "integration" objective.** The Docker sub-module is the only place a real runtime exists in Phase 1. End-to-end smoke tests and the runnable example belong with the only implementation that can exercise them. A standalone integration objective would either duplicate fixtures or force Obj 3 to ship without proof-of-life.

## Cross-cutting concerns to capture in `_project/phase.md`

- **Context cancellation contract** — every `Runtime` method takes `ctx`. Cancel during `Exec` kills the exec instance; cancel during `CopyTo`/`CopyFrom` aborts the stream; `Stop` honors its own `timeout` independently of `ctx`. Document on the interface in `runtime.go`.
- **Container labeling convention** — `tau.managed=true`, `tau.manifest.version=1` (and similar `tau.*` labels) reserved for tau-managed containers so future `Inspect`/`List` can filter.
- **Manifest read path** — read via `Runtime.CopyFrom("/etc/tau/manifest.json")` rather than runtime-specific APIs. Keeps the read path runtime-agnostic and validates `CopyFrom` on every container start.
- **Manifest version negotiation** — Phase 1 accepts only `version: "1"`; unknown versions return a typed error; a missing manifest returns a documented fallback `ContainerInfo.Manifest` (POSIX shell, no declared tools).
- **Phase 2 deferrals** — `tools.go` and `shell.go` (listed in the README package layout) are explicitly Phase 2 work; phase.md must call this out so reviewers don't flag their absence.
- **Versioning** — root module tags `v0.1.0` at phase close; Docker sub-module tags independently as `docker/v0.1.0` per sibling convention.

## Proposed Objective Issue Bodies

### Objective 1 — Runtime Interface & Core Types

```
## Objective

Establish the root container module's foundational types and the OCI-aligned `Runtime` interface that all runtime implementations must satisfy. Provides the contract and the registry mechanism that sub-module implementations register against.

## Scope

In scope:
- `Runtime` interface with the 8 methods defined in `_project/README.md` (Create, Start, Stop, Remove, Exec, CopyTo, CopyFrom, Inspect)
- `Container`, `State` (lifecycle states), `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo` types
- Domain error types
- Thread-safe registry mirroring `provider/registry.go` (`Factory`, `Register`, `Create`, `ListRuntimes`)
- Context-cancellation contract documented on each interface method
- Root-module tests in `tests/`

Out of scope:
- Any runtime implementation (Obj 3)
- Manifest types (Obj 2)
- Persistent shell, tool generators (Phase 2)

## Repositories

`tailored-agentic-units/container` (root module only)

## Dependencies

None. Unblocks Obj 3.
```

### Objective 2 — Image Capability Manifest

```
## Objective

Implement the image capability manifest convention — types, parsing, validation, and fallback — so containers can declare their tools, services, shell, workspace, and environment via `/etc/tau/manifest.json`.

## Scope

In scope:
- `Manifest` type matching the JSON shape in `_project/README.md` (version, name, description, base, shell, workspace, env, tools, services)
- `Parse(io.Reader) (*Manifest, error)` and `Validate(*Manifest) error`
- Version constant; only `version: "1"` accepted in Phase 1; unknown versions return typed error
- `Fallback() *Manifest` returning POSIX-shell defaults for images without a manifest
- `tests/manifest_test.go` with golden JSON fixtures covering parse, validate, version mismatch, missing fields, fallback

Out of scope:
- The mechanism for reading the manifest from a running container (Obj 3 — uses `Runtime.CopyFrom`)
- Tool definition generation from manifest entries (Phase 2)
- Custom JSON-Schema parameter declarations (open question, deferred)

## Repositories

`tailored-agentic-units/container` (root module only)

## Dependencies

None. Independent of Obj 1; can proceed in parallel.
```

### Objective 3 — Docker Runtime Implementation

```
## Objective

Implement the `Runtime` interface against the Docker Engine API as the `container/docker` sub-module. Validates the interface design end-to-end and provides the first functional execution environment for the container library.

## Scope

In scope:
- `container/docker` sub-module with own `go.mod` requiring `protocol` and root `container`
- Docker Engine API client wiring
- Implementation of all 8 `Runtime` methods (Create, Start, Stop, Remove, Exec, CopyTo, CopyFrom, Inspect)
- `tau.managed=true` and `tau.manifest.version=1` label convention applied at `Create`
- `Inspect` reads `/etc/tau/manifest.json` via `CopyFrom`, parses it via Obj 2's `manifest.Parse`, and embeds the result in `ContainerInfo`
- Explicit `Register()` function (no `init()`)
- Integration tests in `docker/tests/` that skip gracefully when Docker is unreachable
- `examples/docker-hello/main.go` demonstrating end-to-end usage

Out of scope:
- Containerd/Podman implementation (future sub-module)
- Volume mounts beyond `CopyTo`/`CopyFrom` (open question, deferred)
- Resource limits, health checks, networking (Phase 3)

## Repositories

`tailored-agentic-units/container` (`docker/` sub-module and `examples/`)

## Dependencies

Depends on Obj 1 (Runtime interface, types, registry) and Obj 2 (Manifest types, Parse).
```

## Proposed `_project/phase.md`

```markdown
# Phase 1 — Runtime Foundation

**Version target:** v0.1.0

## Scope

Establish the OCI-aligned `Runtime` interface, the image capability manifest convention, and the first runtime implementation (Docker). Together these provide a minimal but complete container-execution foundation that downstream phases (toolkit-mode tool bridge, image management, embedded mode) build on.

## Objectives

| # | Objective | Status |
|---|-----------|--------|
| 1 | Runtime Interface & Core Types | Planned |
| 2 | Image Capability Manifest | Planned |
| 3 | Docker Runtime Implementation | Planned |

## Constraints

- Objectives 1 and 2 are independent and may proceed in parallel
- Objective 3 depends on both Obj 1 (interface) and Obj 2 (manifest types)
- Single root `go.mod` tagged `v0.1.0` at phase close; `docker/` sub-module tagged independently as `docker/v0.1.0`
- Sub-module convention: explicit `Register()` (no `init()`); root never imports sub-modules
- `tools.go` and `shell.go` from the README package layout are deferred to Phase 2

## Cross-cutting decisions

- **Context cancellation**: cancel during `Exec` kills the exec instance; cancel during `CopyTo`/`CopyFrom` aborts the stream; `Stop` honors its own `timeout` independently of `ctx`
- **Label convention**: `tau.managed=true` and `tau.manifest.version=<v>` reserved for tau-managed containers
- **Manifest read path**: `Runtime.CopyFrom("/etc/tau/manifest.json")` — runtime-agnostic, validates `CopyFrom` on every start
- **Manifest version negotiation**: Phase 1 accepts only `version: "1"`; unknown versions return a typed error; missing manifest returns a documented fallback (POSIX shell, no declared tools)
```

## Proposed `.claude/CLAUDE.md`

Modeled on `~/tau/kernel/.claude/CLAUDE.md` (lean, tau-ecosystem idioms) with container-specific adaptations. Reflects Phase 1 state; will evolve as code lands.

```markdown
# container

TAU container library — OCI-aligned runtime abstraction for local-first, portable agent execution environments. See `_project/README.md` for vision, architecture, and phase roadmap.

## Modules

```
github.com/tailored-agentic-units/container          # root: Runtime interface, types, manifest
github.com/tailored-agentic-units/container/docker   # sub-module: Docker Engine API implementation
```

Multi-module layout. Root exposes interfaces with zero heavy dependencies; sub-modules implement runtimes and carry their own transitive dependencies. Root never imports sub-modules.

## Dependency Position

```
protocol (L0) ─── format (L1) ─── agent (L2) ─── kernel (L3)
                     │                               │
                     └──── container (L2.5) ─────────┘
```

Root module depends only on `tau/protocol` and `tau/format`. No Go module dependency on `agent` or `kernel` — kernel binaries enter container images at Docker build time, not as Go imports.

## Project Structure

```
container/
├── _project/          # Phase and concept docs (README.md, phase.md, objective.md)
├── .claude/           # Claude Code configuration, plans, context, skills
├── runtime.go         # Runtime interface + registry (Phase 1)
├── container.go       # Container type, State, CreateOptions, ExecOptions, ExecResult, ContainerInfo
├── manifest.go        # Image capability manifest types + Parse/Validate/Fallback
├── errors.go          # Domain error types
├── tests/             # Root module tests (black-box)
├── docker/            # Docker sub-module
│   ├── go.mod
│   ├── docker.go      # Runtime implementation + Register()
│   └── tests/         # Integration tests (skip gracefully when Docker unavailable)
└── examples/          # Runnable examples (docker-hello, etc.)
```

`tools.go` and `shell.go` appear in the README package layout but are Phase 2 work.

## Design Principles

- Every package's exported data structures are its protocol. Higher-level consumers use those types natively — no wrapping, no re-definition.
- Lower-level packages define contracts; higher-level packages implement them. Dependencies flow downward only.
- Address gaps at the lowest affected dependency level rather than working around them at higher levels.
- Sub-module `Register()` is explicit — no `init()` auto-registration. Root registry is thread-safe (mirrors `provider/registry.go`).
- Runtime methods accept `context.Context`. Cancellation aborts in-flight exec/copy streams; `Stop` honors its own timeout independently of `ctx`.

## Container Conventions

- **Labels**: tau-managed containers carry `tau.managed=true` and `tau.manifest.version=<v>`. The `tau.*` namespace is reserved for container metadata.
- **Manifest location**: `/etc/tau/manifest.json` inside the image. Read via `Runtime.CopyFrom` — runtime-agnostic.
- **Manifest versioning**: Phase 1 accepts only `version: "1"`. Unknown versions return a typed error. Missing manifest returns a documented POSIX-shell fallback.

## Testing

- Tests live in `tests/` at each module level (root and per sub-module)
- Black-box only (`package <name>_test`)
- Table-driven for parameterized cases
- Sub-module integration tests skip gracefully when the runtime is unreachable (e.g., no Docker daemon)
- Target 80% coverage on critical paths (types, registry, manifest parsing, lifecycle state transitions)

## Versioning

Root and sub-modules tag independently:
- Root: `v<major>.<minor>.<patch>` (e.g., `v0.1.0`)
- Docker: `docker/v<major>.<minor>.<patch>` (e.g., `docker/v0.1.0`)
- Dev pre-release: `v<target>-dev.<objective>.<issue>`

## Context Documents

Project knowledge artifacts stored in `.claude/context/`:

| Directory | Contents | Naming |
|-----------|----------|--------|
| `concepts/` | Architectural concept documents | `[slug].md` |
| `guides/` | Active implementation guides | `[issue-number]-[slug].md` |
| `sessions/` | Session summaries | `[issue-number]-[slug].md` |
| `reviews/` | Project review reports | `[YYYY-MM-DD]-[scope].md` |

Concepts, guides, and sessions have `.archive/` subdirectories. Reviews are permanent. Directories created on demand.

## Task Session: Documentation Review

During a task execution session, the documentation phase must review project context documents for revisions necessitated by the implementation. Check and update as needed:

- `_project/README.md` — package layout, manifest shape, architecture descriptions
- `_project/phase.md` — objective statuses, cross-cutting decisions
- `_project/objective.md` — sub-issue statuses
- `.claude/CLAUDE.md` — project structure, module list, conventions

Runs before closeout.

## Session Continuity

Plan files in `.claude/plans/` enable session continuity across machines. When pausing, append a **Context Snapshot** to the active plan file capturing current state, files modified, next steps, key decisions, and blockers. Restore by reading the most recent snapshot and resuming from documented next steps.

## Go Conventions

Defer to `~/code/claude-context/rules/go-principles.md` (loaded globally). Container-specific reinforcements:

- **Interface location**: `Runtime` defined in root (`runtime.go`), implemented in sub-modules — never the reverse.
- **Parameter encapsulation**: `Runtime` methods accepting options use struct parameters (`CreateOptions`, `ExecOptions`). Never more than two positional parameters.
- **Error wrapping**: Docker API errors wrapped with operation context (`fmt.Errorf("docker create: %w", err)`). Domain errors in `errors.go` with `Err` prefix.

## Gotchas

- **Sub-module go.mod dependencies**: Sub-modules require both `tau/protocol` and root `tau/container` explicitly. No `replace` directives — tags resolve through the module proxy.
- **Manifest read cost**: Every `Inspect` that populates `Manifest` pays a `CopyFrom` round-trip. Callers should cache `ContainerInfo` if they need repeat access.
- **Label filtering**: When listing tau-managed containers (future work), filter on `tau.managed=true` — do not rely on image names.
```

## Execution Steps (after plan approval)

1. **Create three Objective issues** on `tailored-agentic-units/container` with:
   - Title prefix `Objective: `
   - Label `objective`
   - Milestone `Phase 1 - Runtime Foundation`
   - Issue type `Objective` (via GraphQL `updateIssueIssueType`)
   - Bodies as drafted above
2. **Add each issue to project board #9** and set the `Phase` field to `Phase 1 - Runtime Foundation` (option ID `1be1afec`).
3. **Write `_project/phase.md`** using the content drafted above.
4. **Write `.claude/CLAUDE.md`** using the content drafted above.
5. **Commit** the new `_project/phase.md`, `.claude/CLAUDE.md`, and the staged whitespace fix in `_project/README.md` on a `phase-planning` branch and open a PR.

No `Step 0` transition closeout because no previous phase exists.

## Verification

- `gh issue list --repo tailored-agentic-units/container --label objective` lists three Objective issues, each with the milestone set
- `gh project item-list 9 --owner tailored-agentic-units --format json | jq '.items[].content.title'` shows all three on the board with `Phase 1 - Runtime Foundation` in the Phase field
- `_project/phase.md` exists with the objectives table matching the created issue numbers
- `.claude/CLAUDE.md` exists and accurately describes the current (Phase 1) module layout
- `gh api repos/tailored-agentic-units/container/milestones/1 --jq '.open_issues'` reports `3`

## Critical Files

- `/home/jaime/tau/container/_project/README.md` — vision, package layout, Runtime interface signature, manifest JSON shape
- `/home/jaime/tau/container/_project/phase.md` — to be created
- `/home/jaime/tau/provider/registry.go` — registry pattern to mirror in `container/registry.go`
- `/home/jaime/tau/provider/azure/azure.go` — `Register()` convention reference
- `/home/jaime/tau/provider/azure/go.mod` — sub-module `go.mod` shape reference
- `/home/jaime/tau/kernel/_project/phase.md` — phase.md format reference
- `/home/jaime/tau/tau-platform/EXECUTION-PLAN.md` — confirms B1 scope and v0.1.0 target
