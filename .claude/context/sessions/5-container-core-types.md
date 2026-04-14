# 5 — Define container core types and domain errors

## Summary

First Go code in the `container` root module. Introduces the foundational data
model (`Container`, `State`, `CreateOptions`, `ExecOptions`, `ExecResult`,
`ContainerInfo`) and domain sentinel errors (`ErrRuntimeNotFound`,
`ErrContainerNotFound`, `ErrInvalidState`) that sub-issue #6 (`Runtime`
interface + registry) will build on. Bootstraps `go.mod` with no external
requires.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| `ContainerInfo.Manifest` field | Omitted | Cleaner as an additive change in Obj #2 (manifest types) than an `any` placeholder or forward-declared stub. |
| `Container` vs `ContainerInfo` overlap | Two distinct structs | Shapes look identical today but `ContainerInfo` will grow timestamps, exit codes, network settings, and the manifest pointer. Keeps Create/Inspect semantics explicit. |
| Error style | Sentinel `errors.New` with package-prefixed messages | Matches `Err` prefix convention in CLAUDE.md; keeps `errors.Is` simple. Sub-issue #6 wraps `ErrRuntimeNotFound` with `fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)`. |
| `go.mod` requires | None | No imports beyond stdlib in this PR. Satisfies "protocol/format/stdlib" as a stdlib-only subset. |
| Package godoc location | `container.go` top | Single package-level comment introduces the runtime abstraction; sub-module responsibilities noted inline. |

## Files Modified

- `go.mod` (new) — module declaration, `go 1.26`
- `container.go` (new) — package godoc + six exported types with godoc
- `errors.go` (new) — three sentinel errors with godoc
- `tests/types_test.go` (new) — black-box table-driven tests for `State` constants, `State` distinctness, domain error identity, and domain error distinctness
- `_project/objective.md` — sub-issue #5 status Planned → In Progress

## Patterns Established

- **Sentinel error style**: package-prefixed message (`container: runtime not found`), exported `Err`-prefixed var, grouped in a single `var ( ... )` block in `errors.go`. Sub-modules wrap with operation context.
- **Options struct naming**: `<Method>Options` (e.g., `CreateOptions`, `ExecOptions`) for Runtime methods that would otherwise take 3+ positional parameters.
- **Tests location**: `tests/` at module root, `package container_test` (black-box), table-driven — mirrors `provider/tests/` convention.

## Validation Results

- `go build ./...` — passes
- `go vet ./...` — clean
- `go test ./tests/...` — all 4 test groups pass (10 total sub-tests)
- `go mod graph` — container module shows only `go@1.26`; no external requires

## Follow-ups

- Add `./container` to `/home/jaime/tau/go.work` after this PR merges so the workspace picks up the module for cross-module development. Out of scope for this PR.
- Sub-issue #6 (`Runtime` interface + registry) unblocked once this lands.
- Objective #2 (manifest types) will add the `Manifest` field to `ContainerInfo` when it defines `manifest.Manifest`.
