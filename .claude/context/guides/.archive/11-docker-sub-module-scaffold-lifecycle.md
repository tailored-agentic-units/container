# 11 — Docker sub-module scaffold and lifecycle methods

## Problem Context

Objective #3 implements the `Runtime` interface against Docker. It decomposes into three sub-issues along I/O seams: #11 scaffold+lifecycle, #12 exec+copy, #13 inspect+manifest. This sub-issue lands the `container/docker` sub-module, wires the Docker Engine API client, and implements the four lifecycle methods (Create, Start, Stop, Remove). The remaining four `Runtime` methods are stubbed so the interface compiles and `Register()` can be called — their real implementations land in #12 and #13.

This sub-issue also establishes the integration-test-with-skip pattern (`skipIfNoDaemon`, `ensureImage`) that #12 and #13 reuse. No sibling tau sub-module has integration tests, so this is the pattern going forward.

## Architecture Approach

- Single root `go.mod` is untagged; `docker/go.mod` references the root via a Go pseudo-version targeting current `origin/main` (no `replace` directive, no dev pre-release tag).
- `Register()` is parameterless and wires a default factory using `client.FromEnv` + API version negotiation. Inline in `docker.go` (per phase.md cross-cutting decision).
- Label constants `LabelManaged` / `LabelManifestVersion` are exported from `docker.go`. The *value* of the manifest-version label is sourced from `container.ManifestVersion` at runtime — never hard-coded.
- Reserved labels (`tau.managed`, `tau.manifest.version`) always win over caller-supplied labels of the same key.
- `Stop` converts `time.Duration` → int seconds and passes `&seconds`. Docker's daemon handles kill-after-timeout escalation natively, so `Stop` honors its own timeout independent of `ctx` automatically. `timeout<=0` → pass `nil` ("use Docker default").
- `Remove(force=false)` pre-inspects container state. If running, returns a wrapped `ErrInvalidState` without calling the Docker API. If the daemon returns a conflict anyway (TOCTOU), remap via `cerrdefs.IsConflict` (containerd's errdefs; Docker's `errdefs.IsConflict` is deprecated in favor of this).
- Exec / CopyTo / CopyFrom / Inspect are stubs that return `fmt.Errorf("docker: <method> not implemented")`. Sub-issues #12 and #13 overwrite these bodies.
- Tests are black-box (`package docker_test`) in `docker/tests/`. `alpine:3.21` is the test image (matches the README manifest example). `sleep 30` keeps containers reliably "running" for force-false Remove assertions. Each test uses `t.Cleanup` to force-remove on exit.

## Implementation

### Step 1: Create `docker/go.mod`

New file `docker/go.mod`:

```
module github.com/tailored-agentic-units/container/docker

go 1.26

require (
	github.com/docker/docker v28.5.2+incompatible
	github.com/tailored-agentic-units/container v0.0.0-00010101000000-000000000000
)
```

The container version is a placeholder — Step 2 resolves it to a real pseudo-version.

### Step 2: Resolve the root-container pseudo-version

From `docker/`:

```bash
cd docker
go get github.com/tailored-agentic-units/container@main
go mod tidy
```

This pulls the commit hash of `origin/main`'s head into `docker/go.mod` as `v0.0.0-<timestamp>-<hash>` and populates `go.sum`. If `go get` errors because the commit isn't on origin, first run `git push origin main` from a parent session (do not push from this task — the task is still open). Fallback: if the commit hash simply isn't reachable, use an explicit pseudo-version syntax:

```bash
go get github.com/tailored-agentic-units/container@a02bd4b
```

`go mod tidy` will also pull the Docker SDK's transitive dependencies.

### Step 3: Create `docker/doc.go`

New file `docker/doc.go`:

```go
package docker
```

Leave the godoc comment empty here — Phase 7 (Documentation) fills in the package-level godoc with a usage stub. Keeping the file on its own means the package has a dedicated home for the package-level godoc without interleaving it into `docker.go`.

### Step 4: Create `docker/docker.go`

New file `docker/docker.go`:

```go
package docker

import (
	"context"
	"fmt"
	"io"
	"maps"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/client"
	dc "github.com/docker/docker/api/types/container"

	"github.com/tailored-agentic-units/container"
)

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

func (r *dockerRuntime) Create(ctx context.Context, opts container.CreateOptions) (*container.Container, error) {
	cfg := &dc.Config{
		Image:      opts.Image,
		Cmd:        opts.Cmd,
		Env:        buildEnv(opts.Env),
		WorkingDir: opts.WorkingDir,
		Labels:     mergeLabels(opts.Labels),
	}

	resp, err := r.cli.ContainerCreate(
		ctx, cfg,
		nil, nil, nil,
		opts.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("docker create: %w", err)
	}

	return &container.Container{
		ID:     resp.ID,
		Name:   opts.Name,
		Image:  opts.Image,
		State:  container.StateCreated,
		Labels: cfg.Labels,
	}, nil
}

func (r *dockerRuntime) Start(ctx context.Context, id string) error {
	if err := r.cli.ContainerStart(ctx, id, dc.StartOptions{}); err != nil {
		return fmt.Errorf("docker start: %w", err)
	}
	return nil
}

func (r *dockerRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	var secs *int
	if timeout > 0 {
		s := int(timeout.Seconds())
		secs = &s
	}
	if err := r.cli.ContainerStop(ctx, id, dc.StopOptions{Timeout: secs}); err != nil {
		return fmt.Errorf("docker stop: %w", err)
	}
	return nil
}

func (r *dockerRuntime) Remove(ctx context.Context, id string, force bool) error {
	if !force {
		info, err := r.cli.ContainerInspect(ctx, id)
		if err != nil {
			return fmt.Errorf("docker remove: inspect: %w", err)
		}
		if info.State != nil && info.State.Running {
			return fmt.Errorf("docker remove: %w: container %s is running", container.ErrInvalidState, id)
		}
	}
	if err := r.cli.ContainerRemove(ctx, id, dc.RemoveOptions{Force: force}); err != nil {
		if cerrdefs.IsConflict(err) {
			return fmt.Errorf("docker remove: %w: %v", container.ErrInvalidState, err)
		}
		return fmt.Errorf("docker remove: %w", err)
	}
	return nil
}

func (r *dockerRuntime) Exec(ctx context.Context, id string, opts container.ExecOptions) (*container.ExecResult, error) {
	return nil, fmt.Errorf("docker exec: not implemented in sub-issue #11")
}

func (r *dockerRuntime) CopyTo(ctx context.Context, id string, dst string, content io.Reader) error {
	return fmt.Errorf("docker copy_to: not implemented in sub-issue #11")
}

func (r *dockerRuntime) CopyFrom(ctx context.Context, id string, src string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("docker copy_from: not implemented in sub-issue #11")
}

func (r *dockerRuntime) Inspect(ctx context.Context, id string) (*container.ContainerInfo, error) {
	return nil, fmt.Errorf("docker inspect: not implemented in sub-issue #11")
}

func buildEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func mergeLabels(caller map[string]string) map[string]string {
	out := maps.Clone(caller)
	if out == nil {
		out = make(map[string]string, 2)
	}
	out[LabelManaged] = "true"
	out[LabelManifestVersion] = container.ManifestVersion
	return out
}
```

Godoc comments on `Register`, the constants, and the four lifecycle methods are deferred to Phase 7 (Documentation). Comments on the stub methods are not needed — they'll be replaced wholesale in #12 and #13.

## Validation Criteria

- [ ] `docker/go.mod` declares `github.com/tailored-agentic-units/container/docker` and requires both root `container` (pseudo-version) and `github.com/docker/docker` v28.x
- [ ] `docker/doc.go` exists (godoc content landed in Phase 7 by the AI)
- [ ] `docker/docker.go` exports `Register`, `LabelManaged`, `LabelManifestVersion`; unexported `dockerRuntime` implements all 8 `Runtime` methods (4 real, 4 stub)
- [ ] `Create` applies `tau.managed=true` and `tau.manifest.version=<container.ManifestVersion>`; caller labels merged but cannot override reserved pair
- [ ] `Stop` honors its own timeout independently of `ctx` (validated by Docker's daemon behavior; code converts `timeout` to `int` seconds)
- [ ] `Remove(force=false)` on a running container returns an error detectable via `errors.Is(err, container.ErrInvalidState)`
- [ ] Integration tests skip gracefully when daemon is unreachable OR image pull fails
- [ ] `tests/lifecycle_test.go` covers: create → state-check → start → state-check → stop → remove round-trip; label presence (reserved + caller merged); reserved labels cannot be overridden; force-vs-non-force remove semantics
- [ ] `go build ./...`, `go vet ./...`, `go test ./tests/...` pass from repo root
- [ ] Same three commands pass from within `docker/`
- [ ] `go mod tidy` in `docker/` produces a clean `go.mod` / `go.sum` diff (no spurious deps)
