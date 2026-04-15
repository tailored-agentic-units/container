# Changelog

## [v0.1.0] - 2026-04-15

Initial release. Docker Engine API implementation of the `container.Runtime` interface.

**Added**:
- `dockerRuntime` implementing all 8 `container.Runtime` methods against the Docker Engine API
- `Register()` wiring a default factory that builds a client via `client.FromEnv` with API version negotiation (no `init()` auto-registration)
- Reserved label constants `LabelManaged` (`tau.managed`) and `LabelManifestVersion` (`tau.manifest.version`); reserved keys win on collision with caller-supplied labels
- Lifecycle methods: `Create`, `Start`, `Stop` (timeout independent of `ctx`), `Remove` (`force=false` on running containers wraps `ErrInvalidState`)
- `Exec`: one-shot command with stdout/stderr capture, exit-code reporting, and ctx-cancellation via hijacked-conn close
- `CopyTo`: in-memory tar build with auto `mkdir -p` parent via `Exec`
- `CopyFrom`: tar-stream unwrap via `tarFileReader` adapter that honors `ctx` on `Read`; missing files detectable via `cerrdefs.IsNotFound`
- `Inspect`: maps `ContainerJSON` to `ContainerInfo` (strips leading `/` from `Name`, sources `Image` from `Config.Image`); private `mapState` normalizes Docker `Status` to `container.State` and rejects `paused` (Phase 1 excludes Paused) plus unknown statuses; manifest at `container.ManifestPath` is read via `CopyFrom` and parsed via `container.Parse` — missing file → `Manifest == nil` with no error, malformed → wraps `ErrManifestInvalid`, version mismatch → wraps `ErrManifestVersion`
- Integration-test-with-skip pattern (`skipIfNoDaemon`, `ensureImage`) over `alpine:3.21`
