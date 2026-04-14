# 9 — Image Capability Manifest Types and Validation

## Summary

Landed the image capability manifest convention at the type level: `Manifest`, `Tool`, `Service` types, `ManifestVersion`/`ManifestPath` constants, `Parse`/`Validate`/`Fallback` functions, and two new error sentinels (`ErrManifestInvalid`, `ErrManifestVersion`). Added the `Manifest *Manifest` field to `ContainerInfo`, closing the TODO deferred from Objective 1. Seven golden JSON fixtures plus a black-box test file cover parse success/failure paths, validation cases, fallback round-trip, and the well-known path constant.

Issue #9 was the single sub-issue of Objective 2; Phase 1 Objective 2 closes with this PR. Unblocks Objective 3 (Docker runtime `Inspect`) which will read the manifest via `Runtime.CopyFrom(ManifestPath)` and populate `ContainerInfo.Manifest`.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| JSON decode strictness | `DisallowUnknownFields` | Catches drift at load time rather than silently dropping unknown fields. Strict-now/relax-later preserves more optionality than the reverse. |
| Pass-through slot | Top-level `Options map[string]any` on `Manifest` only | Isolates runtime- or image-specific config that tau doesn't interpret. Mirrors `tau/protocol/config.ProviderConfig.Options` and `tau/format.*.Options`. Not carried onto `Tool`/`Service` — their schemas are narrow and well-scoped. |
| Version check order in `Validate` | Version before name/shell | A version-mismatched manifest reports `ErrManifestVersion` rather than whichever required field is absent first. |
| `Fallback` location | Provided by this package, not callers | Callers (Obj 3 `Inspect`) get a known-valid manifest without having to encode defaults themselves. Round-trip tested. |
| Configuration planning | Deferred | Surveyed `tau/protocol/config` and kernel patterns. `ManifestVersion`/`ManifestPath` belong as package constants (protocol negotiation + well-known path), not config fields. A real `container/config/` package will land at Phase 2 start when runtime-tunable knobs (timeouts, retry policy) arrive. |
| Package reorganization | Deferred | Surveyed layouts across protocol/format/provider/agent/orchestrate. Container at 5 root files is within TAU norms. Revisit at Phase 1 → Phase 2 boundary when `shell`/`tools` work adds pressure. |

## Files Modified

Created:
- `manifest.go`
- `tests/manifest_test.go`
- `tests/testdata/manifest_full.json`
- `tests/testdata/manifest_minimal.json`
- `tests/testdata/manifest_bad_version.json`
- `tests/testdata/manifest_missing_name.json`
- `tests/testdata/manifest_missing_shell.json`
- `tests/testdata/manifest_malformed.json`
- `tests/testdata/manifest_unknown_field.json`
- `.claude/context/guides/9-image-capability-manifest.md` (archived at closeout)
- `.claude/plans/linked-whistling-honey.md`

Edited:
- `errors.go` — added `ErrManifestInvalid`, `ErrManifestVersion`
- `container.go` — added `ContainerInfo.Manifest`, updated block comment
- `_project/README.md` — added `options` field to the manifest JSON example and shape description; documented strict decode posture and `Fallback()` usage
- `_project/phase.md` — marked Objective 2 as Done; added strict-decode + Options pass-through to cross-cutting decisions
- `_project/objective.md` — marked objective and sub-issue as Done
- `.claude/CLAUDE.md` — captured strict-decode + Options pass-through under Container Conventions

## Patterns Established

- **Strict JSON decode for tau-owned schemas.** When tau defines a JSON contract, `DisallowUnknownFields` is the default posture. Pass-through for adjacent tooling lives under an explicit `Options map[string]any` slot, not at the root.
- **Sentinel + `%w` wrap with value context.** `ErrManifestVersion` wrap format `fmt.Errorf("%w: got %q, want %q", ErrManifestVersion, actual, ManifestVersion)` — the sentinel identifies the failure mode, the suffix carries actionable detail.
- **Constants vs. config.** Values that negotiate a protocol (schema version) or name a well-known location (manifest path) stay as exported package constants. They are not runtime-tunable.

## Validation Results

- `go build ./...` — pass
- `go vet ./...` — pass
- `go test ./tests/...` — pass (all existing registry/types tests plus new manifest tests)
- `go test -coverpkg=github.com/tailored-agentic-units/container ./tests/...` — **100.0% coverage** of the container package
- `go mod tidy` — no diff
- `go mod graph` — container root still has only `protocol`/`format`/stdlib deps (no new heavy imports)
