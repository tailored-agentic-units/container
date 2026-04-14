package container

import (
	"encoding/json"
	"fmt"
	"io"
)

// ManifestVersion is the only manifest schema version accepted by Parse and
// Validate in Phase 1. Future schema revisions bump this value; images built
// against an older schema are rejected with ErrManifestVersion so callers can
// surface an explicit upgrade path.
const ManifestVersion = "1"

// ManifestPath is the well-known location of the image capability manifest
// inside a tau-managed container. Runtimes read this path via
// Runtime.CopyFrom during Inspect and feed the bytes to Parse; exporting the
// constant keeps the path single-sourced across runtime implementations.
const ManifestPath = "/etc/tau/manifest.json"

// Service describes an external service that a container image declares as
// available to the agent. Phase 1 carries only a human-readable description;
// structured connection details (endpoint, credentials, auth mode) are
// deferred to a later phase once concrete integrations drive the shape.
type Service struct {
	Description string `json:"description,omitempty"`
}

// Tool describes a CLI tool installed in a container image. The fields are
// deliberately minimal — version for compatibility checks, description for
// system-prompt context — and intentionally do not describe invocation
// semantics. Tool invocation flows through the Phase 2 shell wrapper, not
// through manifest-declared signatures.
type Tool struct {
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
}

// Manifest is the image capability manifest read from /etc/tau/manifest.json.
// Images declare the shell, workspace, environment, tools, and services they
// expose to agents; the manifest is parsed at inspect time and surfaced via
// ContainerInfo.Manifest.
//
// The schema is strict: Parse rejects any top-level field not defined here.
// Runtime-specific configuration that tau itself does not interpret belongs
// under Options, which is the single sanctioned pass-through slot.
type Manifest struct {
	// Version is the manifest schema version. Must equal ManifestVersion for
	// Validate to pass.
	Version string `json:"version"`

	// Name is a short identifier for the image (e.g., "tau-go-dev").
	// Required.
	Name string `json:"name"`

	// Description is a human-readable summary of the image's purpose.
	Description string `json:"description,omitempty"`

	// Base identifies the base image the manifest was built on (e.g.,
	// "alpine:3.21"). Informational only; not used by Validate.
	Base string `json:"base,omitempty"`

	// Shell is the absolute path to the shell binary the agent should use
	// for Runtime.Exec and the Phase 2 persistent shell. Required.
	Shell string `json:"shell"`

	// Workspace is the working directory agents should default to when
	// entering the container. Runtimes may use it as the default WorkingDir
	// for Exec.
	Workspace string `json:"workspace,omitempty"`

	// Env declares environment variables baked into the image. Runtimes
	// merge these with CreateOptions.Env at container creation time.
	Env map[string]string `json:"env,omitempty"`

	// Tools maps CLI tool names to their version and description metadata.
	// Agent system prompts use these to understand what is callable through
	// the shell.
	Tools map[string]Tool `json:"tools,omitempty"`

	// Services maps external service names to their description. Declaring
	// a service here does not configure connectivity; it signals intent and
	// informs the agent what integrations the image was built for.
	Services map[string]Service `json:"services,omitempty"`

	// Options is the controlled pass-through slot for runtime- or
	// image-specific configuration that tau itself does not interpret.
	// Contents are opaque to Parse and Validate; consumers that need typed
	// access should assert their own schema against this map. Mirrors the
	// provider/model Options convention at tau/protocol/config.
	Options map[string]any `json:"options,omitempty"`
}

// Parse decodes a manifest from r using strict JSON semantics: unknown
// top-level fields cause a decode error wrapping ErrManifestInvalid. On
// successful decode the manifest is validated via Validate, whose error is
// returned unwrapped (already sentinel-tagged). Parse returns a non-nil
// manifest only when both decode and validation succeed.
func Parse(r io.Reader) (*Manifest, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}

	if err := Validate(&m); err != nil {
		return nil, err
	}

	return &m, nil
}

// Validate enforces the minimum contract the rest of the package depends on:
// the manifest is non-nil, Version matches ManifestVersion, Name is
// non-empty, and Shell is non-empty. Version is checked before name/shell so
// a mismatched-version manifest surfaces ErrManifestVersion rather than
// whichever required field is absent. Other fields, including Options
// contents, are not validated.
func Validate(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("%w: nil manifest", ErrManifestInvalid)
	}
	if m.Version != ManifestVersion {
		return fmt.Errorf(
			"%w: got %q, want %q",
			ErrManifestVersion,
			m.Version,
			ManifestVersion,
		)
	}
	if m.Name == "" {
		return fmt.Errorf("%w: missing required field: name", ErrManifestInvalid)
	}
	if m.Shell == "" {
		return fmt.Errorf("%w: missing required field: shell", ErrManifestInvalid)
	}
	return nil
}

// Fallback returns a non-nil manifest with POSIX-shell defaults, suitable
// for images that do not carry /etc/tau/manifest.json. The returned manifest
// passes Validate, so callers can substitute it directly for a missing-file
// case without additional checks. Intended for use by Runtime.Inspect
// implementations when CopyFrom reports the manifest path is absent.
func Fallback() *Manifest {
	return &Manifest{
		Version: ManifestVersion,
		Name:    "fallback",
		Shell:   "/bin/sh",
	}
}
