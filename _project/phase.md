# Phase 2 — Agent Tool Bridge

**Version target:** v0.2.0

## Scope

Build the agent-facing surface on top of the Phase 1 runtime foundation: a persistent `Shell` wrapping a new streaming exec primitive, structured built-in tools (file and process), and a manifest-driven tool surface that aggregates everything into the `[]format.ToolDefinition` slice that `agent.Tools(ctx, msgs, tools, opts...)` consumes. The persistent shell is the lynchpin — without it, an agent loses true shell semantics (history, sourced rc files, background jobs) and the "agent operates as a user" property the vision rests on.

## Objectives

| Issue | Objective | Status |
|-------|-----------|--------|
| [#18](https://github.com/tailored-agentic-units/container/issues/18) | Persistent Shell Foundation | Done |
| [#19](https://github.com/tailored-agentic-units/container/issues/19) | Structured Built-in Tools | Todo |
| [#20](https://github.com/tailored-agentic-units/container/issues/20) | Manifest-Driven Tool Surface & Agent Integration | Todo |

## Constraints

- Objectives #18 and #19 are independent and may proceed in parallel
- Objective #20 depends on both Obj #18 (for `Shell` as the `shell` built-in tool) and Obj #19 (for the top-level `Tool` type and the manifest sub-package)
- Sub-issue 20C (toolkit-mode example in `tailored-agentic-units/examples`) is gated on the `v0.2.0` and `docker/v0.2.0` release tags
- Root `go.mod` tagged `v0.2.0` and `docker/` sub-module tagged `docker/v0.2.0` at phase release; phase closeout waits for sub-issue 20C to merge against the published tags before running

## Cross-cutting decisions

- **Streaming exec primitive**: new `Runtime.ExecStream(ctx, id, opts) (*ExecSession, error)` method (not an `ExecOptions.Stream` flag). `ExecSession` exposes `Stdin io.WriteCloser`, `Stdout io.Reader`, `Stderr io.Reader`, `Wait() (int, error)`, `Close() error`. Cancellation semantics extend the existing context model: cancelling `ctx` aborts the in-flight `ExecStream` API call; `Close` kills the session early; `Wait` returns when the process exits.
- **Manifest sub-package**: manifest types extracted to a new `github.com/tailored-agentic-units/container/manifest` sub-package (single `go.mod`, no module split). `manifest.Tool` describes CLI metadata declared in the image; `container.Tool` (new in Obj #19) is the runtime execution unit pairing a `format.ToolDefinition` with a handler. Breaking change to the v0.1.0 surface, permissible pre-1.0.
- **Shell lifecycle**: `Shell.Close()` kills the underlying exec instance and drains streams; the container itself is untouched. A container can host multiple concurrent `Shell` instances, each owning its own `ExecSession`.
- **Dispatch contract**: the Obj #20 `Dispatch(ctx, c, name, args)` helper is stateless and concurrent-safe. Callers may invoke it from multiple goroutines against the same `*Container` without external synchronization beyond what the underlying tool handlers themselves require.
- **Release-vs-closeout sequencing**: the `v0.2.0` release tags ship after sub-issues 20A and 20B merge, but Phase 2 closeout is deferred until sub-issue 20C (which depends on the published tags) merges. Mirrors the Phase 1 pattern with `docker-hello`.
