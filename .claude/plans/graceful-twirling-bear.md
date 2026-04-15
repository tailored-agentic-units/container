# Plan — Issue #13: Docker `Inspect` and manifest integration

## Context

Sub-issue C of Objective #3. Replace the placeholder `Inspect` body in
`docker/docker.go` with a real implementation that maps Docker's
`ContainerJSON` to `container.ContainerInfo` and reads the image capability
manifest at `container.ManifestPath` via the existing `CopyFrom`. Closes the
container repo's contribution to Phase 1 and unblocks the v0.1.0 /
docker/v0.1.0 release session, which in turn unblocks the runnable
`docker-hello` example in the examples repo.

The issue body, the `Manifest-missing semantics` decision in
`_project/objective.md`, and the existing `CopyFrom` / `Parse` infrastructure
fully constrain the design. No architectural choices remain open.

## Files Modified (developer execution)

- `docker/docker.go` — replace the `Inspect` placeholder body; add a `mapState`
  private helper; add `"strings"` to the import block (for `TrimPrefix` on
  `Name`).

Test file `docker/tests/inspect_test.go` is added by AI during Phase 5
(Testing) and is intentionally not part of the implementation guide.

## Implementation

### 1. `Inspect` body in `docker/docker.go`

Call `r.cli.ContainerInspect(ctx, id)`, normalize `State.Status` via
`mapState`, populate `ContainerInfo` (stripping leading `/` from `Name`,
sourcing `Image` from `Config.Image`, taking `Labels` from `Config.Labels`),
then read the manifest by calling the receiver's own `CopyFrom`. Going
through `r.CopyFrom` (rather than the raw client method) keeps the manifest
convention runtime-method-mediated and reuses the tar-stripping that
`CopyFrom` already does.

Manifest disposition matches the issue's acceptance criteria and the
`Manifest-missing semantics` decision:

- File not found (`cerrdefs.IsNotFound` on the wrapped error from `CopyFrom`)
  → return `info` with `Manifest == nil` and `nil` error.
- Other `CopyFrom` errors → wrap with `"docker inspect: manifest read: %w"`.
- `container.Parse` errors → wrap with `"docker inspect: %w"` (Parse already
  tags with `ErrManifestInvalid` / `ErrManifestVersion`, so `errors.Is` chains
  through cleanly).

### 2. `mapState` helper in `docker/docker.go`

Switch on `State.Status`:

| Docker `Status` | `container.State` |
|-----------------|-------------------|
| `created` | `StateCreated` |
| `running`, `restarting` | `StateRunning` |
| `exited`, `dead` | `StateExited` |
| `removing` | `StateRemoved` |
| `paused` | error (Phase 1 excludes Paused — silently coercing would hide it) |
| anything else | error |

Nil `*State` returns an error too (defensive — Docker shouldn't, but cheap).

## Phase 5 Testing (AI, post-implementation)

Black-box `inspect_test.go` will cover: vanilla alpine (nil manifest), running
state mapping, well-formed manifest via `CopyTo`, malformed manifest
(`ErrManifestInvalid`), version-mismatched manifest (`ErrManifestVersion`),
ctx cancellation. Reuses `skipIfNoDaemon` / `ensureImage` /
`newRuntime` / `createSleeper` / `startSleeper` from existing test files.

## Reused Infrastructure

- `docker.go` — `CopyFrom` (the receiver method handles tar stripping and
  ctx-aware reads via `tarFileReader`).
- `manifest.go` — `Parse` (already tags errors with `ErrManifestInvalid` /
  `ErrManifestVersion`).
- `errors.go` — `ErrManifestInvalid`, `ErrManifestVersion`.
- `docker/tests/helpers_test.go`, `lifecycle_test.go`, `io_test.go` —
  reused by Phase 5 testing.

## Verification

```bash
cd /home/jaime/tau/container/docker
go build ./...
go vet ./...
go test ./tests/...
go test -coverpkg=github.com/tailored-agentic-units/container/docker ./tests/...
```

Coverage target ≥ 80% on the docker package per the issue's acceptance
criteria. Daemon required for the integration tests; `skipIfNoDaemon` ensures
graceful skips otherwise.

## Out of Scope

- `docker-hello` runnable example (sub-issue #14, blocked on release tags).
- Any changes to root-package types (`ContainerInfo`, `Manifest`, errors) —
  they already accommodate this work.
- A reusable `isNotFound` predicate — `cerrdefs.IsNotFound` is already in
  scope and works through wrapped error chains; promoting it to a private
  helper would add no value for one caller.
