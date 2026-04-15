# Plan — Issue #11: Docker Sub-Module Scaffold and Lifecycle Methods

## Context

Objective #3 decomposes into three sub-issues (A=#11 scaffold+lifecycle, B=#12 exec+copy, C=#13 inspect+manifest). Sub-issue #11 establishes the `container/docker` sub-module, wires the Docker Engine API client, lands the four lifecycle methods (Create/Start/Stop/Remove), and sets the integration-test-with-skip pattern that B and C extend. The four remaining Runtime methods (Exec, CopyTo, CopyFrom, Inspect) are out of scope for this sub-issue — implementations must stub them to satisfy the interface but may return `errors.ErrUnsupported` with an operation-context wrap.

The design decisions are already nailed down in the issue body and `_project/objective.md` (Architecture decisions section): explicit parameterless `Register()`, label constants sourced from root, `skipIfNoDaemon` + `ensureImage` test helpers with graceful skip, `alpine:3.21` test image, error wrapping via `fmt.Errorf(..., %w)`. The work is therefore translation, not design exploration.

## Module dependency approach

No root `v0.1.0` tag exists yet (per feedback memory, dev pre-release tagging is deferred until the library is runnable — which won't be until sub-issue #13 completes the manifest-integrated Inspect). CLAUDE.md forbids `replace` directives. The Go-idiomatic resolution: **use a pseudo-version in `docker/go.mod` targeting the current `origin/main` commit** (`a02bd4b`). The developer runs `go get github.com/tailored-agentic-units/container@main` inside `docker/` and Go generates `v0.0.0-<timestamp>-<short-hash>` automatically. No manual version string authoring required.

This keeps the "no replace" rule intact, defers the first real tag to post-#13, and is the standard Go approach for untagged intra-repo multi-module development.

## Critical files

**New files:**
- `docker/go.mod` — module declaration, dependencies pinned to Docker v28.x
- `docker/go.sum` — auto-generated
- `docker/doc.go` — package godoc with `Register()` + `container.Create("docker")` usage snippet
- `docker/docker.go` — `Register()`, label constants, `dockerRuntime` struct, four lifecycle methods, stubs for Exec/CopyTo/CopyFrom/Inspect
- `docker/tests/helpers_test.go` — `skipIfNoDaemon`, `ensureImage`
- `docker/tests/lifecycle_test.go` — black-box integration tests

**Modified files:**
- `.claude/CLAUDE.md` — remove `examples/` from Project Structure block (moved to examples repo)
- `CHANGELOG.md` — add `v0.1.0-dev.3.11` section during Phase 8c (note: do NOT cut a tag; memory defers tagging)

## Docker SDK primitives

Pin `github.com/docker/docker` at `v28.x` (latest stable). Import aliases:

```go
import (
    "github.com/docker/docker/client"
    dc "github.com/docker/docker/api/types/container"
    di "github.com/docker/docker/api/types/image"
    "github.com/docker/docker/errdefs"
)
```

Alias `dc` for Docker's `container` package avoids the collision with our `container` root package.

Lifecycle surface:
- `cli.ContainerCreate(ctx, *dc.Config, *dc.HostConfig, nil, nil, name)` → `(dc.CreateResponse, error)`
- `cli.ContainerStart(ctx, id, dc.StartOptions{})`
- `cli.ContainerStop(ctx, id, dc.StopOptions{Timeout: &seconds})` where `seconds := int(timeout.Seconds())`
- `cli.ContainerRemove(ctx, id, dc.RemoveOptions{Force: force})`
- `cli.ContainerInspect(ctx, id)` → `(dc.InspectResponse, error)` (used by Remove to check state before the API call, and by the stubbed Inspect)
- `cli.Ping(ctx)` → `(types.Ping, error)` (used by `skipIfNoDaemon`)
- `cli.ImagePull(ctx, ref, di.PullOptions{})` → `(io.ReadCloser, error)` (used by `ensureImage`)

`errdefs.IsConflict(err)` detects Docker's "cannot remove a running container" response, which we remap to `container.ErrInvalidState`.

## Implementation shape

### `docker/docker.go`

```go
package docker

const (
    LabelManaged         = "tau.managed"
    LabelManifestVersion = "tau.manifest.version"
)

func Register() {
    container.Register("docker", func() (container.Runtime, error) {
        cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
        if err != nil {
            return nil, fmt.Errorf("docker: new client: %w", err)
        }
        return &dockerRuntime{cli: cli}, nil
    })
}

type dockerRuntime struct {
    cli *client.Client
}
```

**Create.** Build `dc.Config` from `CreateOptions` (Image, Cmd, Env as `KEY=VALUE` slice, WorkingDir). Merge `opts.Labels` with reserved labels: reserved labels win on collision. Env map iteration order is nondeterministic — stable sort for test determinism is not required but clean. Call `ContainerCreate` with nil HostConfig/NetworkingConfig/Platform. Return `&Container{ID: resp.ID, Name: opts.Name, Image: opts.Image, State: StateCreated, Labels: mergedLabels}`.

**Start.** `ContainerStart(ctx, id, dc.StartOptions{})`, wrap error.

**Stop.** Convert `timeout` to int seconds: `s := int(timeout.Seconds())`. Pass as `&s`. Docker handles the kill-after-timeout escalation natively — `timeout` is honored independently of `ctx` by the daemon. Wrap error.

**Remove.** When `force=false`, Inspect first to check if the container is running. If `State.Running` is true, return `fmt.Errorf("docker remove: %w: container %s is running", container.ErrInvalidState, id)`. Then call `ContainerRemove(ctx, id, dc.RemoveOptions{Force: force})`. If the daemon races us and returns a conflict anyway, still remap via `errdefs.IsConflict(err)` → `ErrInvalidState`. Wrap other errors. (Pre-check + remap pattern handles both TOCTOU and the daemon-only error path.)

**Exec / CopyTo / CopyFrom / Inspect stubs.** Return `fmt.Errorf("docker: <method> not implemented in sub-issue #11")`. No sentinel — sub-issues #12 and #13 will overwrite these bodies. The stubs exist only to satisfy the `Runtime` interface so `docker.Register()` compiles.

### `docker/tests/helpers_test.go`

```go
package docker_test

func skipIfNoDaemon(t *testing.T) *client.Client {
    t.Helper()
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil { t.Skipf("docker client init failed: %v", err) }
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    if _, err := cli.Ping(ctx); err != nil { t.Skipf("docker daemon unreachable: %v", err) }
    return cli
}

func ensureImage(t *testing.T, cli *client.Client, ref string) {
    t.Helper()
    _, err := cli.ImageInspect(context.Background(), ref)
    if err == nil { return }
    rc, pullErr := cli.ImagePull(context.Background(), ref, di.PullOptions{})
    if pullErr != nil { t.Skipf("image pull failed: %v", pullErr) }
    defer rc.Close()
    _, _ = io.Copy(io.Discard, rc)  // must drain to complete the pull
}
```

### `docker/tests/lifecycle_test.go`

Black-box `package docker_test`. Test matrix (all gated on `skipIfNoDaemon` + `ensureImage`):

1. `TestLifecycle_RoundTrip` — Create → Inspect-state (via the stubbed Inspect — skip this assertion; defer to sub-issue #13) → Start → Stop → Remove. Verify container removed (inspect returns not-found). Since `Inspect` is stubbed in #11, the "Inspect-state" step is replaced with a Docker-SDK-direct `cli.ContainerInspect` to assert state transitions; this is an exception scoped to #11 to avoid blocking on #13. A TODO comment flags that #13 will replace this with `runtime.Inspect`.
2. `TestLifecycle_LabelsApplied` — Create with caller labels `{"my.label": "x"}`. After Create, fetch via Docker SDK `cli.ContainerInspect` to read `.Config.Labels`. Assert `tau.managed=true`, `tau.manifest.version=<ManifestVersion>`, `my.label=x`.
3. `TestLifecycle_ReservedLabelsCannotBeOverridden` — Create with `Labels: {LabelManaged: "false"}`. Assert the container's `tau.managed` label is `"true"` (reserved wins).
4. `TestRemove_ForceFalse_RunningContainer` — Create, Start, `Remove(force=false)`. Assert `errors.Is(err, container.ErrInvalidState)`.
5. `TestRemove_ForceTrue_RunningContainer` — Create, Start, `Remove(force=true)` succeeds; container is gone.

The `alpine:3.21` test image runs `sleep 30` so the container is reliably "running" for force-false tests. Each test uses `t.Cleanup` with `Remove(force=true)` to guarantee teardown even on failure.

## Test scope

We test all reasonably testable public infrastructure — we don't chase a coverage percentage. In scope for #11: `Register`, the exported label constants, and the four lifecycle methods (Create/Start/Stop/Remove) including their documented error paths (e.g., `Remove(force=false)` on a running container). Stubbed methods (Exec/CopyTo/CopyFrom/Inspect) are not tested here — their real implementations and tests land in sub-issues #12 and #13. The five test cases enumerated above cover the lifecycle surface end-to-end; no synthetic edge-case tests beyond what the documented behavior requires.

## Validation

From the repo root:
```bash
go build ./...
go vet ./...
go test ./tests/...
```

From `docker/`:
```bash
go build ./...
go vet ./...
go mod tidy
go test ./tests/...
```

Manual sanity check (optional): write a throwaway `main.go` that calls `docker.Register()` then `container.Create("docker")`, creates an alpine container running `echo hello`, starts, then removes. Delete before commit. (This is outside the tests — the stub `Exec` prevents an end-to-end run anyway; full end-to-end validation arrives at sub-issue #13.)

## Documentation review (pre-closeout)

- `.claude/CLAUDE.md` — Project Structure references `docker/` correctly but CLAUDE.md's Project Structure block doesn't list `examples/` at all (just checked). The issue's scope item "Remove `examples/` from CLAUDE.md's Project Structure block" may be stale — will verify and no-op if already absent.
- `_project/README.md` — Package Structure block shows `docker/register.go` as a separate file; our implementation inlines `Register()` in `docker.go` per the phase.md cross-cutting decision ("`Register()` location: inline in `docker/docker.go`"). README is documentation-only and the phase.md note already flags this as a future README update, so it's fine to leave untouched for #11.
- `_project/phase.md`, `_project/objective.md` — no status changes yet; #11 moves from Todo → In Progress on the project board, but the markdown files roll that up separately.

## Open questions / risks

- **ImageInspect return shape.** Docker SDK v28 changed `ImageInspect` return type in some versions; if `cli.ImageInspect` doesn't compile as a single call, fall back to `cli.ImageList` with a name filter. Minor, resolved at implementation time.
- **Name collision in tests.** Tests that use `cli.ContainerInspect` directly (via the Docker SDK) import `dc "github.com/docker/docker/api/types/container"`. No collision with our root package since the tests are `package docker_test` and import `container` (root) for the domain types.
- **Timeout=0 semantics for Stop.** If a caller passes `timeout=0`, `int(0s.Seconds())=0`, and Docker interprets `&0` as "kill immediately." Documented behavior, acceptable. `timeout<0` should be treated as "no explicit timeout, use Docker default" — pass `nil` in that case (guard: `if timeout > 0 { s := int(...); &s } else { nil }`).
