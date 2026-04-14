# 9 — Implement Image Capability Manifest Types and Validation

## Problem Context

Container images need a portable way to declare their capabilities — shell, workspace, env, tools, services — so tau agents can understand their environment before doing work. The convention is a JSON file at `/etc/tau/manifest.json` inside the image, read at inspect time by the runtime and surfaced through `ContainerInfo.Manifest`.

Objective 1 landed the `Runtime` interface and `ContainerInfo` type with a TODO placeholder for the manifest pointer (`container.go:99-102`). Objective 3 (Docker `Inspect`) will read the file via `Runtime.CopyFrom("/etc/tau/manifest.json")` and populate the pointer. This issue sits between them: land the `Manifest` type, parsing, validation, fallback, error sentinels, and the `ContainerInfo.Manifest` field, with black-box tests. No Docker work here; no Go dependency changes.

## Architecture Approach

**Strict decoding.** `Parse` calls `json.Decoder.DisallowUnknownFields()`. Any top-level field outside the declared schema is a decode error wrapping `ErrManifestInvalid`. Rationale: the manifest is a tau-owned contract, `ManifestVersion` is the negotiation knob, and strict-first preserves more optionality than permissive-first (relaxing strict later is non-breaking; tightening permissive later is breaking).

**Controlled pass-through via `Options`.** The one sanctioned escape hatch for container-runtime-specific config that tau itself doesn't interpret is an `Options map[string]any` field at the top of `Manifest`. Mirrors the established TAU pattern at `tau/protocol/config/provider.go:11` and `tau/format/data.go:9`. Kept at `Manifest` only — `Tool` and `Service` retain their tight `version`/`description` schemas.

**Sentinel errors.** Two new sentinels in `errors.go`:
- `ErrManifestInvalid` — decode failure, unknown field, or missing required field
- `ErrManifestVersion` — version mismatch (distinguished so callers can tell "wrong version" from "malformed JSON" via `errors.Is`)

**Validation surface.** `Validate` enforces the minimum contract: `Version == ManifestVersion`, non-empty `Name`, non-empty `Shell`. Optional fields, `Options` contents, and map entries are black boxes at this layer.

**Fallback.** `Fallback()` returns a non-nil manifest that passes `Validate` — used by Objective 3's `Inspect` when `/etc/tau/manifest.json` is absent from the image.

## Implementation

### Step 1: Add manifest error sentinels to `errors.go`

Append two new sentinels to the existing `var (...)` block in `/home/jaime/tau/container/errors.go`. Place them after `ErrInvalidState`:

```go
	// ErrManifestInvalid is returned when a manifest fails to decode, contains
	// an unknown field, or is missing a required field. Callers distinguish
	// this from a version mismatch via errors.Is.
	ErrManifestInvalid = errors.New("container: manifest invalid")

	// ErrManifestVersion is returned when a manifest's Version field does not
	// match ManifestVersion. Callers distinguish this from a decode failure
	// via errors.Is.
	ErrManifestVersion = errors.New("container: manifest version mismatch")
```

No other changes to `errors.go`.

### Step 2: Create `manifest.go`

New file at `/home/jaime/tau/container/manifest.go`. Complete contents:

```go
package container

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	ManifestVersion = "1"
	ManifestPath    = "/etc/tau/manifest.json"
)

type Manifest struct {
	Version     string             `json:"version"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Base        string             `json:"base,omitempty"`
	Shell       string             `json:"shell"`
	Workspace   string             `json:"workspace,omitempty"`
	Env         map[string]string  `json:"env,omitempty"`
	Tools       map[string]Tool    `json:"tools,omitempty"`
	Services    map[string]Service `json:"services,omitempty"`
	Options     map[string]any     `json:"options,omitempty"`
}

type Tool struct {
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
}

type Service struct {
	Description string `json:"description,omitempty"`
}

func Parse(r io.Reader) (*Manifest, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}

	if err := Validate(&m); err != nil {
		return nil, err
	}

	return &m, nil
}

func Validate(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("%w: nil manifest", ErrManifestInvalid)
	}
	if m.Version != ManifestVersion {
		return fmt.Errorf("%w: got %q, want %q", ErrManifestVersion, m.Version, ManifestVersion)
	}
	if m.Name == "" {
		return fmt.Errorf("%w: missing required field: name", ErrManifestInvalid)
	}
	if m.Shell == "" {
		return fmt.Errorf("%w: missing required field: shell", ErrManifestInvalid)
	}
	return nil
}

func Fallback() *Manifest {
	return &Manifest{
		Version: ManifestVersion,
		Name:    "fallback",
		Shell:   "/bin/sh",
	}
}
```

Notes for the developer:
- No godoc on the exported identifiers — Phase 7 (Documentation) adds those.
- The version-check happens *before* name/shell in `Validate` so a version-mismatched manifest reports that as the primary failure, not whatever required field happens to be missing.
- `Parse` returns the already-wrapped error from `Validate` — no double-wrapping.

### Step 3: Add `Manifest` field to `ContainerInfo`

In `/home/jaime/tau/container/container.go`, locate the `ContainerInfo` struct and its preceding doc comment (currently lines 99-114).

**Change A** — rewrite the comment block above `ContainerInfo` so it no longer references "a manifest pointer once Objective 2 lands". Replace:

```go
// ContainerInfo is the full Runtime.Inspect response for a container. It is a
// superset of Container's shape and will grow additional fields in later
// phases (timestamps, exit codes, network settings, and a manifest pointer
// once Objective 2 lands).
```

with:

```go
// ContainerInfo is the full Runtime.Inspect response for a container. It is a
// superset of Container's shape and will grow additional fields in later
// phases (timestamps, exit codes, network settings).
```

**Change B** — append a `Manifest` field to the `ContainerInfo` struct, immediately after `Labels`:

```go
	// Manifest is the image capability manifest read from
	// /etc/tau/manifest.json at inspect time. Nil when the image has no
	// manifest; callers needing a non-nil value should substitute Fallback().
	Manifest *Manifest
```

No other changes to `container.go`.

### Step 4: Create test-data fixtures

Ensure the directory exists:

```
/home/jaime/tau/container/tests/testdata/
```

Create seven JSON fixtures. Keep indentation consistent (two spaces per level).

**`tests/testdata/manifest_full.json`** — every field populated:

```json
{
  "version": "1",
  "name": "tau-go-dev",
  "description": "Go development environment with common tools",
  "base": "alpine:3.21",
  "shell": "/bin/bash",
  "workspace": "/workspace",
  "env": {
    "GOPATH": "/go",
    "PATH": "/usr/local/go/bin:/go/bin:/usr/local/bin:/usr/bin:/bin"
  },
  "tools": {
    "go":  { "version": "1.26", "description": "Go compiler and toolchain" },
    "git": { "version": "2.47", "description": "Version control" }
  },
  "services": {
    "postgres": { "description": "PostgreSQL database for structured data" }
  },
  "options": {
    "docker": {
      "healthcheck": "/bin/sh -c 'exit 0'"
    },
    "retries": 3
  }
}
```

**`tests/testdata/manifest_minimal.json`** — only required fields:

```json
{
  "version": "1",
  "name": "minimal",
  "shell": "/bin/sh"
}
```

**`tests/testdata/manifest_bad_version.json`** — version mismatch:

```json
{
  "version": "2",
  "name": "x",
  "shell": "/bin/sh"
}
```

**`tests/testdata/manifest_missing_name.json`** — required field absent:

```json
{
  "version": "1",
  "shell": "/bin/sh"
}
```

**`tests/testdata/manifest_missing_shell.json`** — required field absent:

```json
{
  "version": "1",
  "name": "x"
}
```

**`tests/testdata/manifest_malformed.json`** — truncated JSON (no trailing brace):

```json
{
  "version": "1",
  "name": "x",
  "shell": "/bin/sh"
```

**`tests/testdata/manifest_unknown_field.json`** — strict-mode rejection case:

```json
{
  "version": "1",
  "name": "x",
  "shell": "/bin/sh",
  "foo": "bar"
}
```

### Step 5: Commit with the source changes

No test file is authored during developer execution — Phase 5 (Testing) is the AI's responsibility and will write `tests/manifest_test.go` against the fixtures above. Do not pre-create the test file.

## Validation Criteria

- [ ] `errors.go` has `ErrManifestInvalid` and `ErrManifestVersion` sentinels
- [ ] `manifest.go` defines `Manifest`, `Tool`, `Service`, `ManifestVersion`, `ManifestPath`, `Parse`, `Validate`, `Fallback` per the shapes above
- [ ] `Parse` uses `json.Decoder.DisallowUnknownFields()`
- [ ] `Validate` order: nil → version → name → shell
- [ ] `Fallback()` returns a manifest that passes `Validate`
- [ ] `container.go` `ContainerInfo` has `Manifest *Manifest` field; the "once Objective 2 lands" phrasing is removed from the preceding doc comment
- [ ] `tests/testdata/` contains the seven JSON fixtures
- [ ] `go build ./...` passes
- [ ] `go vet ./...` passes
- [ ] `go mod tidy` produces no diff
- [ ] `go mod graph` still shows only `protocol`/`format`/stdlib dependencies
