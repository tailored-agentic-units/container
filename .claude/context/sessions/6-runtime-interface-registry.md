# 6 — Define Runtime interface and thread-safe registry

## Summary

Landed the OCI-aligned `Runtime` interface (8 methods) and the package-level factory registry that runtime sub-modules will register against in Objective #3. With this work, Objective #1 (Runtime Interface & Core Types) is functionally complete.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| File organization | Split `runtime.go` (interface) and `registry.go` (Factory + registry) | Single-responsibility per file; keeps the interface definition uncluttered by registry plumbing. Deviated from the implementation guide's single-file layout. |
| Factory shape | Parameterless `func() (Runtime, error)` | Mirrors `format/registry.go`. Each runtime sub-module closes over its own config in a wrapper, keeping the root module free of runtime-specific config types. |
| Registry pattern | Package-private struct + `sync.RWMutex` + global `register` | Mirrors `provider/registry.go` and `format/registry.go` exactly — established TAU convention. Last-write-wins on duplicate `Register`. |
| `ErrRuntimeNotFound` wrapping | `fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)` | `%w` preserves identity for `errors.Is`; appending the requested name aids debugging. |
| Context cancellation contract | Documented per-method in godoc on `Runtime` | Per the Phase 1 cross-cutting decision. `Stop` honors its own timeout independently of `ctx`; `Exec` cancellation kills the exec instance; `CopyTo`/`CopyFrom` cancellation aborts the stream. |

## Files Modified

- `runtime.go` (new) — `Runtime` interface with 8 methods and per-method cancellation godoc
- `registry.go` (new) — `Factory` type, package-private `registry` struct, `Register`/`Create`/`ListRuntimes`
- `tests/registry_test.go` (new) — black-box registry tests including concurrent Register/Create/List
- `_project/README.md` — added `registry.go` to the package layout block
- `_project/objective.md` — marked sub-issue #6 as Done
- `.claude/CLAUDE.md` — updated project structure to reflect the runtime/registry split
- `CHANGELOG.md` — added `v0.1.0-dev.1.6` entry

## Patterns Established

- Runtime sub-modules will expose their own `Register(opts)` helper that wraps `container.Register(name, factory)` (see Obj #3 plan in objective.md).
- Tests against the global registry use `t.Name()`-derived unique names to avoid cross-test interference — required when testing package-level mutable state.

## Validation Results

- `go test ./tests/...` — passes
- `go test -race ./tests/...` — passes
- `go vet ./...` — clean
- `gofmt -l .` — clean
- All acceptance criteria from issue #6 met
