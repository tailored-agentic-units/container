# 12 - Docker runtime Exec and file copy methods

## Problem Context

Sub-issue B of [Objective #3](https://github.com/tailored-agentic-units/container/issues/3). Sub-issue #11 landed the docker sub-module scaffold and the four lifecycle methods (Create / Start / Stop / Remove). The remaining stubs in `docker/docker.go` — `Exec`, `CopyTo`, `CopyFrom` — return "not implemented in sub-issue #11" errors. This issue replaces those three stubs with real implementations.

`Inspect` (and the manifest read it performs) stays out of scope and ships as sub-issue C, which depends on the `CopyFrom` written here.

## Architecture Approach

Three streaming APIs, each with a slightly different cancellation seam:

- **Exec** uses Docker's hijacked-connection exec flow (`ContainerExecCreate` → `ContainerExecAttach` → drain via `stdcopy.StdCopy` → `ContainerExecInspect`). Docker SDK v28.5.2 exposes no `ContainerExecKill`; cancelling an in-flight exec is done by closing the hijacked connection, which terminates the attached process. We launch the demux drain in a goroutine and `select` on `ctx.Done()` against the goroutine's done channel.

- **CopyTo** wraps `CopyToContainer`, which requires the destination's parent directory to exist and a tar stream as the body. We `mkdir -p` the parent via the freshly-written `Exec` method (no helper duplication), then build a single-entry in-memory tar from the caller's `io.Reader`. This means CopyTo has a precondition: the container must be running and have a POSIX shell + `mkdir`. Document the precondition on the method.

- **CopyFrom** wraps `CopyFromContainer`, which returns `(io.ReadCloser, container.PathStat, error)` where the ReadCloser yields a tar stream. The caller wants raw file bytes, so we advance the tar reader to the first entry header and return a small adapter (`tarFileReader`) whose `Read` delegates to the tar reader and whose `Close` closes the underlying stream. The adapter checks `ctx.Err()` at the top of each `Read` so cancelling `ctx` aborts streaming reads — matching the Runtime interface contract.

`docker/errdefs` `Is*` functions are deprecated thin aliases over `containerd/errdefs` as of Docker SDK v28.5.2. The existing code already uses `cerrdefs.IsConflict` for the conflict path in `Remove`. Continue the convention: do NOT import `docker/errdefs`. For `CopyFrom`'s missing-file case, wrap the original Docker error with `%w` so callers can chain `cerrdefs.IsNotFound(err)` themselves — no new domain error.

`AttachStdin` is a Phase 1 `ExecOptions` field, but there is no `Stdin io.Reader` field on `ExecOptions` to source bytes from. Pass the flag through to `ExecCreate` so the daemon allocates a stdin pipe, then immediately `CloseWrite()` on the hijacked connection. When a future caller needs to pipe stdin in, the field will be added then.

## Implementation

### Step 1: Implement `Exec`

In `docker/docker.go`, replace the existing `Exec` stub. Add two new imports — `bytes` and `github.com/docker/docker/pkg/stdcopy` — to the import block. The `dc` alias already covers `dc.ExecOptions`, `dc.ExecAttachOptions`, and `dc.ExecInspect`.

```go
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
```

### Step 2: Implement `CopyTo`

Replace the `CopyTo` stub. Add `archive/tar` and `path` to the import block (`io` is already imported).

```go
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
```

### Step 3: Implement `CopyFrom` and `tarFileReader`

Replace the `CopyFrom` stub. Add the `tarFileReader` helper at the bottom of the file, after `mergeLabels`.

```go
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
```

## Validation Criteria

- [ ] `docker/docker.go` no longer contains the three "not implemented in sub-issue #11" stub errors
- [ ] Import block adds: `archive/tar`, `bytes`, `path`, `github.com/docker/docker/pkg/stdcopy`
- [ ] No new entries in `docker/go.mod` (these are stdlib + transitive Docker SDK)
- [ ] `cd docker && go build ./...` succeeds
- [ ] `cd docker && go vet ./...` passes
- [ ] `Exec` returns `*ExecResult` with `Stdout`/`Stderr` set only when the corresponding `Attach*` flag is true
- [ ] `CopyTo` calls `r.Exec(...mkdir -p...)` for non-trivial parent paths
- [ ] `CopyFrom` returns a `tarFileReader` that yields raw file bytes (caller doesn't see the tar wrapper)
- [ ] `docker/errdefs` is NOT imported anywhere; the file only imports `cerrdefs` (already present)
