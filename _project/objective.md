# Objective 1 — Runtime Interface & Core Types

**Parent issue:** [#1](https://github.com/tailored-agentic-units/container/issues/1)
**Phase:** [Phase 1 — Runtime Foundation](./phase.md)
**Milestone:** Phase 1 - Runtime Foundation
**Status:** Planned

## Scope

Establish the root container module's foundational types and the OCI-aligned `Runtime` interface that all runtime implementations must satisfy. Provides the contract and the registry mechanism that sub-module implementations register against.

In scope:
- `Runtime` interface with the 8 methods (Create, Start, Stop, Remove, Exec, CopyTo, CopyFrom, Inspect)
- `Container`, `State`, `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo` types
- Domain error types (`ErrRuntimeNotFound`, `ErrContainerNotFound`, `ErrInvalidState`)
- Thread-safe registry mirroring `provider/registry.go` (`Factory`, `Register`, `Create`, `ListRuntimes`)
- Context-cancellation contract documented on each interface method
- Root-module tests in `tests/`

Out of scope: any runtime implementation (Obj 3), manifest types (Obj 2), Phase 2 work.

## Acceptance Criteria

- Root module compiles with no transitive heavy dependencies (`go mod graph` shows only `protocol`/`format`/stdlib)
- `Runtime` interface fully documented; each method's godoc states its context-cancellation behavior
- Registry is concurrency-safe (covered by tests with parallel Register/Create)
- `errors.Is` works against `ErrRuntimeNotFound` from `Create`
- Black-box tests in `tests/` pass; `go vet ./...` clean

## Sub-issues

| # | Issue | Title | Depends on | Status |
|---|-------|-------|-----------|--------|
| 1 | [#5](https://github.com/tailored-agentic-units/container/issues/5) | Define container core types and domain errors | — | Done |
| 2 | [#6](https://github.com/tailored-agentic-units/container/issues/6) | Define Runtime interface and thread-safe registry | #5 | Done |

## Architecture decisions

### Factory signature: parameterless

```go
type Factory func() (Runtime, error)
```

Mirrors `format/registry.go` rather than `provider/registry.go` (which takes `*config.ProviderConfig`). Container has no analogous shared-config package — Docker, containerd, and future runtimes each have distinct config shapes. Sub-modules close over their own config in a wrapper:

```go
// in docker/docker.go (Obj 3)
func Register(opts Options) {
    container.Register("docker", func() (container.Runtime, error) {
        return newRuntime(opts)
    })
}
```

Keeps the root module free of runtime-specific config types and respects the project convention of explicit `Register()` (no `init()` auto-registration).

### Lifecycle states

```go
type State string
const (
    StateCreated State = "created"
    StateRunning State = "running"
    StateExited  State = "exited"
    StateRemoved State = "removed"
)
```

OCI-aligned. `paused` excluded for Phase 1 — no Pause method in the interface.

### Error types

Sentinel-style with `Err` prefix (per CLAUDE.md):
- `ErrRuntimeNotFound` — registry returns when an unknown name is requested; `Create` wraps with the requested name via `fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)` so callers can `errors.Is` it
- `ErrContainerNotFound` — runtime methods return when ID is missing
- `ErrInvalidState` — operation invalid for current lifecycle state

Sub-modules wrap these with `fmt.Errorf("docker create: %w", err)` (Obj 3 work).

### Parameter encapsulation

`Runtime.Create` takes `CreateOptions`; `Runtime.Exec` takes `ExecOptions`. Other methods take only `(ctx, id, ...)` with at most one extra positional parameter — no struct needed.

### Context cancellation contract (per phase decision)

- `Create`/`Start`/`Remove`/`Inspect` — cancel aborts the in-flight API call
- `Exec` — cancel kills the exec instance
- `CopyTo`/`CopyFrom` — cancel aborts the stream
- `Stop` — honors its own `timeout` independently of `ctx`; `ctx` cancellation only aborts the API call to initiate stop
