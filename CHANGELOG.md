# Changelog

## [v0.1.0] - 2026-04-15

Initial release. OCI-aligned runtime abstraction and image capability manifest convention for the TAU ecosystem.

**Added**:
- Core container types: `Container`, `State` (StateCreated, StateRunning, StateExited, StateRemoved), `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo`
- Sentinel domain errors: `ErrRuntimeNotFound`, `ErrContainerNotFound`, `ErrInvalidState`, `ErrManifestInvalid`, `ErrManifestVersion`
- OCI-aligned `Runtime` interface (8 methods: Create, Start, Stop, Remove, Exec, CopyTo, CopyFrom, Inspect) with per-method context-cancellation godoc
- Thread-safe factory registry: `Factory`, `Register`, `Create`, `ListRuntimes`
- Image capability manifest types (`Manifest`, `Tool`, `Service`) and constants (`ManifestVersion`, `ManifestPath`)
- `Parse` / `Validate` / `Fallback` with strict JSON decoding (`DisallowUnknownFields`) and an `Options` pass-through slot
- `ContainerInfo.Manifest` field for inspect-time manifest surfacing
