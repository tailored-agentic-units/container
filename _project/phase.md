# Phase 1 — Runtime Foundation

**Version target:** v0.1.0

## Scope

Establish the OCI-aligned `Runtime` interface, the image capability manifest convention, and the first runtime implementation (Docker). Together these provide a minimal but complete container-execution foundation that downstream phases (toolkit-mode tool bridge, image management, embedded mode) build on.

## Objectives

| # | Issue | Objective | Status |
|---|-------|-----------|--------|
| 1 | [#1](https://github.com/tailored-agentic-units/container/issues/1) | Runtime Interface & Core Types | Done |
| 2 | [#2](https://github.com/tailored-agentic-units/container/issues/2) | Image Capability Manifest | In Progress |
| 3 | [#3](https://github.com/tailored-agentic-units/container/issues/3) | Docker Runtime Implementation | Planned |

## Constraints

- Objectives 1 and 2 are independent and may proceed in parallel
- Objective 3 depends on both Obj 1 (interface) and Obj 2 (manifest types)
- Single root `go.mod` tagged `v0.1.0` at phase close; `docker/` sub-module tagged independently as `docker/v0.1.0`
- Sub-module convention: explicit `Register()` (no `init()`); root never imports sub-modules
- `tools.go` and `shell.go` from the README package layout are deferred to Phase 2

## Cross-cutting decisions

- **Context cancellation**: cancel during `Exec` kills the exec instance; cancel during `CopyTo`/`CopyFrom` aborts the stream; `Stop` honors its own `timeout` independently of `ctx`
- **Label convention**: `tau.managed=true` and `tau.manifest.version=<v>` reserved for tau-managed containers
- **Manifest read path**: `Runtime.CopyFrom("/etc/tau/manifest.json")` — runtime-agnostic, validates `CopyFrom` on every start
- **Manifest version negotiation**: Phase 1 accepts only `version: "1"`; unknown versions return a typed error; missing manifest returns a documented fallback (POSIX shell, no declared tools)
- **`Register()` location**: inline in `docker/docker.go` (matches sibling packages `provider/azure`, `provider/bedrock`). The separate `register.go` shown in the README package layout is a documentation artifact and will be removed from the README in a future update.
