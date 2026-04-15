# Objective 3 — Docker Runtime Implementation

**Parent issue:** [#3](https://github.com/tailored-agentic-units/container/issues/3)
**Phase:** [Phase 1 — Runtime Foundation](./phase.md)
**Milestone:** Phase 1 - Runtime Foundation
**Status:** In Progress

## Scope

Implement the `Runtime` interface against the Docker Engine API as the `container/docker` sub-module. Validates the interface design end-to-end and delivers Phase 1's first functional execution environment. Cross-repo: library work lands on `tailored-agentic-units/container`; the runnable example lands on `tailored-agentic-units/examples` (blocked on Phase 1 release tags).

In scope:
- `container/docker` sub-module with its own `go.mod` requiring `protocol` (transitive) and root `container`, plus the Docker client SDK
- All 8 `Runtime` methods (Create, Start, Stop, Remove, Exec, CopyTo, CopyFrom, Inspect)
- `tau.managed=true` and `tau.manifest.version=<ManifestVersion>` labels applied at `Create`
- `Inspect` reads `ManifestPath` via `CopyFrom`, parses via `container.Parse`, populates `ContainerInfo.Manifest`
- Explicit parameterless `Register()` wiring a default `client.FromEnv` factory — no `init()`
- Integration tests in `docker/tests/` that skip gracefully when Docker is unreachable or images cannot be pulled
- `cmd/docker-hello/` runnable example in the examples repo, blocked on `v0.1.0` and `docker/v0.1.0` tags

Out of scope:
- Containerd / Podman runtimes (future sub-module)
- Volume mounts beyond `CopyTo`/`CopyFrom` (open question, deferred)
- Resource limits, health checks, networking (Phase 3)
- `RegisterWithClient` helper (deferred — YAGNI)

## Acceptance Criteria

- `docker/` sub-module builds and tests pass (`go build ./...`, `go vet ./...`, `go test ./docker/tests/...`)
- All 8 Runtime methods implemented with documented context/cancel semantics
- `Create` applies the reserved `tau.*` labels; caller labels merge but cannot override
- `Stop` honors its own `timeout` independently of `ctx`
- `Remove(force=false)` on running containers surfaces `ErrInvalidState` via `errors.Is`
- `Inspect` populates `ContainerInfo.Manifest` when a valid manifest is present; leaves nil when absent; surfaces `ErrManifestInvalid` / `ErrManifestVersion` via `errors.Is` on malformed / mismatched manifests
- Test suite skips gracefully when the Docker daemon is unreachable or the test image cannot be pulled
- `docker` package coverage ≥ 80% via `go test -coverpkg=.../docker ./docker/tests/...`
- `cmd/docker-hello/` example merges on the examples repo after release tags exist; demonstrates the nil-manifest path explicitly

## Sub-issues

| # | Issue | Title | Depends on | Repo | Status |
|---|-------|-------|-----------|------|--------|
| A | [#11](https://github.com/tailored-agentic-units/container/issues/11) | Docker sub-module scaffold and lifecycle methods | — | container | Done |
| B | [#12](https://github.com/tailored-agentic-units/container/issues/12) | Docker runtime Exec and file copy methods | #11 | container | Done |
| C | [#13](https://github.com/tailored-agentic-units/container/issues/13) | Docker runtime Inspect and manifest integration | #11, #12 | container | Done |
| D | [#14](https://github.com/tailored-agentic-units/container/issues/14) | docker-hello runnable example in examples repo | #11, #12, #13 + v0.1.0 tags | examples (tracked on container) | Todo, blocked on release |

Sub-issue D is tracked on the container repo so Obj #3's roll-up is honest, but the implementing PR opens against `tailored-agentic-units/examples` — that module consumes tagged releases, not workspace refs, which is why D is gated on the Phase 1 release session.

## Architecture decisions

### Decomposition along I/O seams

The Runtime surface splits into three natural cohorts: lifecycle (`Create`/`Start`/`Stop`/`Remove`), I/O (`Exec`/`CopyTo`/`CopyFrom`), and inspect-with-manifest (`Inspect` + manifest read). Each ships as one PR and each has testable behavior in isolation. Bundling all 8 methods into one PR was rejected for review burden; splitting `Exec` from Copy was rejected because all three share the streaming-cancellation pattern and the same tar-archive handling primitives.

### Default factory only

`Register()` is parameterless and wires a default factory that constructs a Docker client from `client.FromEnv` with API version negotiation. Tests that need a pre-configured client use the unexported `dockerRuntime` constructor directly. A `RegisterWithClient` helper is deferred until a caller asks for it.

### Label constants sourced from the root

`LabelManaged = "tau.managed"` and `LabelManifestVersion = "tau.manifest.version"` are exported from `docker.go`. The *value* of the manifest-version label is read from `container.ManifestVersion` at runtime — the docker sub-module never hard-codes `"1"`, so a future bump of the manifest schema version flows through automatically.

### Test image — `alpine:3.21`

Matches the `base` field in the README manifest example. Integration tests use a shared `ensureImage(t, "alpine:3.21")` helper that pulls if absent and calls `t.Skip` on pull failure — network unavailability is a valid skip reason, same disposition as daemon unavailability.

### Skip-when-unavailable detection

Obj 3 establishes the pattern since no sibling tau sub-module has integration tests. Pattern: shared `skipIfNoDaemon(t *testing.T) *client.Client` helper that constructs a client via `client.FromEnv`, pings with a 2s timeout, and calls `t.Skip` on failure. Per-test rather than `TestMain` so `go test -run <subset>` still skips cleanly.

### Example lives in the examples repo

The runnable example lands in `tailored-agentic-units/examples` rather than a `container/examples/` directory. The examples module is the cross-repo integration point that imports tagged releases of every tau library; placing the example there exercises the published contract that downstream consumers will actually experience. The corollary is that sub-issue D is blocked on the Phase 1 release tags — a different kind of dependency than the implementation-ordering dependencies between A/B/C.

### Manifest-missing semantics

When `CopyFrom` reports the manifest file is not found, `Inspect` returns a successful `ContainerInfo` with `Manifest == nil`. Callers that need a non-nil manifest substitute `container.Fallback()` themselves — this matches the `ContainerInfo.Manifest` godoc and keeps the Docker sub-module free of a decision that belongs to callers. Malformed or version-mismatched manifests surface as errors (`ErrManifestInvalid`, `ErrManifestVersion`) rather than silently falling back, so drift is caught at inspect time.
