# Objective 3 Planning — Docker Runtime Implementation

## Context

Phase 1's final objective implements the `Runtime` interface (`runtime.go`) against the Docker Engine API as the `container/docker` sub-module. This validates the interface design end-to-end and delivers the first functional execution environment.

Objectives 1 and 2 are complete:
- Obj 1 landed `Runtime` interface, core types (`Container`, `CreateOptions`, `ExecOptions`, `ContainerInfo`), state model, registry, and domain errors.
- Obj 2 landed the manifest types (`Manifest`, `Tool`, `Service`), `Parse`, `Validate`, `Fallback`, `ManifestVersion`/`ManifestPath` constants, and `ContainerInfo.Manifest`.

Obj 3 depends on both — it wires them together behind a concrete runtime and proves the interface is implementable without reshaping.

Transition closeout for Obj 2 is done: issue #2 is closed (all sub-issues complete), `_project/objective.md` is deleted.

## Scope (from issue #3)

**In scope:**
- `container/docker` sub-module with own `go.mod` requiring `protocol` and root `container`
- Docker Engine API client wiring
- All 8 `Runtime` methods (Create, Start, Stop, Remove, Exec, CopyTo, CopyFrom, Inspect)
- `tau.managed=true` and `tau.manifest.version=1` labels applied at `Create`
- `Inspect` reads `/etc/tau/manifest.json` via `CopyFrom`, parses via `container.Parse`, populates `ContainerInfo.Manifest`
- Explicit `Register()` function — parameterless, no `init()`
- Integration tests in `docker/tests/` that skip gracefully when Docker is unreachable
- End-to-end runnable example landing in `tailored-agentic-units/examples` as `cmd/docker-hello/` — scoped as the final sub-issue, blocked on the Phase 1 release (v0.1.0 + docker/v0.1.0) because the examples module imports tagged releases, not local workspace refs

**Out of scope:** containerd/podman runtimes, volume mounts beyond CopyTo/CopyFrom, resource limits, health checks, networking (Phase 3).

**Cross-repo:** Sub-issues A/B/C land in `tailored-agentic-units/container`; sub-issue D lands in `tailored-agentic-units/examples`.

## Conventions to mirror (from sibling sub-modules)

From `/home/jaime/tau/provider/{azure,bedrock,ollama}/` survey:

- `Register()` lives inline in the primary implementation file (e.g. `docker.go`), not a separate `register.go` — matches the CLAUDE.md cross-cutting decision.
- `doc.go` carries the package-level godoc and a usage stub showing `Register()` + `container.Create("docker")`.
- Tests live in `tests/` at sub-module level, black-box (`package docker_test`).
- Factory signature is `func() (Runtime, error)` (parameterless, per `registry.go`). Runtime-specific configuration is closed over at `Register()` time, not passed to the factory — sub-module can expose `RegisterWithOptions(...)` helpers later if needed. Phase 1 registers a single default factory.

Skip-when-unavailable pattern does not exist elsewhere in tau. Obj 3 establishes it: attempt `client.Ping(ctx)` in each test file's `TestMain` (or per-test helper); on error → `t.Skip("docker daemon unreachable")`.

## Decomposition (3 sub-issues, strictly ordered)

Each sub-issue maps to one branch, one PR, and ships independently testable behavior. The split runs along natural seams in the Runtime surface: lifecycle / I/O / inspect-manifest-integration.

### Sub-issue A — Sub-module scaffold + lifecycle methods
**Depends on:** nothing (unblocked once Obj 2 is merged — done).

Establishes the module:
- `docker/go.mod` requiring `tau/protocol` (transitively via root) and root `tau/container`, plus `github.com/docker/docker` client
- `docker/doc.go` with package godoc
- `docker/docker.go` holding the Docker client wrapper, unexported `dockerRuntime` struct implementing `Runtime`, and exported `Register()` that wires a default factory into `container.Register("docker", ...)`
- Implements **Create, Start, Stop, Remove** with documented context/cancel semantics
- Applies `tau.managed=true` and `tau.manifest.version=<ManifestVersion>` labels in `Create`
- `Stop` escalates to kill after `timeout`, independent of `ctx` (per Phase 1 cross-cutting decision)
- `Remove` with `force=false` and running container returns `%w ErrInvalidState`

**Tests (`docker/tests/lifecycle_test.go`):** pull `alpine:3.21`, exercise create/start/stop/remove round-trip, label presence assertion, state transitions, force-vs-non-force remove semantics. Skip gracefully via shared `skipIfNoDaemon` helper.

Establishes the test-skip pattern subsequent PRs build on.

### Sub-issue B — Exec + Copy methods
**Depends on:** sub-issue A (uses the client wrapper and lifecycle for test fixtures).

Implements the three I/O methods:
- `Exec` — creates an exec instance, attaches optional stdin/stdout/stderr per `ExecOptions`, returns `ExecResult`; cancelling `ctx` kills the exec and wraps `ctx.Err()`
- `CopyTo` — tar-stream upload via `ContainerArchive.PutArchive`; creates parent dirs as needed
- `CopyFrom` — returns a `ReadCloser` the caller MUST close; cancelling `ctx` aborts the stream

**Tests (`docker/tests/io_test.go`):** exec with stdout/stderr capture, exec with non-zero exit, exec with ctx cancel, copy-to followed by copy-from round-trip (content integrity check), copy-from absent file surfaces a usable error.

### Sub-issue C — Inspect + manifest integration
**Depends on:** sub-issues A and B (uses lifecycle for fixtures, CopyFrom to read manifest).

Implements `Inspect` and integrates the manifest:
- `Inspect` maps Docker `ContainerJSON` fields to `ContainerInfo` (ID, Name, Image, State, Labels)
- Reads `ManifestPath` via `CopyFrom`, feeds bytes to `container.Parse`, assigns to `ContainerInfo.Manifest`
- Missing manifest (`CopyFrom` reports "no such file") → `ContainerInfo.Manifest = nil` (per `ContainerInfo.Manifest` godoc — callers substitute `Fallback()` if they need a non-nil value)
- Malformed manifest → error wraps `ErrManifestInvalid` via `Parse`
- Version mismatch → error wraps `ErrManifestVersion` via `Parse`

**Tests (`docker/tests/inspect_test.go`):**
- Inspect on vanilla alpine → `Manifest == nil`
- Inspect on a container built with a manifest injected via `CopyTo` before start → `Manifest` non-nil and well-formed
- Inspect on a container with a malformed manifest → error surface check via `errors.Is(err, ErrManifestInvalid)`
- Cancellation during inspect aborts cleanly

When merged, the container repo's contribution to Obj 3 is complete. Obj 3 itself is not closed until sub-issue D also merges.

### Sub-issue D — `docker-hello` runnable example
**Repository:** `tailored-agentic-units/examples`.
**Depends on:** sub-issues A, B, C merged AND `container/v0.1.0` and `container/docker/v0.1.0` tagged. The examples module consumes tagged releases, not local workspace refs, so this sub-issue is **blocked** until Phase 1 release completes.

Adds `cmd/docker-hello/main.go` in the examples repo:
- `go get github.com/tailored-agentic-units/container@v0.1.0` and the docker sub-module at its tag
- Calls `docker.Register()`, `container.Create("docker")`, pulls `alpine:3.21`, creates a labeled container, starts, execs `echo hello`, inspects (demonstrates nil-manifest case — alpine has no tau manifest), stops, removes
- A short `cmd/docker-hello/README.md` pointing back to the container repo and describing prerequisites (running Docker daemon)
- `examples/README.md` sub-READMEs list extended with the new entry

Smoke-level demonstration; no assertions. Proves end-to-end that a third-party module can consume the published tau container runtime as intended.

## Architecture decisions

### Client injection vs. default factory
Phase 1 registers a single default factory that constructs a Docker client from ambient environment (`client.FromEnv`). Exposing a `RegisterWithClient(client *client.Client)` helper is deferred until a caller needs it — YAGNI. The unexported `dockerRuntime` struct takes a `*client.Client` in its constructor so test helpers can inject a pre-configured client without going through `Register`.

### Label constants
`tau.managed` and `tau.manifest.version` are exported as package constants in `docker.go` (e.g. `LabelManaged`, `LabelManifestVersion`). Callers that filter containers by label (future work per CLAUDE.md "Gotchas") use these rather than string literals. The *value* `tau.manifest.version=<version>` is set from root `container.ManifestVersion` so the docker sub-module never hard-codes "1".

### Test image
All integration tests use `alpine:3.21` — small, cached after first pull, matches the README manifest example's `base`. Test helper `ensureImage(t, "alpine:3.21")` pulls if absent and calls `t.Skip` on pull failure (network unavailable is a valid skip reason too).

### Skip-when-unavailable detection
`docker/tests/helpers_test.go` exports `skipIfNoDaemon(t *testing.T) *client.Client` that:
1. Constructs a Docker client via `client.FromEnv`
2. Calls `cli.Ping(ctx)` with a short timeout (2s)
3. On error, calls `t.Skip("docker daemon unreachable: %v", err)` and returns nil
4. On success returns the client for the test to use

This avoids `TestMain` — per-test skip is friendlier when running a subset via `go test -run`.

### Error wrapping
Every Docker API call error is wrapped with operation context: `fmt.Errorf("docker create: %w", err)` (per CLAUDE.md Go Conventions). Domain errors surface unwrapped via `errors.Is`.

### Example placement
Lives in `tailored-agentic-units/examples` at `cmd/docker-hello/`, not inside the container repo. Reason: the examples module is the cross-repo integration point that imports tagged releases of every tau library; placing the runnable example there exercises the published contract (what downstream consumers will actually experience), not a workspace-local import. This is why sub-issue D is blocked on the Phase 1 release tags.

No `examples/` directory is created inside the `container` repo. CLAUDE.md's "Project Structure" block (which currently lists `examples/`) is updated during sub-issue A's task closeout to remove that directory — Obj 3 won't create it.

## Sub-issues table

| # | Repo | Title | Depends on | Branch (suggested) |
|---|------|-------|-----------|---------------------|
| A | container | Docker sub-module scaffold and lifecycle methods | — | `3-docker-lifecycle` |
| B | container | Docker runtime Exec and file copy methods | A | `3-docker-io` |
| C | container | Docker runtime Inspect and manifest integration | A, B | `3-docker-inspect` |
| D | examples | `docker-hello` runnable example | A, B, C merged + v0.1.0 tagged | `docker-hello` |

A/B/C: label `feature`, issue type `Task`, milestone `Phase 1 - Runtime Foundation` on the container repo, project `TAU Container`, Phase field `Phase 1 - Runtime Foundation`.

D: label `feature`, issue type `Task`, on `tailored-agentic-units/examples`. Issue body explicitly documents the release-tag blocker; it stays open throughout Obj 3 execution and only becomes actionable after the Phase 1 release session tags v0.1.0 and docker/v0.1.0. Add to the `TAU Container` project board with Phase = `Phase 1 - Runtime Foundation` so Obj 3's roll-up reflects the pending work.

## Verification

Plan is executable end-to-end when:
1. Issue #3 has 4 linked sub-issues (3 on container, 1 on examples) visible in GitHub's sub-issue UI.
2. Each sub-issue body contains Context / Scope / Approach / Acceptance Criteria sections sufficient to bootstrap a task execution session without re-reading this plan. Sub-issue D's body explicitly states the v0.1.0 / docker/v0.1.0 tag prerequisite.
3. Each sub-issue is on the `TAU Container` project board with Phase = "Phase 1 - Runtime Foundation" and Status = "Todo".
4. `_project/objective.md` exists with the sub-issues table (including D) and architecture decisions above.
5. `_project/phase.md` objectives table shows Obj 2 = Done, Obj 3 = In Progress.
6. Obj 3 issue body updated to list both repositories (container + examples) and the example sub-issue.

## Files to modify during execution

- **Create** `_project/objective.md` — scope, acceptance criteria, sub-issues table (A/B/C/D), architecture decisions
- **Edit** `_project/phase.md` — flip Obj 3 status from "Planned" to "In Progress"
- **Edit** Obj 3 issue body (`gh issue edit 3`) — update `Repositories` to list both `tailored-agentic-units/container` and `tailored-agentic-units/examples`; move the runnable example to its own line noting it lives in the examples repo behind the release-tag blocker
- **Create** 3 sub-issues on `tailored-agentic-units/container` (A, B, C) via `gh issue create`
- **Create** 1 sub-issue on `tailored-agentic-units/examples` (D) via `gh issue create` — body documents the v0.1.0 / docker/v0.1.0 tag blocker
- **Link** all 4 sub-issues to parent issue #3 via `addSubIssue` GraphQL mutation
- **Assign** `Task` issue type via `updateIssueIssueType` GraphQL mutation. Container repo type ID `IT_kwDOD155C84B2CKc`; examples repo type ID must be fetched (issue types are per-repo)
- **Add** each sub-issue to project `PVT_kwDOD155C84BUESJ` with Phase option `1be1afec` ("Phase 1 - Runtime Foundation")
- **Fetch** at execution time: examples repo's `Task` issue type ID, `feature` label presence on examples repo (create if absent), and whether the examples repo uses the same Phase 1 milestone convention

## Resolved design choices

- **Decomposition:** 3 PRs (A lifecycle, B I/O, C inspect+example).
- **Factory surface (Sub-issue A):** default factory only (`client.FromEnv`). Tests use the unexported constructor directly. `RegisterWithClient` is deferred until a caller needs it.
- **Test image:** `alpine:3.21`, pulled on-demand via `ensureImage(t, ...)`; pull failure → `t.Skip` (network absence is a valid skip reason, same disposition as daemon absence).
