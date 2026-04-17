# container

TAU container library — OCI-aligned runtime abstraction for local-first, portable agent execution environments. See `_project/README.md` for vision, architecture, and phase roadmap.

## Status

**Paused. Reference artifact for the `tau/runtime` migration.**

Phase 1 shipped as `v0.1.0`. Phase 2 Objective #18 landed (`ExecStream`, PTY-attached `Shell`); remainder of Phase 2 and all of Phase 3 are **not being pursued here**. The universal kernel-runtime contract work has moved to [`tau/runtime`](https://github.com/tailored-agentic-units/runtime). When the runtime contract is finished and a `native` reference implementation is proven, `tau/runtime/container` + `tau/runtime/container/docker` will be initialized fresh against that contract, using this library's `_project/`, `.claude/context/`, and implementation source as design references. No code is moved. Once that transition completes (Phase 6 of `~/tau/tau-platform/EXECUTION-PLAN.md`), this repo is deleted locally and on GitHub.

**Do not add new code here.** Documentation revisions to reflect evolving understanding are fine. Active container-runtime development resumes in `tau/runtime/container` during Phase 6.

The L2.5 positioning claimed below ("Dependency Position") is superseded — see `_project/README.md` for the revised architecture.

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
├── runtime.go         # Runtime interface
├── registry.go        # Factory type + thread-safe registry (Register/Create/ListRuntimes)
├── container.go       # Container type, State, CreateOptions, ExecOptions, ExecResult, ContainerInfo
├── exec.go            # ExecStreamOptions, ExecSession (Phase 2 streaming exec primitive)
├── shell.go           # Shell, ShellOptions, NewShell (PTY-attached persistent shell)
├── manifest.go        # Image capability manifest types + Parse/Validate/Fallback
├── errors.go          # Domain error types
├── tests/             # Root module tests (black-box)
└── docker/            # Docker sub-module
    ├── go.mod
    ├── doc.go         # Package godoc
    ├── docker.go      # Runtime implementation + Register() + label constants
    ├── exec.go        # ExecStream implementation + execStream/execStdin/eofReader helpers
    └── tests/         # Integration tests (skip gracefully when Docker unavailable)
```

`tools.go` appears in the README package layout but is Phase 2 work. `exec.go` (root) and `docker/exec.go` landed with Phase 2 Obj #18 sub-issue #22; `shell.go` landed with sub-issue #23, completing Obj #18. Runnable examples live in `tailored-agentic-units/examples` (the cross-repo integration module that consumes tagged releases) — there is no `examples/` directory inside this repo.

## Design Principles

- Every package's exported data structures are its protocol. Higher-level consumers use those types natively — no wrapping, no re-definition.
- Lower-level packages define contracts; higher-level packages implement them. Dependencies flow downward only.
- Address gaps at the lowest affected dependency level rather than working around them at higher levels.
- Sub-module `Register()` is explicit — no `init()` auto-registration. Root registry is thread-safe (mirrors `provider/registry.go`).
- Runtime methods accept `context.Context`. Cancellation aborts in-flight exec/copy streams; `Stop` honors its own timeout independently of `ctx`.

## Container Conventions

- **Labels**: tau-managed containers carry `tau.managed=true` and `tau.manifest.version=<v>`. The `tau.*` namespace is reserved for container metadata.
- **Manifest location**: `/etc/tau/manifest.json` inside the image (exported as `ManifestPath`). Read via `Runtime.CopyFrom` — runtime-agnostic.
- **Manifest versioning**: Phase 1 accepts only `version: "1"` (`ManifestVersion`). Unknown versions return `ErrManifestVersion`. Missing manifest returns the documented POSIX-shell fallback via `Fallback()`.
- **Manifest decode is strict**: `Parse` calls `DisallowUnknownFields`. Any top-level field outside the declared schema is an `ErrManifestInvalid`. Runtime- or image-specific configuration that tau does not interpret belongs under the top-level `options` slot — the single sanctioned pass-through, mirroring the `Options map[string]any` convention at `tau/protocol/config` and `tau/format`.

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
