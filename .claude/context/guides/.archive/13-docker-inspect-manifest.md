# 13 - Docker runtime Inspect and manifest integration

## Problem Context

Sub-issue C closes the container repo's contribution to Objective #3. The
Docker runtime currently has a placeholder `Inspect` body that returns a
"not implemented in sub-issue #11" sentinel error. This task replaces that
placeholder with the real implementation: map Docker's `ContainerJSON` fields
to `container.ContainerInfo`, then read the image capability manifest at
`container.ManifestPath` via the runtime's own `CopyFrom` and feed the bytes
to `container.Parse`. With this in place, the library at `main` is
feature-complete for Phase 1 and ready for the v0.1.0 / docker/v0.1.0
release.

## Architecture Approach

**State mapping.** A small private helper `mapState` normalizes Docker's
`Status` string into the four-value `container.State` set defined in
`container.go`. The mapping table:

| Docker `Status` | `container.State` |
|-----------------|-------------------|
| `created` | `StateCreated` |
| `running`, `restarting` | `StateRunning` |
| `exited`, `dead` | `StateExited` |
| `removing` | `StateRemoved` |
| `paused` | error |
| anything else | error |

`paused` is rejected explicitly because Phase 1 intentionally excludes Paused
from the state set — silently coercing it would hide a state the runtime
cannot represent. A nil `*State` also returns an error (defensive).

**Manifest read.** Call `r.CopyFrom(ctx, id, container.ManifestPath)` on the
receiver — going through the runtime's own method (rather than reaching into
`r.cli.CopyFromContainer` directly) keeps the manifest convention
runtime-method-mediated and reuses the tar-stripping that `CopyFrom` already
performs. `CopyFrom` already wraps its errors with
`fmt.Errorf("docker copy_from: %w", err)`, so `cerrdefs.IsNotFound(err)`
detects a missing manifest through the wrapped chain.

**Manifest absent vs. error.** Per the `Manifest-missing semantics` decision
in `_project/objective.md`:

- File not found → return `info` with `Manifest == nil` and `nil` error.
  Callers that need a non-nil manifest substitute `container.Fallback()`
  themselves.
- Other `CopyFrom` errors → wrap with `"docker inspect: manifest read: %w"`.
- `container.Parse` errors → wrap with `"docker inspect: %w"`. `Parse`
  already tags with `ErrManifestInvalid` / `ErrManifestVersion`, so
  `errors.Is` chains through cleanly.

**Field sourcing from `ContainerJSON`.**

- `Name` — strip the leading `/` Docker adds (e.g., `/serene_thompson` →
  `serene_thompson`) so the value matches what callers passed to
  `CreateOptions.Name` and what `Container.Name` already reports.
- `Image` — use `Config.Image` (the originally requested reference like
  `alpine:3.21`), not the top-level `Image` field (which is the resolved
  image ID hash).
- `Labels` — take from `Config.Labels`.

**No `isNotFound` helper.** `cerrdefs.IsNotFound` from
`github.com/containerd/errdefs` is already imported (the `Remove` path uses
it) and works through the wrapped error chain. Adding a one-line wrapper for
a single caller would add no value.

## Implementation

### Step 1: Add `"strings"` to the import block in `docker/docker.go`

The existing import block has standard-library imports grouped first. Add
`"strings"` in alphabetical order (between `"path"` and `"time"`):

```go
import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/tailored-agentic-units/container"

	cerrdefs "github.com/containerd/errdefs"
	dc "github.com/docker/docker/api/types/container"
)
```

No other import changes are needed — `cerrdefs` and `dc` are already
aliased.

### Step 2: Replace the `Inspect` placeholder body in `docker/docker.go`

Find the existing two-line placeholder:

```go
func (r *dockerRuntime) Inspect(ctx context.Context, id string) (*container.ContainerInfo, error) {
	return nil, fmt.Errorf("docker inspect: not implemented in sub-issue #11")
}
```

Replace its body with the real implementation:

```go
func (r *dockerRuntime) Inspect(ctx context.Context, id string) (*container.ContainerInfo, error) {
	raw, err := r.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}

	state, err := mapState(raw.State)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}

	info := &container.ContainerInfo{
		ID:     raw.ID,
		Name:   strings.TrimPrefix(raw.Name, "/"),
		Image:  raw.Config.Image,
		State:  state,
		Labels: raw.Config.Labels,
	}

	rc, err := r.CopyFrom(ctx, id, container.ManifestPath)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return info, nil
		}
		return nil, fmt.Errorf("docker inspect: manifest read: %w", err)
	}
	defer rc.Close()

	manifest, err := container.Parse(rc)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}
	info.Manifest = manifest

	return info, nil
}
```

### Step 3: Add the `mapState` helper in `docker/docker.go`

Place the helper at the bottom of the file, alongside `buildEnv` and
`mergeLabels`:

```go
func mapState(s *dc.State) (container.State, error) {
	if s == nil {
		return "", fmt.Errorf("nil container state")
	}
	switch s.Status {
	case "created":
		return container.StateCreated, nil
	case "running", "restarting":
		return container.StateRunning, nil
	case "exited", "dead":
		return container.StateExited, nil
	case "removing":
		return container.StateRemoved, nil
	case "paused":
		return "", fmt.Errorf("paused state not supported in Phase 1")
	default:
		return "", fmt.Errorf("unknown docker state %q", s.Status)
	}
}
```

## Validation Criteria

- [ ] `Inspect` populates `ID`, `Name` (without leading `/`), `Image`,
      `State`, and `Labels` from `ContainerJSON`
- [ ] `mapState` covers `created` / `running` / `restarting` / `exited` /
      `dead` / `removing`; `paused` and unknown statuses return an error
- [ ] Missing manifest produces `Manifest == nil` and `nil` error
- [ ] Malformed manifest produces an error where
      `errors.Is(err, container.ErrManifestInvalid)` is true
- [ ] Version-mismatched manifest produces an error where
      `errors.Is(err, container.ErrManifestVersion)` is true
- [ ] Cancelling `ctx` aborts the inspect cleanly
- [ ] `cd docker && go build ./...` passes
- [ ] `cd docker && go vet ./...` passes
