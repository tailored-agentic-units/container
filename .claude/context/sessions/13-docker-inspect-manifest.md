# 13 - Docker runtime Inspect and manifest integration

## Summary

Replaced the placeholder `Inspect` body in `docker/docker.go` with a real
implementation that maps Docker's `ContainerJSON` to `container.ContainerInfo`
and reads the image capability manifest at `container.ManifestPath` via the
runtime's own `CopyFrom`. Added a private `mapState` helper that normalizes
Docker's `Status` string into the four-value `container.State` set, with
explicit error rejection for `paused` (Phase 1 excludes Paused) and unknown
statuses. Added eight black-box integration tests covering all acceptance
criteria. Closes the container repo's contribution to Objective #3 — the
library at `main` is now feature-complete for Phase 1 and ready for the
v0.1.0 / docker/v0.1.0 release.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Manifest read path | `r.CopyFrom(...)` (own method) | Keeps the manifest convention runtime-method-mediated and reuses tar-stripping in `CopyFrom`/`tarFileReader`. |
| `isNotFound` predicate | Use `cerrdefs.IsNotFound` directly | Already imported; works through wrapped error chain. A one-line wrapper for one caller adds no value. |
| `paused` state mapping | Return error (don't coerce) | Phase 1 state set excludes Paused. Silent coercion would hide a state the runtime cannot represent. |
| `Name` normalization | Strip leading `/` | Matches what callers passed to `CreateOptions.Name` and what `Container.Name` reports at Create time. |
| `Image` source | `Config.Image` (not top-level `Image`) | The originally requested reference (e.g., `alpine:3.21`), not the resolved image ID hash. |
| `mapState` location | Private helper in `docker.go` | Black-box test convention rules out a white-box unit test; `mapState` is exercised through `Inspect` integration tests covering created/running/exited/paused. |

## Files Modified

- `docker/docker.go` — `Inspect` body, `mapState` helper, `strings` import, godoc on `Inspect`
- `docker/tests/inspect_test.go` — new black-box integration tests
- `_project/objective.md` — sub-issue C marked Done
- `CHANGELOG.md` — added `v0.1.0-dev.3.13` entry

## Patterns Established

- **Manifest read goes through `Runtime.CopyFrom`.** Future runtime implementations (containerd, podman) should follow the same pattern: call their own `CopyFrom`, parse via `container.Parse`, and surface absent files as `Manifest == nil`. This keeps the manifest convention fully runtime-agnostic at the type level.
- **State normalization rejects unknown states explicitly.** Runtimes whose native state model includes states outside the four-value `container.State` set should return errors rather than silent coercion. Phase-by-phase additions to `State` are visible at the boundary instead of masked.

## Validation Results

- `go build ./...` — passes (root + docker)
- `go vet ./...` — passes (root + docker)
- `go test ./tests/...` — passes; eight new `TestInspect_*` tests added, all pass
- Coverage: 78.1% of the docker package via `go test -coverpkg=...`. Below the issue's literal 80% number but the gap is in defensive `mapState` branches (`nil` state, unknown statuses, `dead`, `restarting`, `removing`) that integration tests cannot reasonably trigger. Per the user's clarification mid-session, the real target is reasonable coverage of public infrastructure rather than a hard 80% number — public Inspect behavior is well covered.

## Pre-existing Issues Observed (Not in Scope)

- `TestExec_NonZeroExit` (sub-issue #12) is flaky: races the daemon-side exit-code write after `ContainerExecAttach` completes, occasionally returning ExitCode 0 instead of the actual exit code. Passes on retry. Worth a follow-up in the Exec code path.
