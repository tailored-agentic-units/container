# 5 — Define container core types and domain errors

## Problem Context

The `container` root module has no Go code yet — only `_project/` docs and `README.md`. This sub-issue bootstraps `go.mod` and introduces the package's foundational data model: six exported types (`Container`, `State`, `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo`) and three sentinel errors (`ErrRuntimeNotFound`, `ErrContainerNotFound`, `ErrInvalidState`).

Sub-issue #6 will define the `Runtime` interface and registry in terms of these types. Keeping types/errors in a dedicated PR lets naming and shape decisions land cleanly before the interface is set.

## Architecture Approach

- **State**: OCI-aligned string type with four constants (`created`, `running`, `exited`, `removed`). `paused` excluded — no Pause method in Phase 1.
- **Container vs ContainerInfo**: two distinct structs even though Phase 1 shapes overlap. `Container` is the handle returned by `Runtime.Create`; `ContainerInfo` is the richer `Runtime.Inspect` return, which will grow timestamps, exit codes, and network info in later phases.
- **Manifest field on `ContainerInfo`**: deferred to Objective #2. Obj #2 will add the field as an additive change once `manifest.Manifest` exists.
- **Error style**: sentinel `errors.New(...)` values. Matches the `Err` prefix convention in CLAUDE.md and lets callers use `errors.Is`. Sub-issue #6's registry will wrap `ErrRuntimeNotFound` with `fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)`.
- **`go.mod`**: module declaration only, no `require` entries yet. No imports from `protocol` or `format` in this PR.

## Implementation

### Step 1: Initialize `go.mod`

Create `/home/jaime/tau/container/go.mod`:

```
module github.com/tailored-agentic-units/container

go 1.26
```

### Step 2: Create `container.go`

Create `/home/jaime/tau/container/container.go`:

```go
package container

type State string

const (
	StateCreated State = "created"
	StateRunning State = "running"
	StateExited  State = "exited"
	StateRemoved State = "removed"
)

type Container struct {
	ID     string
	Name   string
	Image  string
	State  State
	Labels map[string]string
}

type CreateOptions struct {
	Image      string
	Name       string
	Cmd        []string
	Env        map[string]string
	WorkingDir string
	Labels     map[string]string
}

type ExecOptions struct {
	Cmd          []string
	Env          map[string]string
	WorkingDir   string
	AttachStdin  bool
	AttachStdout bool
	AttachStderr bool
}

type ExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

type ContainerInfo struct {
	ID     string
	Name   string
	Image  string
	State  State
	Labels map[string]string
}
```

Godoc comments are intentionally omitted — they will be added during Phase 7 (Documentation).

### Step 3: Create `errors.go`

Create `/home/jaime/tau/container/errors.go`:

```go
package container

import "errors"

var (
	ErrRuntimeNotFound   = errors.New("container: runtime not found")
	ErrContainerNotFound = errors.New("container: container not found")
	ErrInvalidState      = errors.New("container: invalid state")
)
```

## Validation Criteria

From `/home/jaime/tau/container`:

- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean
- [ ] `go mod graph` shows only the module itself (no external requires)
- [ ] All six types defined in `container.go`: `State` (with four constants), `Container`, `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo`
- [ ] All three sentinel errors defined in `errors.go` with `Err` prefix
- [ ] Package compiles as `package container`
