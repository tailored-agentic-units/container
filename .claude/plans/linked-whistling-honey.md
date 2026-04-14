# Issue #9 — Image Capability Manifest Types and Validation

## Context

Objective 2 introduces the image capability manifest convention — a JSON file at `/etc/tau/manifest.json` that a container image carries to declare its shell, workspace, env, tools, and services to tau agents. Objective 1 landed `Runtime` and `ContainerInfo` with a TODO placeholder at `container.go:99-102` for a `Manifest` pointer. Objective 3 (Docker runtime) will read `/etc/tau/manifest.json` via `Runtime.CopyFrom` during `Inspect` and populate that field.

This issue is the single sub-issue of Objective 2: land the manifest types, parse/validate/fallback behavior, error sentinels, and the `ContainerInfo.Manifest` field, with black-box tests. It unblocks Objective 3 and has zero new heavy dependencies (stdlib-only: `encoding/json`, `errors`, `fmt`, `io`).

## Scope Refinement (from conversation)

Two user-directed decisions landed during session planning, deviating slightly from the objective-planning doc:

1. **Strict decoding.** `Parse` MUST call `json.Decoder.DisallowUnknownFields()` — any field outside the declared Manifest/Tool/Service shape is a decode error that wraps `ErrManifestInvalid`. This prevents drift where images silently attach unsupported fields and callers never notice.

2. **Controlled pass-through via `Options`.** The manifest declares its own `Options map[string]any` field (json tag `options,omitempty`) as the single isolated slot for container-runtime-specific configuration that tau itself does not interpret. Mirrors the established TAU pattern at `tau/protocol/config/provider.go:11` and `tau/format/data.go:9`. `Options` is kept at the top-level `Manifest` only — `Tool` and `Service` retain their tight `version`/`description` schemas.

Documentation consequence: `_project/README.md` needs a parallel update during Phase 7 to add `options` to the example JSON and to the manifest-shape description. The objective.md note "Types — verbatim from README" is superseded by this decision.

## Critical Files

| File | Action | Purpose |
|------|--------|---------|
| `manifest.go` | create | Types (`Manifest`, `Tool`, `Service`), constants (`ManifestVersion`, `ManifestPath`), funcs (`Parse`, `Validate`, `Fallback`) |
| `errors.go` | edit | Add `ErrManifestVersion` and `ErrManifestInvalid` sentinels |
| `container.go` | edit | Add `Manifest *Manifest` field to `ContainerInfo`; remove TODO at lines 99-102 |
| `tests/manifest_test.go` | create | Black-box table-driven tests |
| `tests/testdata/*.json` | create | Golden fixtures (full, minimal, bad-version, missing-field, malformed, unknown-field) |
| `_project/README.md` | edit (Phase 7) | Add `options` to example JSON + shape description |

Existing patterns to follow (already in the codebase):
- Sentinel style in `errors.go:8-23` — `var Err... = errors.New("container: ...")`
- Godoc on every exported identifier — see `container.go`, `runtime.go`, `registry.go`
- Test file naming and black-box convention — see `tests/types_test.go`, `tests/registry_test.go`
- `Options map[string]any` pass-through — `tau/protocol/config/provider.go:11`

## Implementation

### Step 1 — `errors.go`

Append two sentinels to the existing `var (...)` block:

- `ErrManifestInvalid` — "container: manifest invalid" — decode failure, unknown field, or missing required field.
- `ErrManifestVersion` — "container: manifest version mismatch" — `Version` field is not `ManifestVersion`.

Callers detect both via `errors.Is`.

### Step 2 — `manifest.go` (new file)

Package clause `package container`. No imports beyond `encoding/json`, `errors`, `fmt`, `io`.

Types matching the JSON shape, plus the `Options` pass-through slot:

```go
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

const (
    ManifestVersion = "1"
    ManifestPath    = "/etc/tau/manifest.json"
)
```

Functions:

- `Parse(r io.Reader) (*Manifest, error)` — use `json.NewDecoder(r)`, call `DisallowUnknownFields()`, then `Decode` into a `*Manifest`. On decode error return `fmt.Errorf("container: %w: %v", ErrManifestInvalid, err)`. On success call `Validate` and return its result (already wrapped).
- `Validate(m *Manifest) error` — check in order:
  1. `m == nil` → wrapped `ErrManifestInvalid` ("nil manifest")
  2. `m.Version != ManifestVersion` → wrapped `ErrManifestVersion` including the bad version
  3. `m.Name == ""` → wrapped `ErrManifestInvalid` ("missing required field: name")
  4. `m.Shell == ""` → wrapped `ErrManifestInvalid` ("missing required field: shell")
  5. otherwise `nil`
- `Fallback() *Manifest` — returns `&Manifest{Version: ManifestVersion, Name: "fallback", Shell: "/bin/sh"}`. Must pass `Validate` (round-trip safe).

### Step 3 — `container.go`

At `container.go:99-102`, the current godoc says:

> … will grow additional fields in later phases (timestamps, exit codes, network settings, and a manifest pointer once Objective 2 lands).

Rewrite the block comment so it no longer references "once Objective 2 lands" and append the new field to `ContainerInfo`:

```go
// Manifest is the image capability manifest read from /etc/tau/manifest.json
// at inspect time. Nil when the image has no manifest; callers needing a
// non-nil value should substitute Fallback().
Manifest *Manifest
```

Keep the godoc accurate: `ContainerInfo` is still a superset of `Container` and may still grow (timestamps, exit codes, network settings); only the manifest-TODO text is removed.

### Step 4 — `tests/testdata/` fixtures (new)

Minimal golden JSON files:

- `manifest_full.json` — every field populated, including `options` with a mixed-type map.
- `manifest_minimal.json` — only `version`, `name`, `shell`.
- `manifest_bad_version.json` — `version: "2"`, otherwise valid.
- `manifest_missing_name.json` — valid shape with `name` absent.
- `manifest_missing_shell.json` — valid shape with `shell` absent.
- `manifest_malformed.json` — truncated/invalid JSON.
- `manifest_unknown_field.json` — a top-level field not in the schema (e.g. `"foo": "bar"`).

### Step 5 — `tests/manifest_test.go` (new)

`package container_test`. Import the standard testing, `errors`, `os`, `strings`, and the container package.

Tests (table-driven where they share shape):

1. `TestParse_Valid` — loads `manifest_full.json` and `manifest_minimal.json`, asserts no error and spot-checks a few decoded fields (version, name, shell, and for full: a tool entry, a service entry, an options value).
2. `TestParse_Errors` — table of (fixture file, expected sentinel via `errors.Is`): bad-version → `ErrManifestVersion`; missing-name → `ErrManifestInvalid`; missing-shell → `ErrManifestInvalid`; malformed → `ErrManifestInvalid`; unknown-field → `ErrManifestInvalid`.
3. `TestValidate_Direct` — constructs manifests in-code (nil, wrong version, missing name, missing shell, fully valid) and asserts sentinel identity via `errors.Is`.
4. `TestFallback_RoundTrip` — `Fallback()` is non-nil and passes `Validate`.
5. `TestParse_RejectsUnknownField` — explicit assertion (may fold into #2) that decoding a manifest with an extra top-level field fails with `ErrManifestInvalid`. This is the contract that forces pass-through to flow through `Options`.

Use `os.Open` on fixture paths relative to the test file (`testdata/manifest_*.json` — Go resolves `testdata` from the test's package directory).

## Validation

End-to-end checks run from repo root:

- `go build ./...` — package compiles.
- `go vet ./...` — no vet warnings.
- `go test ./tests/...` — all manifest tests plus existing types/registry tests pass.
- `go mod tidy` — produces no diff.
- `go mod graph | grep -v '^github.com/tailored-agentic-units/'` — still shows only `protocol`, `format`, and stdlib.

Manual sanity:

- Grep for the TODO at `container.go:99-102`: should be gone.
- Confirm `errors.Is(err, container.ErrManifestInvalid)` and `errors.Is(err, container.ErrManifestVersion)` return true for the respective failure modes (covered by `TestParse_Errors`).

## Deferred to Phase 7 (Documentation)

- Update `_project/README.md` manifest JSON example and shape description to include the `options` field and explain its pass-through role.
- Add godoc to every new exported identifier in `manifest.go`.
- Update `.claude/CLAUDE.md` only if structure changes warrant it (no new packages here; likely no change).

## Out of Scope

- Reading the manifest from a running container (Objective 3 — `Runtime.CopyFrom`).
- Tool definition generation from manifest entries (Phase 2).
- Custom JSON-Schema parameter declarations on tools (deferred open question).
- Per-field validation beyond the three required fields (version, name, shell) — `Options` contents and other optional fields are black boxes.
