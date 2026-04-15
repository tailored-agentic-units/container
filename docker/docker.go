package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

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

// Exec runs opts.Cmd inside the running container identified by id and
// returns its result. The container must be in the running state. Stdout
// and Stderr in the returned ExecResult are populated only when the
// corresponding AttachStdout/AttachStderr flag is set; unattached streams
// stay nil. AttachStdin is honored on the create call but receives EOF
// immediately because Phase 1 ExecOptions does not carry a stdin reader.
// Cancelling ctx closes the hijacked exec connection, terminating the
// process inside the container; the returned error wraps ctx.Err.
func (r *dockerRuntime) Exec(ctx context.Context, id string, opts container.ExecOptions) (*container.ExecResult, error) {
	create, err := r.cli.ContainerExecCreate(ctx, id, dc.ExecOptions{
		Cmd:          opts.Cmd,
		Env:          buildEnv(opts.Env),
		WorkingDir:   opts.WorkingDir,
		AttachStdin:  opts.AttachStdin,
		AttachStdout: opts.AttachStdout,
		AttachStderr: opts.AttachStderr,
	})
	if err != nil {
		return nil, fmt.Errorf("docker exec: create: %w", err)
	}

	hr, err := r.cli.ContainerExecAttach(ctx, create.ID, dc.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker exec: attach: %w", err)
	}
	defer hr.Close()

	if opts.AttachStdin {
		// Phase 1 has no Stdin reader; close the write side so the
		// process sees EOF on stdin instead of hanging.
		_ = hr.CloseWrite()
	}

	var stdout, stderr bytes.Buffer
	drainErr := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(&stdout, &stderr, hr.Reader)
		drainErr <- err
	}()

	select {
	case <-ctx.Done():
		// Closing the hijacked conn unblocks StdCopy; drain its error
		// so the goroutine doesn't leak, then surface ctx.Err.
		hr.Close()
		<-drainErr
		return nil, fmt.Errorf("docker exec: %w", ctx.Err())
	case err := <-drainErr:
		if err != nil {
			return nil, fmt.Errorf("docker exec: drain: %w", err)
		}
	}

	inspect, err := r.cli.ContainerExecInspect(ctx, create.ID)
	if err != nil {
		return nil, fmt.Errorf("docker exec: inspect: %w", err)
	}

	res := &container.ExecResult{ExitCode: inspect.ExitCode}
	if opts.AttachStdout {
		res.Stdout = stdout.Bytes()
	}
	if opts.AttachStderr {
		res.Stderr = stderr.Bytes()
	}
	return res, nil
}

// CopyTo writes content into the container at dst as a single regular file
// (mode 0644). Parent directories are created as needed via "mkdir -p"
// executed inside the container, so the container must be running and its
// image must provide a POSIX shell and mkdir. Cancelling ctx aborts the
// upload; partial state may remain on the container filesystem.
func (r *dockerRuntime) CopyTo(ctx context.Context, id string, dst string, content io.Reader) error {
	parent := path.Dir(dst)
	base := path.Base(dst)

	if parent != "" && parent != "/" && parent != "." {
		if _, err := r.Exec(ctx, id, container.ExecOptions{
			Cmd: []string{"mkdir", "-p", parent},
		}); err != nil {
			return fmt.Errorf("docker copy_to: mkdir parent: %w", err)
		}
	}

	body, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("docker copy_to: read source: %w", err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: base,
		Mode: 0o644,
		Size: int64(len(body)),
	}); err != nil {
		return fmt.Errorf("docker copy_to: tar header: %w", err)
	}
	if _, err := tw.Write(body); err != nil {
		return fmt.Errorf("docker copy_to: tar write: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("docker copy_to: tar close: %w", err)
	}

	if err := r.cli.CopyToContainer(ctx, id, parent, &buf, dc.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("docker copy_to: %w", err)
	}
	return nil
}

// CopyFrom returns a ReadCloser yielding the raw bytes of the file at src
// inside the container. The Docker API delivers the file as a tar archive;
// CopyFrom advances past the header so the caller never sees the tar
// wrapper. The caller MUST close the returned ReadCloser. Cancelling ctx
// aborts subsequent Read calls — the next Read returns ctx.Err — but
// closing is still required to release the underlying connection.
//
// When src does not exist in the container, the returned error wraps the
// Docker not-found error; callers can detect it via cerrdefs.IsNotFound
// (github.com/containerd/errdefs).
func (r *dockerRuntime) CopyFrom(ctx context.Context, id string, src string) (io.ReadCloser, error) {
	rc, _, err := r.cli.CopyFromContainer(ctx, id, src)
	if err != nil {
		return nil, fmt.Errorf("docker copy_from: %w", err)
	}
	tr := tar.NewReader(rc)
	if _, err := tr.Next(); err != nil {
		rc.Close()
		return nil, fmt.Errorf("docker copy_from: tar next: %w", err)
	}
	return &tarFileReader{ctx: ctx, tr: tr, rc: rc}, nil
}

// Inspect returns the full ContainerInfo view for the container identified
// by id. State is normalized via mapState; Docker's "paused" status is
// rejected explicitly because the Phase 1 state set excludes Paused. The
// manifest at container.ManifestPath is read via CopyFrom and parsed via
// container.Parse: a missing file leaves Manifest nil with no error
// (callers needing a non-nil value substitute container.Fallback);
// malformed or version-mismatched manifests surface as errors wrapping
// container.ErrManifestInvalid or container.ErrManifestVersion. Cancelling
// ctx aborts both the Docker inspect call and the manifest read.
func (r *dockerRuntime) Inspect(ctx context.Context, id string) (*container.ContainerInfo, error) {
	raw, err := r.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}

	state, err := mapState(raw.State)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}

	info := &container.ContainerInfo{
		ID:     raw.ID,
		Name:   strings.TrimPrefix(raw.Name, "/"),
		Image:  raw.Config.Image,
		State:  state,
		Labels: raw.Config.Labels,
	}

	rc, err := r.CopyFrom(ctx, id, container.ManifestPath)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return info, nil
		}
		return nil, fmt.Errorf("docker inspect: manifest read: %w", err)
	}
	defer rc.Close()

	manifest, err := container.Parse(rc)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}
	info.Manifest = manifest

	return info, nil
}

type tarFileReader struct {
	ctx context.Context
	tr  *tar.Reader
	rc  io.ReadCloser
}

func (t *tarFileReader) Read(p []byte) (int, error) {
	if err := t.ctx.Err(); err != nil {
		return 0, err
	}
	return t.tr.Read(p)
}

func (t *tarFileReader) Close() error { return t.rc.Close() }

func buildEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func mapState(s *dc.State) (container.State, error) {
	if s == nil {
		return "", fmt.Errorf("nil container state")
	}

	switch s.Status {
	case "created":
		return container.StateCreated, nil
	case "running", "restarting":
		return container.StateRunning, nil
	case "exited", "dead":
		return container.StateExited, nil
	case "removing":
		return container.StateRemoved, nil
	case "paused":
		return "", fmt.Errorf("paused state not supported in Phase 1")
	default:
		return "", fmt.Errorf("unknown docker state %q", s.Status)
	}
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
