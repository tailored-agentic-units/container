# Plan — Issue #6: Define Runtime interface and thread-safe registry

## Context

Issue #6 lands the second sub-issue of Objective #1 (Runtime Interface & Core Types). #5 established `Container`, `State`, the option/result structs, and sentinel errors. #6 adds the OCI-aligned `Runtime` interface (8 methods) and the package-level factory registry that runtime sub-modules (Docker, containerd) will register against in Objective #3.

The issue body fully specifies the interface signature, the registry shape, the factory rationale, and acceptance criteria. The two precedent files (`provider/registry.go`, `format/registry.go`) confirm the exact pattern to mirror — `format/registry.go` is the closer match because container's `Factory` is parameterless.

## Approach

Add a single source file `runtime.go` to the root module, plus a single test file `tests/registry_test.go`. No new sub-directories, no changes to existing files.

### `runtime.go` structure

Imports: `context`, `fmt`, `io`, `sync`, `time`.

1. **`Runtime` interface** — 8 methods exactly as specified in the issue:
   - `Create(ctx, opts CreateOptions) (*Container, error)`
   - `Start(ctx, id string) error`
   - `Stop(ctx, id string, timeout time.Duration) error`
   - `Remove(ctx, id string, force bool) error`
   - `Exec(ctx, id string, opts ExecOptions) (*ExecResult, error)`
   - `CopyTo(ctx, id, dst string, content io.Reader) error`
   - `CopyFrom(ctx, id, src string) (io.ReadCloser, error)`
   - `Inspect(ctx, id string) (*ContainerInfo, error)`

   Each method's godoc states its context-cancellation behavior per `_project/phase.md` cross-cutting decision.

2. **`Factory` type** — `type Factory func() (Runtime, error)` (parameterless, matching `format/registry.go`).

3. **Registry** — package-private struct guarded by `sync.RWMutex`:
   ```go
   type registry struct {
       factories map[string]Factory
       mu        sync.RWMutex
   }
   var register = &registry{factories: make(map[string]Factory)}
   ```

4. **Registry functions** — `Register(name, factory)`, `Create(name) (Runtime, error)`, `ListRuntimes() []string`. `Create` returns `fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)` on miss so `errors.Is` works.

### `tests/registry_test.go` structure

Black-box (`package container_test`). Tests:

1. `TestRegister_AndCreate` — register a stub factory, retrieve via `Create`, verify factory was invoked.
2. `TestCreate_UnknownName` — `Create("missing")` returns error; `errors.Is(err, container.ErrRuntimeNotFound)` is true; error message includes the requested name.
3. `TestRegister_Overwrite` — registering the same name twice, `Create` returns the second factory's value (last wins, matches provider pattern).
4. `TestListRuntimes` — register N factories under unique names, `ListRuntimes()` returns all names (order-independent comparison).
5. `TestRegistry_Concurrent` — goroutines registering and calling `Create` in parallel under `t.Parallel`; passes `-race` cleanly.
6. `TestFactory_Error` — factory returning an error propagates through `Create`.

Tests use unique `t.Name()`-derived registry names to avoid cross-test interference (the registry is global package state). A stub `Runtime` impl satisfies the interface inside the test file (typed nil receiver methods returning nil).

## Files

| File | Action | Notes |
|------|--------|-------|
| `runtime.go` | new | Interface + Factory + registry + Register/Create/ListRuntimes |
| `tests/registry_test.go` | new | Black-box registry behavior + concurrency |

No edits to `container.go`, `errors.go`, `go.mod`, or existing tests.

## Reference

- `format/registry.go` — closest precedent (parameterless Factory). Mirror the registry struct, package-private `register` global, lock discipline.
- Issue #6 body — interface signatures, Factory rationale, AC checklist.
- `_project/phase.md` cross-cutting decisions — context-cancellation contract documented per method in godoc.

## Verification

```bash
cd /home/jaime/tau/container
go vet ./...
go test ./tests/...
go test -race ./tests/...
```

Acceptance:
- `go vet ./...` clean
- All tests pass under `-race`
- Black-box `package container_test` (no internal access)
- `errors.Is(err, container.ErrRuntimeNotFound)` returns true for unknown-name lookups
