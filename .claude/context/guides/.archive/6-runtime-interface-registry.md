# 6 — Define Runtime interface and thread-safe registry

## Problem Context

Sub-issue of Objective #1. With the core types and sentinel errors landed (#5), the package now needs the OCI-aligned `Runtime` contract that all runtime implementations must satisfy and the registry that runtime sub-modules register against. Together they form the foundation Objective #3 (Docker implementation) builds on.

## Architecture Approach

Single new file `runtime.go` at the root of the module. It defines:

1. The `Runtime` interface (8 methods) with per-method context-cancellation behavior captured in godoc.
2. The `Factory` type — parameterless, so each sub-module closes over its own runtime-specific config in a wrapper. Mirrors `format/registry.go` rather than `provider/registry.go`.
3. A package-private `registry` struct guarded by `sync.RWMutex`, plus the package-level functions `Register`, `Create`, and `ListRuntimes`.

`Create` wraps `ErrRuntimeNotFound` with the requested name via `fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)` so callers can use `errors.Is`.

No edits to `container.go`, `errors.go`, or `go.mod`.

## Implementation

### Step 1: Create `runtime.go`

New file at the module root. The file is shown without godoc — comments are added during the Documentation phase.

```go
package container

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

type Runtime interface {
	Create(ctx context.Context, opts CreateOptions) (*Container, error)
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string, timeout time.Duration) error
	Remove(ctx context.Context, id string, force bool) error
	Exec(ctx context.Context, id string, opts ExecOptions) (*ExecResult, error)
	CopyTo(ctx context.Context, id string, dst string, content io.Reader) error
	CopyFrom(ctx context.Context, id string, src string) (io.ReadCloser, error)
	Inspect(ctx context.Context, id string) (*ContainerInfo, error)
}

type Factory func() (Runtime, error)

type registry struct {
	factories map[string]Factory
	mu        sync.RWMutex
}

var register = &registry{
	factories: make(map[string]Factory),
}

func Register(name string, factory Factory) {
	register.mu.Lock()
	defer register.mu.Unlock()
	register.factories[name] = factory
}

func Create(name string) (Runtime, error) {
	register.mu.RLock()
	factory, exists := register.factories[name]
	register.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)
	}

	return factory()
}

func ListRuntimes() []string {
	register.mu.RLock()
	defer register.mu.RUnlock()

	names := make([]string, 0, len(register.factories))
	for name := range register.factories {
		names = append(names, name)
	}
	return names
}
```

## Validation Criteria

- [ ] `runtime.go` exists at the module root and defines `Runtime` with all 8 methods listed above.
- [ ] `Factory`, `Register`, `Create`, `ListRuntimes` are exported with the signatures shown.
- [ ] Registry struct uses `sync.RWMutex` and is package-private; the package-level singleton is `register`.
- [ ] `Create` returns an error wrapping `ErrRuntimeNotFound` for unknown names (the `%w` verb preserves identity for `errors.Is`).
- [ ] `go build ./...` succeeds.
- [ ] `go vet ./...` is clean.
