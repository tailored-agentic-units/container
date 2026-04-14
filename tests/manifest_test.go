package container_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/tailored-agentic-units/container"
)

func fixture(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatalf("open fixture %q: %v", name, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestParse_Full(t *testing.T) {
	m, err := container.Parse(fixture(t, "manifest_full.json"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if m.Version != container.ManifestVersion {
		t.Errorf("Version = %q, want %q", m.Version, container.ManifestVersion)
	}
	if m.Name != "tau-go-dev" {
		t.Errorf("Name = %q, want %q", m.Name, "tau-go-dev")
	}
	if m.Shell != "/bin/bash" {
		t.Errorf("Shell = %q, want %q", m.Shell, "/bin/bash")
	}
	if m.Workspace != "/workspace" {
		t.Errorf("Workspace = %q, want %q", m.Workspace, "/workspace")
	}

	goTool, ok := m.Tools["go"]
	if !ok {
		t.Fatal("Tools[\"go\"] missing")
	}
	if goTool.Version != "1.26" {
		t.Errorf("Tools[\"go\"].Version = %q, want %q", goTool.Version, "1.26")
	}

	pg, ok := m.Services["postgres"]
	if !ok {
		t.Fatal("Services[\"postgres\"] missing")
	}
	if pg.Description == "" {
		t.Error("Services[\"postgres\"].Description is empty")
	}

	if m.Env["GOPATH"] != "/go" {
		t.Errorf("Env[\"GOPATH\"] = %q, want %q", m.Env["GOPATH"], "/go")
	}

	retries, ok := m.Options["retries"]
	if !ok {
		t.Fatal("Options[\"retries\"] missing")
	}
	// JSON numbers decode into float64 when targeting map[string]any.
	if got, ok := retries.(float64); !ok || got != 3 {
		t.Errorf("Options[\"retries\"] = %v (%T), want float64(3)", retries, retries)
	}
}

func TestParse_Minimal(t *testing.T) {
	m, err := container.Parse(fixture(t, "manifest_minimal.json"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if m.Version != container.ManifestVersion {
		t.Errorf("Version = %q, want %q", m.Version, container.ManifestVersion)
	}
	if m.Name != "minimal" {
		t.Errorf("Name = %q, want %q", m.Name, "minimal")
	}
	if m.Shell != "/bin/sh" {
		t.Errorf("Shell = %q, want %q", m.Shell, "/bin/sh")
	}
	if m.Tools != nil {
		t.Errorf("Tools = %v, want nil", m.Tools)
	}
	if m.Services != nil {
		t.Errorf("Services = %v, want nil", m.Services)
	}
	if m.Options != nil {
		t.Errorf("Options = %v, want nil", m.Options)
	}
}

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantSentinel error
		wantMessage  string
	}{
		{
			name:        "bad version",
			fixture:     "manifest_bad_version.json",
			wantSentinel: container.ErrManifestVersion,
			wantMessage:  `"2"`,
		},
		{
			name:        "missing name",
			fixture:     "manifest_missing_name.json",
			wantSentinel: container.ErrManifestInvalid,
			wantMessage:  "name",
		},
		{
			name:        "missing shell",
			fixture:     "manifest_missing_shell.json",
			wantSentinel: container.ErrManifestInvalid,
			wantMessage:  "shell",
		},
		{
			name:        "malformed JSON",
			fixture:     "manifest_malformed.json",
			wantSentinel: container.ErrManifestInvalid,
		},
		{
			name:        "unknown top-level field",
			fixture:     "manifest_unknown_field.json",
			wantSentinel: container.ErrManifestInvalid,
			wantMessage:  "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := container.Parse(fixture(t, tt.fixture))
			if err == nil {
				t.Fatalf("Parse returned nil error; got manifest %+v", m)
			}
			if m != nil {
				t.Errorf("Parse returned non-nil manifest on error: %+v", m)
			}
			if !errors.Is(err, tt.wantSentinel) {
				t.Errorf("errors.Is(err, %v) = false; err = %v", tt.wantSentinel, err)
			}
			if tt.wantMessage != "" && !strings.Contains(err.Error(), tt.wantMessage) {
				t.Errorf("err message %q does not contain %q", err.Error(), tt.wantMessage)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		manifest    *container.Manifest
		wantSentinel error
	}{
		{
			name:        "nil",
			manifest:    nil,
			wantSentinel: container.ErrManifestInvalid,
		},
		{
			name: "wrong version",
			manifest: &container.Manifest{
				Version: "99",
				Name:    "x",
				Shell:   "/bin/sh",
			},
			wantSentinel: container.ErrManifestVersion,
		},
		{
			name: "empty version",
			manifest: &container.Manifest{
				Name:  "x",
				Shell: "/bin/sh",
			},
			wantSentinel: container.ErrManifestVersion,
		},
		{
			name: "missing name",
			manifest: &container.Manifest{
				Version: container.ManifestVersion,
				Shell:   "/bin/sh",
			},
			wantSentinel: container.ErrManifestInvalid,
		},
		{
			name: "missing shell",
			manifest: &container.Manifest{
				Version: container.ManifestVersion,
				Name:    "x",
			},
			wantSentinel: container.ErrManifestInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := container.Validate(tt.manifest)
			if err == nil {
				t.Fatal("Validate returned nil error")
			}
			if !errors.Is(err, tt.wantSentinel) {
				t.Errorf("errors.Is(err, %v) = false; err = %v", tt.wantSentinel, err)
			}
		})
	}
}

func TestValidate_Valid(t *testing.T) {
	m := &container.Manifest{
		Version: container.ManifestVersion,
		Name:    "x",
		Shell:   "/bin/sh",
	}
	if err := container.Validate(m); err != nil {
		t.Errorf("Validate returned error on valid manifest: %v", err)
	}
}

func TestFallback_RoundTrip(t *testing.T) {
	m := container.Fallback()
	if m == nil {
		t.Fatal("Fallback returned nil")
	}
	if err := container.Validate(m); err != nil {
		t.Errorf("Fallback manifest failed Validate: %v", err)
	}
	if m.Version != container.ManifestVersion {
		t.Errorf("Fallback Version = %q, want %q", m.Version, container.ManifestVersion)
	}
	if m.Name == "" {
		t.Error("Fallback Name is empty")
	}
	if m.Shell == "" {
		t.Error("Fallback Shell is empty")
	}
}

func TestManifestPath_Constant(t *testing.T) {
	if container.ManifestPath != "/etc/tau/manifest.json" {
		t.Errorf("ManifestPath = %q, want %q", container.ManifestPath, "/etc/tau/manifest.json")
	}
}
