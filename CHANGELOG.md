# Changelog

## v0.1.0-dev.3.11
- Add `container/docker` sub-module: `Register()`, `LabelManaged`/`LabelManifestVersion` constants, lifecycle methods (`Create`/`Start`/`Stop`/`Remove`) with reserved-label merging, timeout-independent stop, and `ErrInvalidState` on `Remove(force=false)` for running containers; Exec/CopyTo/CopyFrom/Inspect stubbed for sub-issues #12 and #13; integration-test-with-skip pattern (`skipIfNoDaemon`, `ensureImage`) over `alpine:3.21` (#11)

## v0.1.0-dev.2.9
- Add image capability manifest types (`Manifest`, `Tool`, `Service`), constants (`ManifestVersion`, `ManifestPath`), and `Parse`/`Validate`/`Fallback` with strict JSON decoding and an `Options` pass-through slot; add `ErrManifestInvalid`/`ErrManifestVersion` sentinels; add `ContainerInfo.Manifest` field (#9)

## v0.1.0-dev.1.6
- Define OCI-aligned `Runtime` interface (8 methods) with per-method context-cancellation godoc and add thread-safe factory registry (`Factory`, `Register`, `Create`, `ListRuntimes`) wrapping `ErrRuntimeNotFound` for unknown names (#6)

## v0.1.0-dev.1.5
- Introduce root module `go.mod`, core container types (`Container`, `State`, `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo`), and sentinel domain errors (`ErrRuntimeNotFound`, `ErrContainerNotFound`, `ErrInvalidState`) (#5)
