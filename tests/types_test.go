package container_test

import (
	"errors"
	"testing"

	"github.com/tailored-agentic-units/container"
)

func TestState_Constants(t *testing.T) {
	tests := []struct {
		name  string
		state container.State
		want  string
	}{
		{"created", container.StateCreated, "created"},
		{"running", container.StateRunning, "running"},
		{"exited", container.StateExited, "exited"},
		{"removed", container.StateRemoved, "removed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.state) != tt.want {
				t.Errorf("got %q, want %q", string(tt.state), tt.want)
			}
		})
	}
}

func TestState_DistinctValues(t *testing.T) {
	all := []container.State{
		container.StateCreated,
		container.StateRunning,
		container.StateExited,
		container.StateRemoved,
	}

	seen := make(map[container.State]bool, len(all))
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate State value: %q", s)
		}
		seen[s] = true
	}
}

func TestDomainErrors_Identity(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrRuntimeNotFound", container.ErrRuntimeNotFound},
		{"ErrContainerNotFound", container.ErrContainerNotFound},
		{"ErrInvalidState", container.ErrInvalidState},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("sentinel is nil")
			}
			if tt.err.Error() == "" {
				t.Error("sentinel has empty message")
			}
			if !errors.Is(tt.err, tt.err) {
				t.Error("errors.Is(err, err) returned false")
			}
		})
	}
}

func TestDomainErrors_Distinct(t *testing.T) {
	sentinels := []error{
		container.ErrRuntimeNotFound,
		container.ErrContainerNotFound,
		container.ErrInvalidState,
	}

	for i, a := range sentinels {
		for j, b := range sentinels {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("sentinels %d and %d should be distinct", i, j)
			}
		}
	}
}
