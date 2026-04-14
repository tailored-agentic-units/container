package container

import (
	"context"
	"io"
	"time"
)

// Runtime is the OCI-aligned contract every container runtime implementation
// must satisfy. Implementations live in sub-modules (for example
// github.com/tailored-agentic-units/container/docker) and are constructed
// through the package-level registry; root-module callers depend on this
// interface, never on a concrete runtime.
//
// Per the Phase 1 cross-cutting decision, every method takes a
// context.Context. Cancellation semantics differ by method and are documented
// per-method below; implementations MUST honor them so callers can rely on
// uniform cancellation behavior across runtimes.
type Runtime interface {
	// Create provisions a new container from opts and returns a Container
	// handle carrying the runtime-assigned ID. Cancelling ctx aborts the
	// in-flight Create API call; if the runtime had already started
	// provisioning, partial state may remain and the caller is responsible
	// for cleanup via Remove.
	Create(ctx context.Context, opts CreateOptions) (*Container, error)

	// Start transitions a created container to the running state.
	// Cancelling ctx aborts the in-flight Start API call; the container
	// may still transition to running on the runtime side, so callers
	// should Inspect to reconcile state.
	Start(ctx context.Context, id string) error

	// Stop requests a graceful stop, escalating to a kill if the container
	// has not exited within timeout. Stop honors timeout independently of
	// ctx; cancelling ctx only aborts the API call that initiates the stop
	// request, not the timeout countdown the runtime applies.
	Stop(ctx context.Context, id string, timeout time.Duration) error

	// Remove deletes the container from the runtime. When force is true the
	// runtime kills the container if it is still running; when false,
	// removing a running container returns an error that wraps
	// ErrInvalidState. Cancelling ctx aborts the in-flight Remove API call.
	Remove(ctx context.Context, id string, force bool) error

	// Exec runs a one-shot command inside an already-running container and
	// returns its ExecResult. Cancelling ctx kills the exec instance inside
	// the container; the returned error wraps ctx.Err() in that case.
	// Callers needing a persistent interactive session should use the
	// Phase 2 shell wrapper rather than repeatedly invoking Exec.
	Exec(ctx context.Context, id string, opts ExecOptions) (*ExecResult, error)

	// CopyTo writes content into the container at dst, creating parent
	// directories as needed. Cancelling ctx aborts the copy stream; partial
	// writes may remain on the container filesystem.
	CopyTo(ctx context.Context, id string, dst string, content io.Reader) error

	// CopyFrom reads the file at src out of the container and returns a
	// stream the caller MUST close. Cancelling ctx aborts the copy stream;
	// closing the returned ReadCloser is still required.
	CopyFrom(ctx context.Context, id string, src string) (io.ReadCloser, error)

	// Inspect returns the full ContainerInfo view of the container.
	// Cancelling ctx aborts the in-flight Inspect API call. Inspect
	// implementations that populate Manifest pay a CopyFrom round-trip per
	// call; callers needing repeat access should cache the result.
	Inspect(ctx context.Context, id string) (*ContainerInfo, error)
}
