# Objective 2 Planning — Image Capability Manifest

## Context

**Why this session**: `/dev-workflow objective 2` — decompose Objective #2 ("Image Capability Manifest") into sub-issue(s) that can each be executed in a single task session.

**Where Obj 2 sits**: Phase 1 (Runtime Foundation, v0.1.0). Independent of Obj 1 (now fully closed: #5 + #6 merged). Obj 3 (Docker Runtime) depends on Obj 2 because the Docker `Inspect` populates `ContainerInfo.Manifest` via `Runtime.CopyFrom("/etc/tau/manifest.json")`.

**What Obj 2 produces**: `manifest.go` in the root module — types matching the JSON shape in `_project/README.md`, parse + validate + fallback functions, and golden-fixture tests. Plus a small follow-up edit to `container.go` adding the deferred `Manifest *Manifest` field to `ContainerInfo` (intentionally omitted in #5 with a TODO).

## Step 0 — Transition Closeout (Objective 1)

Status assessment via GraphQL: 2/2 sub-issues closed (#5 Done, #6 Done). No incomplete work. Step 0b skipped.

**Clean-slate actions** (deferred to execution, listed for visibility):

1. Update `_project/phase.md` row 1: `Planned` → `Done` for Obj 1.
2. Delete `_project/objective.md` (will be recreated for Obj 2 in Step 5).
3. Close Objective 1 issue: `gh issue close 1 --repo tailored-agentic-units/container`.

## Architecture Decisions

### One sub-issue, not two

Scope is tightly coupled (~150 LOC of types + parse/validate/fallback in one file, plus one cohesive test file). Splitting types from parse would leave the first PR with no testable behavior. Mirrors the Phase 1 precedent where atomic, independently shippable units of work get one PR each.

### Manifest shape — verbatim from `_project/README.md`

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
}

type Tool struct {
    Version     string `json:"version,omitempty"`
    Description string `json:"description,omitempty"`
}

type Service struct {
    Description string `json:"description,omitempty"`
}

const ManifestVersion = "1"
const ManifestPath   = "/etc/tau/manifest.json"
```

### API surface

```go
func Parse(r io.Reader) (*Manifest, error)   // decode + Validate
func Validate(m *Manifest) error              // version check, required fields
func Fallback() *Manifest                     // POSIX-shell defaults
```

- `Parse` decodes JSON, then calls `Validate`. Decode errors and validation errors both return wrapped sentinel errors.
- `Validate` enforces: `Version == ManifestVersion`, non-empty `Name`, non-empty `Shell`.
- Unknown version returns a typed error so callers can distinguish "wrong version" from "malformed JSON". Add to `errors.go`:
  - `ErrManifestVersion` — version mismatch
  - `ErrManifestInvalid` — missing required field / decode failure
- `Fallback()` returns: `Version: "1"`, `Name: "fallback"`, `Shell: "/bin/sh"`, no tools/services. Callers (Obj 3 `Inspect`) use this when `/etc/tau/manifest.json` is absent.

### Container types update

Add to `ContainerInfo` in `container.go`:

```go
// Manifest is the image capability manifest read from /etc/tau/manifest.json
// at inspect time. Nil when the image has no manifest (callers should
// substitute Fallback() if they need a non-nil value).
Manifest *Manifest
```

This closes the TODO already documented in `container.go:99-102` and `_project/objective.md:34` (sub-issue #5 noted Manifest field deferred to Obj 2).

## Sub-Issue Plan

### Sub-issue 2.1 — Implement image capability manifest types and validation

**Repo**: `tailored-agentic-units/container`
**Labels**: `feature`
**Issue type**: Task
**Parent**: #2
**Milestone**: Phase 1 - Runtime Foundation
**Branch (when executed)**: `7-image-capability-manifest` (auto-named from issue number when created)

**Body (Context / Scope / Approach / Acceptance Criteria)**:

- **Context**: First and only sub-issue of Obj #2. Independent of Obj #1's runtime/registry work (already merged via #5, #6). Unblocks Obj #3 Docker `Inspect` which will read `/etc/tau/manifest.json` via `Runtime.CopyFrom`.
- **Scope**:
  - `manifest.go` — `Manifest`, `Tool`, `Service` types; `ManifestVersion`/`ManifestPath` constants; `Parse`, `Validate`, `Fallback`
  - `errors.go` — add `ErrManifestVersion`, `ErrManifestInvalid` sentinels
  - `container.go` — add `Manifest *Manifest` field to `ContainerInfo` (closes the TODO at line 99-102)
  - `tests/manifest_test.go` — golden JSON fixtures (`tests/testdata/manifest_*.json`) covering: full valid manifest, minimal valid manifest, version mismatch (returns `ErrManifestVersion`), missing required field (returns `ErrManifestInvalid`), malformed JSON (returns `ErrManifestInvalid`), `Fallback()` returns POSIX-shell defaults that pass `Validate`
- **Approach**: see Architecture Decisions above
- **Acceptance Criteria**:
  - [ ] `manifest.go` types match `_project/README.md` JSON shape; godoc on every exported identifier
  - [ ] `Parse` returns `ErrManifestInvalid` for decode errors and missing required fields
  - [ ] `Parse` returns `ErrManifestVersion` for version mismatch (verifiable via `errors.Is`)
  - [ ] `Fallback()` returns a manifest that passes `Validate` (round-trip safe)
  - [ ] `ContainerInfo.Manifest *Manifest` field added with godoc explaining nil semantics
  - [ ] Black-box tests in `tests/manifest_test.go` (`package container_test`); golden fixtures under `tests/testdata/`
  - [ ] `go test ./tests/...`, `go vet ./...`, `go build ./...` all pass
  - [ ] `go mod graph` still shows only `protocol`/`format`/stdlib (no new heavy deps)

## Files To Modify (during execution)

| Path | Change |
|------|--------|
| `manifest.go` | New file — types, constants, Parse/Validate/Fallback |
| `errors.go` | Add `ErrManifestVersion`, `ErrManifestInvalid` |
| `container.go` | Add `Manifest *Manifest` field to `ContainerInfo`; remove TODO comment at lines 99-102 |
| `tests/manifest_test.go` | New file — table-driven black-box tests |
| `tests/testdata/manifest_*.json` | New golden fixtures (full, minimal, bad-version, missing-field, malformed) |
| `_project/objective.md` | Replace Obj 1 content with Obj 2 sub-issue table & decisions |
| `_project/phase.md` | Mark Obj 1 row Done; Obj 2 row → In Progress |

## Step 4–6 Operations (this session, after plan approval)

1. **Close out Obj 1**: update `_project/phase.md` (Obj 1 → Done), delete `_project/objective.md`, close issue #1 on GitHub.
2. **Create sub-issue 2.1** on `tailored-agentic-units/container` with the body above; assign `Task` issue type via GraphQL `updateIssueIssueType`; link to parent #2 via `addSubIssue`; set milestone "Phase 1 - Runtime Foundation"; apply `feature` label.
3. **Project board**: add the new sub-issue to TAU Container project (#9, `PVT_kwDOD155C84BUESJ`); set `Phase` field to `Phase 1 - Runtime Foundation` (option id `1be1afec`); set `Status` to `Todo`. Also update Obj 1's project item Phase to `Done` (option id `5425ec6f`).
4. **Write `_project/objective.md`** with: title, parent issue link (#2), phase link, scope, acceptance criteria (lifted from issue body), sub-issues table (one row), architecture decisions section.
5. **Update `_project/phase.md`**: Obj 1 status → `Done`; Obj 2 status → `In Progress`.

## Verification

After execution:

- `gh issue view 2 --repo tailored-agentic-units/container --json subIssuesSummary` shows 1 sub-issue linked
- New sub-issue appears on project board in `Todo` status, `Phase 1 - Runtime Foundation` phase
- `_project/objective.md` describes Obj 2 with the single sub-issue
- `_project/phase.md` reflects Obj 1 done, Obj 2 in progress
- Issue #1 closed on GitHub
