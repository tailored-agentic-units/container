# Changelog

## v0.1.0-dev.1.6
- Define OCI-aligned `Runtime` interface (8 methods) with per-method context-cancellation godoc and add thread-safe factory registry (`Factory`, `Register`, `Create`, `ListRuntimes`) wrapping `ErrRuntimeNotFound` for unknown names (#6)

## v0.1.0-dev.1.5
- Introduce root module `go.mod`, core container types (`Container`, `State`, `CreateOptions`, `ExecOptions`, `ExecResult`, `ContainerInfo`), and sentinel domain errors (`ErrRuntimeNotFound`, `ErrContainerNotFound`, `ErrInvalidState`) (#5)
