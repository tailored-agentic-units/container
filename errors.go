package container

import "errors"

// Domain errors returned by package container and its runtime sub-modules.
// Callers use errors.Is to distinguish the semantic cause; sub-modules wrap
// these with operation context via fmt.Errorf("docker create: %w", err).
var (
	// ErrRuntimeNotFound is returned when a runtime name is requested from the
	// registry but no factory has been registered under that name. Callers
	// that look up runtimes dynamically should check for this error via
	// errors.Is.
	ErrRuntimeNotFound = errors.New("container: runtime not found")

	// ErrContainerNotFound is returned by Runtime methods when the supplied
	// container ID does not correspond to a known container.
	ErrContainerNotFound = errors.New("container: container not found")

	// ErrInvalidState is returned when a Runtime operation is invalid for the
	// container's current lifecycle state (for example, starting a container
	// that is already running).
	ErrInvalidState = errors.New("container: invalid state")
)
