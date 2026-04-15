# 11 — Docker sub-module scaffold and lifecycle methods

## Summary

Landed `container/docker` sub-module: module declaration, package godoc, `dockerRuntime` type implementing all 8 `Runtime` methods (4 real — Create/Start/Stop/Remove — and 4 stubs deferred to #12/#13), parameterless `Register()` wiring a `client.FromEnv` default factory, exported `LabelManaged` / `LabelManifestVersion` constants, and the integration-test-with-skip pattern (`skipIfNoDaemon`, `ensureImage`) that sub-issues #12 and #13 reuse. Five black-box integration tests cover lifecycle round-trip, label application, reserved-label precedence, and force-vs-non-force Remove semantics.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Root module dependency | Pseudo-version targeting `origin/main` | No root tag exists yet (deferred until library is runnable per feedback memory); `replace` directives forbidden by CLAUDE.md. `go get github.com/tailored-agentic-units/container@main` produces the standard Go pseudo-version. |
| Conflict detection | `cerrdefs.IsConflict` from `github.com/containerd/errdefs` | Docker's `errdefs.IsConflict` is deprecated in favor of the containerd errdefs module. |
| `Remove(force=false)` on running container | Pre-inspect + remap | Inspect first, return `ErrInvalidState` without hitting the API; also remap daemon conflict errors to cover the TOCTOU race. Lets callers branch on `errors.Is(err, container.ErrInvalidState)` regardless of which check fires. |
| `Stop` timeout of 0 or negative | Pass `nil` to Docker | Uses daemon default rather than interpreting `&0` as "kill immediately." |
| Label merge | `maps.Clone` + nil guard | Terser than a copy loop; guard handles nil caller map without panicking on write. |
| Env encapsulation | `buildEnv` helper | Isolates env transformation for future tweaks (sorting, key validation) without cluttering `Create`. |
| `Register()` location | Inline in `docker.go` | Matches `provider/azure`, `provider/bedrock` sibling pattern and the phase.md cross-cutting decision. |
| Workspace inclusion | Added `./container/docker` to `tau/go.work` | Matches the pattern used for `./provider/azure`, `./provider/bedrock`, etc. |

## Files Modified

- `docker/go.mod` — new, requires `tau/container` (pseudo-version), `docker/docker` v28.5.2, `containerd/errdefs` v1.0.0
- `docker/go.sum` — new, auto-generated
- `docker/doc.go` — new, package godoc with Register usage snippet
- `docker/docker.go` — new, full runtime implementation + label constants + helpers
- `docker/tests/helpers_test.go` — new, `skipIfNoDaemon` and `ensureImage`
- `docker/tests/lifecycle_test.go` — new, 5 integration tests
- `.claude/CLAUDE.md` — Project Structure updated to list `docker/doc.go` + clarify `docker.go` exports label constants
- `../go.work` — added `./container/docker` to the workspace use list

## Patterns Established

- **Integration test skip pattern.** Each test calls `skipIfNoDaemon(t)` to get a client (2s Ping timeout), then `ensureImage(t, cli, ref)` to pull or skip on offline. `t.Cleanup` with `Remove(force=true)` guarantees teardown on failure. Subsequent sub-modules and sub-issues (#12, #13, and any future containerd runtime) reuse this shape.
- **Label-merge helper with `maps.Clone`.** Cleaner than a copy loop. The nil guard is required because `maps.Clone(nil)` returns `nil`, and writes to nil maps panic.
- **Stub method body convention.** Stubs return `fmt.Errorf("docker <op>: not implemented in sub-issue #11")` — no sentinel, no TODO marker. Sub-issues #12 and #13 overwrite these bodies wholesale.
- **Reserved-label precedence via helper.** `mergeLabels` writes reserved keys *after* the caller map clone, guaranteeing reserved wins without a per-key check.

## Validation Results

From `/home/jaime/tau/container`:
- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./tests/...` — pass (root tests)

From `/home/jaime/tau/container/docker`:
- `go build ./...` — clean
- `go vet ./...` — clean
- `go mod tidy` — clean diff
- `go test ./tests/...` — pass (5 integration tests, ~3.4s against local Docker daemon with `alpine:3.21`)

Acceptance criteria from issue #11: all satisfied except the defensively-listed "Remove examples/ from CLAUDE.md Project Structure" — CLAUDE.md never listed `examples/` (verified at plan time), so that bullet was a no-op.
