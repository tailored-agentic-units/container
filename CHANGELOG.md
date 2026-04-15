# Changelog

## v0.1.0-dev.3.13
- Implement Docker runtime `Inspect`: map `ContainerJSON` to `ContainerInfo` (strip leading `/` from `Name`, source `Image` from `Config.Image`); private `mapState` normalizes Docker `Status` to `container.State` and rejects `paused` (Phase 1 excludes Paused) plus unknown statuses; manifest at `container.ManifestPath` is read via `CopyFrom` and parsed via `container.Parse` — missing file → `Manifest == nil` with no error, malformed → wraps `ErrManifestInvalid`, version mismatch → wraps `ErrManifestVersion`; `docker/tests/inspect_test.go` covers vanilla nil-manifest, running/exited state mapping, paused → error, well-formed manifest, malformed/version-mismatch error surfaces, and ctx cancellation (#13)

## v0.1.0-dev.3.12
- Implement Docker runtime `Exec` (one-shot command with stdout/stderr capture, exit-code reporting, and ctx-cancellation via hijacked-conn close), `CopyTo` (in-memory tar build with auto `mkdir -p` parent via `Exec`), and `CopyFrom` (tar-stream unwrap via `tarFileReader` adapter that honors ctx on `Read`); document `cerrdefs.IsNotFound` for the missing-file case; `docker/tests/io_test.go` covers stdout/stderr capture, no-attach nil buffers, non-zero exit, ctx cancel, copy round-trip + nested path, and absent-file detection (#12)

## v0.1.0-dev.3.11
- Add `container/docker` sub-module: `Register()`, `LabelManaged`/`LabelManifestVersion` constants, lifecycle methods (`Create`/`Start`/`Stop`/`Remove`) with reserved-label merging, timeout-independent stop, and `ErrInvalidState` on `Remove(force=false)` for running containers; Exec/CopyTo/CopyFrom/Inspect stubbed for sub-issues #12 and #13; integration-test-with-skip pattern (`skipIfNoDaemon`, `ensureImage`) over `alpine:3.21` (#11)

## v0.1.0-dev.2.9
- Add image capability manifest types (`Manifest`, `Tool`, `Service`), constants (`ManifestVersion`, `ManifestPath`), and `Parse`/`Validate`/`Fallback` with strict JSON decoding and an `Options` pass-through slot; add `ErrManifestInvalid`/`ErrManifestVersion` sentinels; add `ContainerInfo.Manifest` field (#9)

## v0.1.0-dev.1.6
- Define OCI-aligned `Runtime` interface (8 methods) with per-method context-cancellation godoc and add thread-safe factory registry (`Factory`, `Register`, `Create`, `ListRuntimes`) wrapping `ErrRuntimeNotFound` for unknown names (#6)

## v0.1.0-dev.1.5
- Introduce root module `go.mod`, core container types (`Container`, `State`, `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo`), and sentinel domain errors (`ErrRuntimeNotFound`, `ErrContainerNotFound`, `ErrInvalidState`) (#5)
