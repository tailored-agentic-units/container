# Objective Planning — Obj #1: Runtime Interface & Core Types

## Context

Phase 1 of the `container` repo establishes the OCI-aligned `Runtime` abstraction. Objective #1 lays the foundation: the contract and data model that Obj #2 (manifest types) and Obj #3 (Docker implementation) build on.

The repo currently has only `_project/` docs and a top-level `README.md` — no Go files yet. This objective produces the first Go code in the root module.

Issue: https://github.com/tailored-agentic-units/container/issues/1
Repo: `tailored-agentic-units/container` (root module only)
Milestone: `Phase 1 - Runtime Foundation`

No previous `_project/objective.md` exists, so no transition closeout is needed.

## Scope (verbatim from issue body)

In scope:
- `Runtime` interface with 8 methods (Create, Start, Stop, Remove, Exec, CopyTo, CopyFrom, Inspect)
- `Container`, `State`, `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo` types
- Domain error types
- Thread-safe registry mirroring `provider/registry.go` (`Factory`, `Register`, `Create`, `ListRuntimes`)
- Context-cancellation contract documented on each interface method
- Root-module tests in `tests/`

Out of scope: any runtime implementation (Obj #3), manifest types (Obj #2), Phase 2 work.

## Architectural decisions

### Registry pattern (mirrors `provider/registry.go`)

Provider's pattern (verified by exploration of `/home/jaime/tau/provider/registry.go`):
- `sync.RWMutex` on private registry struct
- `Register(name, factory)` — no return, overwrites silently
- `Create(...) (Runtime, error)` — `fmt.Errorf("unknown runtime: %s", name)` on miss
- `ListRuntimes() []string`

**Difference for container**: provider's `Factory` takes `*config.ProviderConfig`; container has no analogous shared-config package. Sub-modules each have their own config shape (Docker URL/TLS today; containerd socket tomorrow). Therefore:

```go
type Factory func() (Runtime, error)
```

Sub-modules close over their own config via a `Register(opts)` wrapper:

```go
// in docker/docker.go
func Register(opts Options) {
    container.Register("docker", func() (container.Runtime, error) {
        return newRuntime(opts)
    })
}
```

This matches the project convention "explicit `Register()` — no `init()` auto-registration" and keeps the root module free of any runtime-specific config types.

### Parameter encapsulation

`Runtime.Create` takes `CreateOptions` (struct), `Runtime.Exec` takes `ExecOptions` (struct). Aligns with `go-principles.md`: "More than 2 parameters? Use a struct." Other methods take only `(ctx, id, ...)` — at most one extra positional param, no struct needed.

### Context cancellation contract

Documented per-method on the interface (per `_project/phase.md` cross-cutting decision):
- `Create`/`Start`/`Remove`/`Inspect` — cancel aborts the in-flight API call
- `Exec` — cancel kills the exec instance
- `CopyTo`/`CopyFrom` — cancel aborts the stream
- `Stop` — honors its own `timeout` parameter independently of `ctx`; `ctx` cancellation only aborts the API call to initiate stop

### Error types

Domain errors in `errors.go` with `Err` prefix (per CLAUDE.md). Sentinel-style for predicates the registry/runtime distinguishes:
- `ErrRuntimeNotFound` — unknown runtime name in `Create`
- `ErrContainerNotFound` — Inspect/Start/Stop/Remove on missing container
- `ErrInvalidState` — operation invalid for current lifecycle state (e.g., Start on already-running)
- Registry uses `fmt.Errorf` wrapping where useful; sub-modules wrap Docker errors with `fmt.Errorf("docker create: %w", err)` (Obj #3 work, not this objective).

### Lifecycle states (`State` type)

```go
type State string
const (
    StateCreated State = "created"
    StateRunning State = "running"
    StateExited  State = "exited"
    StateRemoved State = "removed"
)
```

OCI-aligned. Docker's state model maps cleanly. `paused` excluded for Phase 1 — no Pause method in the interface.

## Sub-issue decomposition

**Recommendation: 2 sub-issues**, one branch / one PR each.

| # | Title | Depends on | Files |
|---|-------|-----------|-------|
| 1 | Core types & domain errors | — | `container.go`, `errors.go`, `tests/types_test.go` |
| 2 | Runtime interface & registry | #1 | `runtime.go`, `tests/registry_test.go` |

Rationale: types are the data model the interface signs in terms of. Reviewing them in isolation lets naming/shape decisions land cleanly before the interface is set. The registry has the only non-trivial logic and pairs naturally with the interface it dispatches.

Alternative considered: single sub-issue (everything in one PR, ~500-800 lines). Honest given how tightly coupled the pieces are. Rejected because the decomposition keeps each PR scannable in one sitting.

### Sub-issue #1: Core types & domain errors

- **Title**: `Define container core types and domain errors`
- **Labels**: `feature`
- **Issue type**: `Task`
- **Acceptance criteria**:
  - `container.go` defines `Container`, `State` (+ constants), `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo`
  - `errors.go` defines `ErrRuntimeNotFound`, `ErrContainerNotFound`, `ErrInvalidState` with documented semantics
  - `tests/types_test.go` covers `State` string conversions and any non-trivial type behavior
  - Black-box `package container_test`
  - Root `go.mod` initialized; depends only on `tau/protocol` and `tau/format`

### Sub-issue #2: Runtime interface & registry

- **Title**: `Define Runtime interface and thread-safe registry`
- **Labels**: `feature`
- **Issue type**: `Task`
- **Depends on**: #1
- **Acceptance criteria**:
  - `runtime.go` defines `Runtime` interface (8 methods) with godoc context-cancellation contract per method
  - `runtime.go` defines `Factory func() (Runtime, error)`, `Register`, `Create`, `ListRuntimes`
  - Registry uses `sync.RWMutex` on a package-private struct, mirroring `provider/registry.go`
  - `tests/registry_test.go` covers Register/Create/ListRuntimes including duplicate registration, unknown-name lookup, concurrent access
  - Black-box `package container_test`

## Critical files (to be created)

- `/home/jaime/tau/container/container.go`
- `/home/jaime/tau/container/runtime.go`
- `/home/jaime/tau/container/errors.go`
- `/home/jaime/tau/container/tests/types_test.go`
- `/home/jaime/tau/container/tests/registry_test.go`
- `/home/jaime/tau/container/go.mod`

## Reference implementations (reuse via study, not import)

- `/home/jaime/tau/provider/registry.go` — registry locking, Register/Create/List shape
- `/home/jaime/tau/format/registry.go` — parameterless `Factory` signature precedent
- `/home/jaime/tau/provider/tests/registry_test.go` — black-box test pattern, mock factory helper

## `_project/objective.md` content

To be created during sub-issue creation step:
- Objective title and parent issue link (#1)
- Phase reference (Phase 1)
- Scope and acceptance criteria (from issue body)
- Sub-issues table (the two above with status: Planned)
- Architecture decisions section (Factory signature choice, error types, lifecycle states)

## Verification

After both sub-issues land:
- `go build ./...` passes from repo root
- `go test ./tests/...` passes
- `go vet ./...` clean
- Importing `github.com/tailored-agentic-units/container` from a scratch program compiles with no transitive heavy deps (verify via `go mod graph`)
- Registry round-trip: register a stub Factory in a test, `Create` returns the expected Runtime, `ListRuntimes` includes the name

## GitHub operations to perform on approval

1. Create sub-issue #1 on `tailored-agentic-units/container` with the body above; assign `Task` issue type; link as sub-issue of #1
2. Create sub-issue #2 with body, dependency note pointing to #1, `Task` issue type; link as sub-issue of #1
3. Add both sub-issues to the project board (if one exists) under the same Phase as #1; assign Phase 1 milestone
4. Write `/home/jaime/tau/container/_project/objective.md`
