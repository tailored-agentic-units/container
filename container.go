// Package container defines the OCI-aligned runtime abstraction and core data
// model for tau-managed containers. The package exposes runtime-agnostic types
// and interfaces; concrete runtime implementations (Docker, containerd) live in
// sub-modules that register themselves via the package-level registry.
package container

// State is a container's lifecycle state. Values are OCI-aligned strings; the
// Phase 1 state set excludes "paused" because the Runtime interface does not
// define a Pause operation.
type State string

// Container lifecycle states. Runtimes normalize their native state model to
// these values when populating Container and ContainerInfo.
const (
	// StateCreated indicates the container has been created but not started.
	StateCreated State = "created"
	// StateRunning indicates the container is currently running.
	StateRunning State = "running"
	// StateExited indicates the container has stopped after running.
	StateExited State = "exited"
	// StateRemoved indicates the container has been removed from the runtime.
	StateRemoved State = "removed"
)

// Container is the handle returned by Runtime.Create. It carries the identity
// and initial state of a container; callers use ID for subsequent Runtime
// method calls. For the full inspect-time view of a container, see
// ContainerInfo.
type Container struct {
	// ID is the runtime-assigned unique identifier.
	ID string
	// Name is the human-readable container name.
	Name string
	// Image is the image reference the container was created from.
	Image string
	// State is the container's current lifecycle state.
	State State
	// Labels are the key/value labels attached to the container. Tau-managed
	// containers carry tau.managed=true and additional labels under the tau.*
	// namespace.
	Labels map[string]string
}

// CreateOptions carries the parameters for Runtime.Create. Callers constructing
// tau-managed containers SHOULD set the tau.managed=true label so downstream
// listing and filtering can identify managed instances without matching on
// image names.
type CreateOptions struct {
	// Image is the image reference to create the container from.
	Image string
	// Name is an optional human-readable name for the container. Empty leaves
	// naming to the runtime.
	Name string
	// Cmd overrides the image's default command. Empty uses the image default.
	Cmd []string
	// Env is the environment variables to set in the container.
	Env map[string]string
	// WorkingDir overrides the image's working directory. Empty uses the image
	// default.
	WorkingDir string
	// Labels are the labels to attach to the container. The tau.* namespace is
	// reserved for tau-managed metadata.
	Labels map[string]string
}

// ExecOptions carries the parameters for Runtime.Exec. Exec runs a one-shot
// command inside an already-running container. Callers that need a persistent
// interactive session should use the Phase 2 shell wrapper rather than
// repeatedly invoking Exec.
type ExecOptions struct {
	// Cmd is the command and arguments to execute. Required.
	Cmd []string
	// Env is the environment variables to set for the exec'd process. These
	// merge with the container's environment.
	Env map[string]string
	// WorkingDir sets the working directory for the exec'd process. Empty uses
	// the container's working directory.
	WorkingDir string
	// AttachStdin attaches the caller's stdin to the exec'd process.
	AttachStdin bool
	// AttachStdout captures stdout into ExecResult.Stdout.
	AttachStdout bool
	// AttachStderr captures stderr into ExecResult.Stderr.
	AttachStderr bool
}

// ExecResult captures the outcome of a one-shot Runtime.Exec.
type ExecResult struct {
	// ExitCode is the exit status of the exec'd process.
	ExitCode int
	// Stdout contains captured standard output when ExecOptions.AttachStdout is
	// set. Nil otherwise.
	Stdout []byte
	// Stderr contains captured standard error when ExecOptions.AttachStderr is
	// set. Nil otherwise.
	Stderr []byte
}

// ContainerInfo is the full Runtime.Inspect response for a container. It is a
// superset of Container's shape and will grow additional fields in later
// phases (timestamps, exit codes, network settings).
type ContainerInfo struct {
	// ID is the runtime-assigned unique identifier.
	ID string
	// Name is the human-readable container name.
	Name string
	// Image is the image reference the container was created from.
	Image string
	// State is the container's current lifecycle state.
	State State
	// Labels are the key/value labels attached to the container.
	Labels map[string]string
	// Manifest is the image capability manifest read from ManifestPath at
	// inspect time. Nil when the image has no manifest; callers needing a
	// non-nil value should substitute Fallback. Populating this field costs
	// a CopyFrom round-trip per Inspect, so callers that need repeat access
	// should cache the ContainerInfo.
	Manifest *Manifest
}
