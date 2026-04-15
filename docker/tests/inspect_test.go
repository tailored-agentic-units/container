package docker_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tailored-agentic-units/container"
	"github.com/tailored-agentic-units/container/docker"
)

func TestInspect_VanillaAlpine_NilManifest(t *testing.T) {
	rt, _ := newRuntime(t)
	c := createSleeper(t, rt, nil)

	info, err := rt.Inspect(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.ID != c.ID {
		t.Errorf("ID: got %q, want %q", info.ID, c.ID)
	}
	if info.Image != testImage {
		t.Errorf("Image: got %q, want %q", info.Image, testImage)
	}
	if info.State != container.StateCreated {
		t.Errorf("State: got %q, want %q", info.State, container.StateCreated)
	}
	if got := info.Labels[docker.LabelManaged]; got != "true" {
		t.Errorf("LabelManaged: got %q, want %q", got, "true")
	}
	if got := info.Labels[docker.LabelManifestVersion]; got != container.ManifestVersion {
		t.Errorf("LabelManifestVersion: got %q, want %q", got, container.ManifestVersion)
	}
	if info.Manifest != nil {
		t.Errorf("Manifest: got %+v, want nil", info.Manifest)
	}
	if strings.HasPrefix(info.Name, "/") {
		t.Errorf("Name has leading slash: %q", info.Name)
	}
}

func TestInspect_RunningState(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	info, err := rt.Inspect(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.State != container.StateRunning {
		t.Errorf("State: got %q, want %q", info.State, container.StateRunning)
	}
}

func TestInspect_WellFormedManifest(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	const body = `{"version":"1","name":"tau-test","shell":"/bin/sh","workspace":"/workspace"}`
	if err := rt.CopyTo(context.Background(), c.ID, container.ManifestPath, strings.NewReader(body)); err != nil {
		t.Fatalf("CopyTo manifest: %v", err)
	}

	info, err := rt.Inspect(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Manifest == nil {
		t.Fatal("Manifest: got nil, want populated")
	}
	if info.Manifest.Name != "tau-test" {
		t.Errorf("Manifest.Name: got %q, want %q", info.Manifest.Name, "tau-test")
	}
	if info.Manifest.Shell != "/bin/sh" {
		t.Errorf("Manifest.Shell: got %q, want %q", info.Manifest.Shell, "/bin/sh")
	}
	if info.Manifest.Workspace != "/workspace" {
		t.Errorf("Manifest.Workspace: got %q, want %q", info.Manifest.Workspace, "/workspace")
	}
}

func TestInspect_MalformedManifest(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	if err := rt.CopyTo(context.Background(), c.ID, container.ManifestPath, strings.NewReader("not json")); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	_, err := rt.Inspect(context.Background(), c.ID)
	if err == nil {
		t.Fatal("Inspect malformed manifest: expected error, got nil")
	}
	if !errors.Is(err, container.ErrManifestInvalid) {
		t.Errorf("err is not ErrManifestInvalid: %v", err)
	}
}

func TestInspect_VersionMismatchManifest(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	const body = `{"version":"99","name":"tau-test","shell":"/bin/sh"}`
	if err := rt.CopyTo(context.Background(), c.ID, container.ManifestPath, strings.NewReader(body)); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	_, err := rt.Inspect(context.Background(), c.ID)
	if err == nil {
		t.Fatal("Inspect version-mismatched manifest: expected error, got nil")
	}
	if !errors.Is(err, container.ErrManifestVersion) {
		t.Errorf("err is not ErrManifestVersion: %v", err)
	}
}

func TestInspect_ExitedState(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)
	if err := rt.Stop(context.Background(), c.ID, 2*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	info, err := rt.Inspect(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.State != container.StateExited {
		t.Errorf("State: got %q, want %q", info.State, container.StateExited)
	}
}

func TestInspect_PausedState_ReturnsError(t *testing.T) {
	rt, cli := newRuntime(t)
	c := startSleeper(t, rt)
	if err := cli.ContainerPause(context.Background(), c.ID); err != nil {
		t.Skipf("ContainerPause unsupported on this host: %v", err)
	}
	t.Cleanup(func() {
		_ = cli.ContainerUnpause(context.Background(), c.ID)
	})

	_, err := rt.Inspect(context.Background(), c.ID)
	if err == nil {
		t.Fatal("Inspect of paused container: expected error, got nil")
	}
}

func TestInspect_CtxCancel(t *testing.T) {
	rt, _ := newRuntime(t)
	c := createSleeper(t, rt, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.Inspect(ctx, c.ID)
	if err == nil {
		t.Fatal("Inspect with cancelled ctx: expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err is not context.Canceled: %v", err)
	}
}
