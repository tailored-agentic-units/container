package docker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	dcli "github.com/docker/docker/client"

	"github.com/tailored-agentic-units/container"
	"github.com/tailored-agentic-units/container/docker"
)

func newRuntime(t *testing.T) (container.Runtime, *dcli.Client) {
	t.Helper()
	cli := skipIfNoDaemon(t)
	ensureImage(t, cli, testImage)

	docker.Register()
	rt, err := container.Create("docker")
	if err != nil {
		t.Fatalf("create docker runtime: %v", err)
	}
	return rt, cli
}

func createSleeper(t *testing.T, rt container.Runtime, extraLabels map[string]string) *container.Container {
	t.Helper()
	c, err := rt.Create(context.Background(), container.CreateOptions{
		Image:  testImage,
		Cmd:    []string{"sleep", "30"},
		Labels: extraLabels,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_ = rt.Remove(context.Background(), c.ID, true)
	})
	return c
}

func TestLifecycle_RoundTrip(t *testing.T) {
	rt, cli := newRuntime(t)
	c := createSleeper(t, rt, nil)

	inspect, err := cli.ContainerInspect(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("inspect after create: %v", err)
	}
	if inspect.State.Status != "created" {
		t.Errorf("state after Create: got %q, want %q", inspect.State.Status, "created")
	}

	if err := rt.Start(context.Background(), c.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	inspect, err = cli.ContainerInspect(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("inspect after start: %v", err)
	}
	if inspect.State.Status != "running" {
		t.Errorf("state after Start: got %q, want %q", inspect.State.Status, "running")
	}

	if err := rt.Stop(context.Background(), c.ID, 2*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := rt.Remove(context.Background(), c.ID, false); err != nil {
		t.Fatalf("Remove(force=false) after Stop: %v", err)
	}

	if _, err := cli.ContainerInspect(context.Background(), c.ID); err == nil {
		t.Errorf("inspect after Remove: expected error, got nil")
	}
}

func TestLifecycle_LabelsApplied(t *testing.T) {
	rt, cli := newRuntime(t)
	c := createSleeper(t, rt, map[string]string{"my.label": "x"})

	inspect, err := cli.ContainerInspect(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	want := map[string]string{
		docker.LabelManaged:         "true",
		docker.LabelManifestVersion: container.ManifestVersion,
		"my.label":                  "x",
	}
	for k, v := range want {
		if got := inspect.Config.Labels[k]; got != v {
			t.Errorf("label %q: got %q, want %q", k, got, v)
		}
	}
}

func TestLifecycle_ReservedLabelsCannotBeOverridden(t *testing.T) {
	rt, cli := newRuntime(t)
	c := createSleeper(t, rt, map[string]string{docker.LabelManaged: "false"})

	inspect, err := cli.ContainerInspect(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if got := inspect.Config.Labels[docker.LabelManaged]; got != "true" {
		t.Errorf("reserved label overridden: got %q, want %q", got, "true")
	}
}

func TestRemove_ForceFalse_RunningContainer(t *testing.T) {
	rt, _ := newRuntime(t)
	c := createSleeper(t, rt, nil)
	if err := rt.Start(context.Background(), c.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	err := rt.Remove(context.Background(), c.ID, false)
	if err == nil {
		t.Fatal("Remove(force=false) on running container: expected error, got nil")
	}
	if !errors.Is(err, container.ErrInvalidState) {
		t.Errorf("err is not ErrInvalidState: %v", err)
	}
}

func TestRemove_ForceTrue_RunningContainer(t *testing.T) {
	rt, cli := newRuntime(t)
	c := createSleeper(t, rt, nil)
	if err := rt.Start(context.Background(), c.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := rt.Remove(context.Background(), c.ID, true); err != nil {
		t.Fatalf("Remove(force=true) on running container: %v", err)
	}
	if _, err := cli.ContainerInspect(context.Background(), c.ID); err == nil {
		t.Error("container still exists after force Remove")
	}
}
