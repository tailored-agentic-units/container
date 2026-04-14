# Issue #5 — Define container core types and domain errors

## Context

First Go code in the `container` root module. Foundational sub-issue of Objective #1 (Runtime Interface & Core Types) — produces the data model and sentinel errors that sub-issue #6 (Runtime interface + registry) will be defined in terms of. Repo currently has only `_project/` docs and `README.md`; this PR bootstraps `go.mod` and introduces the package.

Scope is verbatim from the issue body and matches the plan recorded in `_project/objective.md` and `.claude/plans/delightful-doodling-yeti.md` (objective-planning working doc). No exploration required — design decisions are already settled.

## Files to create

- `/home/jaime/tau/container/go.mod` — `module github.com/tailored-agentic-units/container`, `go 1.26`
- `/home/jaime/tau/container/container.go` — six exported types
- `/home/jaime/tau/container/errors.go` — three sentinel errors
- `/home/jaime/tau/container/tests/types_test.go` — black-box table-driven tests

## Design

### `container.go`

Six exported types, godoc on every identifier.

```go
package container

// State — OCI-aligned lifecycle string type
type State string
const (
    StateCreated State = "created"
    StateRunning State = "running"
    StateExited  State = "exited"
    StateRemoved State = "removed"
)

// Container — handle returned by Runtime.Create
type Container struct {
    ID     string
    Name   string
    Image  string
    State  State
    Labels map[string]string
}

// CreateOptions — Runtime.Create input. `tau.managed=true` label SHOULD be set for tau-managed containers.
type CreateOptions struct {
    Image      string
    Name       string
    Cmd        []string
    Env        map[string]string
    WorkingDir string
    Labels     map[string]string
}

// ExecOptions — Runtime.Exec input
type ExecOptions struct {
    Cmd          []string
    Env          map[string]string
    WorkingDir   string
    AttachStdin  bool
    AttachStdout bool
    AttachStderr bool
}

// ExecResult — Runtime.Exec output
type ExecResult struct {
    ExitCode int
    Stdout   []byte
    Stderr   []byte
}

// ContainerInfo — Runtime.Inspect output. Manifest field deferred to Obj #2 (additive change).
type ContainerInfo struct {
    ID     string
    Name   string
    Image  string
    State  State
    Labels map[string]string
}
```

### `errors.go`

```go
package container

import "errors"

var (
    ErrRuntimeNotFound   = errors.New("container: runtime not found")
    ErrContainerNotFound = errors.New("container: container not found")
    ErrInvalidState      = errors.New("container: invalid state")
)
```

Sentinel style (`errors.New`), `Err` prefix per CLAUDE.md. Registry in sub-issue #6 will wrap with `fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)`; sub-modules will wrap with `fmt.Errorf("docker create: %w", err)`.

### `tests/types_test.go`

Black-box (`package container_test`), table-driven.

- `TestState_Constants` — verify each `State` constant's underlying string value
- `TestState_StringConversion` — verify `string(s)` round-trips as documented
- `TestDomainErrors_Identity` — verify each sentinel is non-nil, has a descriptive message, and is distinguishable via `errors.Is` (each sentinel is itself; sentinels are not equal to each other)

### `go.mod`

```
module github.com/tailored-agentic-units/container

go 1.26
```

No `require` entries — this PR introduces no imports beyond stdlib (`errors` only). `protocol`/`format` requires will arrive when later sub-issues actually import them. Satisfies acceptance criterion "`go mod graph` shows only protocol/format/stdlib" (only-stdlib is a valid subset).

## Key design decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| `ContainerInfo.Manifest` field | Omitted in this PR | Cleaner than `any` placeholder or forward-declared stub. Obj #2 adds it as an additive change when `manifest.Manifest` exists. |
| `Container` vs `ContainerInfo` overlap | Kept as two distinct structs | Shapes look identical today but diverge later: `ContainerInfo` will carry timestamps, exit codes, network info. Keeps Create/Inspect return semantics explicit. |
| Error style | Sentinel `errors.New` | Matches project convention (`Err` prefix in CLAUDE.md); simplifies `errors.Is`. Typed struct errors only needed when carrying structured context — not the case here. |
| `go.mod` requires | None yet | No imports in this PR. `go mod tidy` would strip speculative requires anyway. |
| `go.work` inclusion | Out of scope | Workspace-level change. Flag to developer during closeout; don't touch in this PR. |

## Reference implementations (study, do not import)

- `/home/jaime/tau/provider/registry.go` — error style (`fmt.Errorf("unknown provider: %s", c.Name)` — we upgrade to `%w` wrapping per Obj #1 acceptance)
- `/home/jaime/tau/provider/tests/registry_test.go` — black-box pattern, table-driven shape

## Verification

From `/home/jaime/tau/container`:

```bash
go build ./...
go vet ./...
go test ./tests/...
go mod graph    # should show only stdlib
```

Acceptance-criteria mapping:

- [x] `container.go` defines all six types with godoc on every exported identifier
- [x] `errors.go` defines the three sentinel errors with documented semantics
- [x] `go.mod` initialized; `go build ./...` passes
- [x] `tests/types_test.go` runs as `package container_test` with table-driven cases for `State`
- [x] `go vet ./...` clean
- [x] No transitive heavy dependencies (only stdlib, satisfies "protocol/format/stdlib")

## Notes for closeout

- Flag to developer: add `./container` to `/home/jaime/tau/go.work` after PR merges so the workspace picks up the module for cross-module development. Out of scope for this PR.
- Implementation guide to be written at `.claude/context/guides/5-container-core-types.md` after plan approval.
