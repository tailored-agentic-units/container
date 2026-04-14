# Objective 2 — Image Capability Manifest

**Parent issue:** [#2](https://github.com/tailored-agentic-units/container/issues/2)
**Phase:** [Phase 1 — Runtime Foundation](./phase.md)
**Milestone:** Phase 1 - Runtime Foundation
**Status:** Done

## Scope

Implement the image capability manifest convention — types, parsing, validation, and fallback — so containers can declare their tools, services, shell, workspace, and environment via `/etc/tau/manifest.json`.

In scope:
- `Manifest` type matching the JSON shape in `_project/README.md` (version, name, description, base, shell, workspace, env, tools, services)
- `Parse(io.Reader) (*Manifest, error)` and `Validate(*Manifest) error`
- `ManifestVersion` constant; only `version: "1"` accepted in Phase 1; unknown versions return a typed error
- `Fallback() *Manifest` returning POSIX-shell defaults for images without a manifest
- `ContainerInfo.Manifest *Manifest` field (closes the TODO deferred from sub-issue #5)
- `tests/manifest_test.go` with golden JSON fixtures covering parse, validate, version mismatch, missing fields, malformed JSON, fallback

Out of scope:
- The mechanism for reading the manifest from a running container (Obj 3 — uses `Runtime.CopyFrom`)
- Tool definition generation from manifest entries (Phase 2)
- Custom JSON-Schema parameter declarations (open question, deferred)

## Acceptance Criteria

- `manifest.go` types match the README JSON shape; godoc on every exported identifier
- `Parse` returns a wrapped `ErrManifestInvalid` for decode errors and missing required fields
- `Parse` returns a wrapped `ErrManifestVersion` for version mismatch (verifiable via `errors.Is`)
- `Fallback()` returns a manifest that passes `Validate` (round-trip safe)
- `ContainerInfo.Manifest *Manifest` field added with godoc clarifying nil semantics
- Black-box tests in `tests/manifest_test.go` pass; `go vet ./...` clean
- `go mod graph` still shows only `protocol`/`format`/stdlib (no new heavy deps)

## Sub-issues

| # | Issue | Title | Depends on | Status |
|---|-------|-------|-----------|--------|
| 1 | [#9](https://github.com/tailored-agentic-units/container/issues/9) | Implement image capability manifest types and validation | — | Done |

## Architecture decisions

### Single sub-issue

Scope is tightly coupled (~150 LOC of types + parse/validate/fallback in one file plus one cohesive test file). Splitting types from parse/validate would leave the first PR with no testable behavior. Mirrors the Phase 1 precedent of one PR per atomic, independently shippable unit of work.

### Manifest shape — verbatim from README

Field set matches the JSON example in `_project/README.md`. `Description`, `Base`, `Workspace`, `Env`, `Tools`, `Services` are `omitempty`; only `Version`, `Name`, and `Shell` are required for `Validate` to pass.

### Constants

```go
const (
    ManifestVersion = "1"
    ManifestPath    = "/etc/tau/manifest.json"
)
```

`ManifestPath` is exported so Obj 3's Docker `Inspect` can pass it to `Runtime.CopyFrom` without re-declaring the path constant — keeps the well-known location single-sourced.

### Error sentinels

Two new error values in `errors.go` (sentinel-style with `Err` prefix per CLAUDE.md):

- `ErrManifestVersion` — version mismatch; lets callers distinguish "wrong version" from "malformed JSON" via `errors.Is`
- `ErrManifestInvalid` — decode failure or missing required field

### Fallback semantics

`Fallback()` returns a non-nil `*Manifest` with `Version: "1"`, `Name: "fallback"`, `Shell: "/bin/sh"` and no tools/services. The result must round-trip through `Validate`. Callers (Obj 3 `Inspect`) substitute this when `/etc/tau/manifest.json` is absent in the image.

### `ContainerInfo.Manifest` placement

The Manifest pointer lives on `ContainerInfo` (the `Inspect` return shape), not on `Container` (the `Create` return shape). Manifest read happens at inspect time via `CopyFrom`, not at create time — populating it on `Container` would require an extra API round-trip during `Create` that callers may not want.
