package docker

import (
	"context"
	"fmt"
	"io"
	"maps"
	"time"

	"github.com/docker/docker/client"

	"github.com/tailored-agentic-units/container"

	cerrdefs "github.com/containerd/errdefs"
	dc "github.com/docker/docker/api/types/container"
)

// Label keys applied by Create to every tau-managed container. The tau.*
// namespace is reserved; caller-supplied labels sharing these keys are
// ignored. The value of LabelManifestVersion is sourced from
// container.ManifestVersion so a future schema bump flows through
// automatically without touching this sub-module.
const (
	LabelManaged         = "tau.managed"
	LabelManifestVersion = "tau.manifest.version"
)

type dockerRuntime struct {
	cli *client.Client
}

// Register wires the default Docker factory into the root container
// registry under the name "docker". Call Register once from application
// code before invoking container.Create("docker") — per project
// convention, Register is never called from package init. The default
// factory builds a client from the host environment via client.FromEnv
// and negotiates the Docker API version with the daemon.
func Register() {
	container.Register("docker", func() (container.Runtime, error) {
		cli, err := client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)

		if err != nil {
			return nil, fmt.Errorf("docker: new client: %w", err)
		}
		return &dockerRuntime{cli: cli}, nil
	})
}

// Create provisions a new Docker container from opts. The reserved labels
// LabelManaged and LabelManifestVersion are merged into opts.Labels and win
// on collision. The returned Container reports StateCreated; callers must
// invoke Start before the container runs.
func (r *dockerRuntime) Create(ctx context.Context, opts container.CreateOptions) (*container.Container, error) {
	cfg := &dc.Config{
		Image:      opts.Image,
		Cmd:        opts.Cmd,
		Env:        buildEnv(opts.Env),
		WorkingDir: opts.WorkingDir,
		Labels:     mergeLabels(opts.Labels),
	}

	resp, err := r.cli.ContainerCreate(
		ctx, cfg,
		nil, nil, nil,
		opts.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("docker create: %w", err)
	}

	return &container.Container{
		ID:     resp.ID,
		Name:   opts.Name,
		Image:  opts.Image,
		State:  container.StateCreated,
		Labels: cfg.Labels,
	}, nil
}

// Start transitions the container identified by id to the running state.
func (r *dockerRuntime) Start(ctx context.Context, id string) error {
	if err := r.cli.ContainerStart(ctx, id, dc.StartOptions{}); err != nil {
		return fmt.Errorf("docker start: %w", err)
	}
	return nil
}

// Stop requests a graceful stop and lets the Docker daemon escalate to a
// kill after timeout elapses. The daemon enforces timeout independently of
// ctx — cancelling ctx only aborts the API call that initiates the stop,
// not the timeout countdown. A non-positive timeout passes nil to Docker,
// which applies the daemon default.
func (r *dockerRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	var secs *int
	if timeout > 0 {
		s := int(timeout.Seconds())
		secs = &s
	}
	if err := r.cli.ContainerStop(ctx, id, dc.StopOptions{Timeout: secs}); err != nil {
		return fmt.Errorf("docker stop: %w", err)
	}
	return nil
}

// Remove deletes the container identified by id. When force is false,
// Remove first inspects the container and returns an error wrapping
// container.ErrInvalidState if the container is running; any conflict
// returned by the daemon is likewise remapped to ErrInvalidState so callers
// can branch on errors.Is regardless of which check fires.
func (r *dockerRuntime) Remove(ctx context.Context, id string, force bool) error {
	if !force {
		info, err := r.cli.ContainerInspect(ctx, id)
		if err != nil {
			return fmt.Errorf("docker remove: inspect: %w", err)
		}
		if info.State != nil && info.State.Running {
			return fmt.Errorf("docker remove: %w: container %s is running", container.ErrInvalidState, id)
		}
	}
	if err := r.cli.ContainerRemove(ctx, id, dc.RemoveOptions{Force: force}); err != nil {
		if cerrdefs.IsConflict(err) {
			return fmt.Errorf("docker remove: %w: %v", container.ErrInvalidState, err)
		}
		return fmt.Errorf("docker remove: %w", err)
	}
	return nil
}

func (r *dockerRuntime) Exec(ctx context.Context, id string, opts container.ExecOptions) (*container.ExecResult, error) {
	return nil, fmt.Errorf("docker exec: not implemented in sub-issue #11")
}

func (r *dockerRuntime) CopyTo(ctx context.Context, id string, dst string, content io.Reader) error {
	return fmt.Errorf("docker copy_to: not implemented in sub-issue #11")
}

func (r *dockerRuntime) CopyFrom(ctx context.Context, id string, src string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("docker copy_from: not implemented in sub-issue #11")
}

func (r *dockerRuntime) Inspect(ctx context.Context, id string) (*container.ContainerInfo, error) {
	return nil, fmt.Errorf("docker inspect: not implemented in sub-issue #11")
}

func buildEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func mergeLabels(caller map[string]string) map[string]string {
	out := maps.Clone(caller)
	if out == nil {
		out = make(map[string]string, 2)
	}
	out[LabelManaged] = "true"
	out[LabelManifestVersion] = container.ManifestVersion
	return out
}
