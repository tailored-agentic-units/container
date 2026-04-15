# Plan — Issue #12: Docker runtime Exec and file copy methods

## Context

Sub-issue B of Objective 3. Implements the three I/O methods (`Exec`, `CopyTo`, `CopyFrom`) on `dockerRuntime`, replacing the "not implemented in sub-issue #11" stubs in `docker/docker.go`. Lifecycle (#11) is merged on `main`; this branch builds on top, leaving sub-issue C (`Inspect` + manifest integration) to consume `CopyFrom`.

The core challenge is uniform context-cancellation across three different streaming APIs: a hijacked TCP connection (Exec), a tar-stream upload (CopyTo), and a tar-stream download (CopyFrom). Each requires a slightly different cancellation primitive.

## Errdefs note

`github.com/docker/docker/errdefs` `Is*` functions are all `var Is* = cerrdefs.Is*` aliases marked Deprecated as of v28.5.2. The detection layer has pivoted entirely to `github.com/containerd/errdefs`. The existing code already uses `cerrdefs.IsConflict` (`docker.go:124`); this PR continues the convention with `cerrdefs.IsNotFound` for the missing-file case in `CopyFrom`. We will NOT add an import of `docker/errdefs`.

## Architecture approach

### Exec

Sequence: `ContainerExecCreate` → `ContainerExecAttach` → `stdcopy.StdCopy` to demux into per-stream buffers → `ContainerExecInspect` for `ExitCode`. Capture into `bytes.Buffer` always; only assign `ExecResult.Stdout`/`Stderr` when the corresponding `Attach*` flag is set (so unattached streams stay nil per `ExecResult` godoc and the acceptance criteria).

**Cancellation**: Docker SDK v28.5.2 has no `ContainerExecKill`. The conventional way to abort an in-flight exec is to close the hijacked connection, which terminates the attached process. Pattern: launch the demux drain in a goroutine; in the calling goroutine, `select` on `ctx.Done()` and the goroutine's done channel. On `ctx.Done()`, call `HijackedResponse.Close()` to unblock the drain, then `return nil, fmt.Errorf("docker exec: %w", ctx.Err())`. On normal completion, fall through to `ContainerExecInspect`.

**Stdin**: when `AttachStdin` is true and `ExecOptions.Stdin` (we don't have one — see open question) is provided, copy from caller's stdin to `HijackedResponse.Conn`, then call `CloseWrite()`. Phase 1 `ExecOptions` defines `AttachStdin bool` but does NOT define a stdin reader field — so the flag is currently load-bearing only on the Docker side (it tells the daemon to allocate a stdin pipe). Without a reader to source bytes from, Phase 1 implementation will set the flag through to `ExecCreate` and immediately `CloseWrite()` on the hijacked conn. This is consistent with the issue scope (no interactive TTY — that's Phase 2).

### CopyTo

`CopyToContainer` requires the parent directory to exist and a tar stream as content. Approach:

1. Split `dst` into `parent = filepath.Dir(dst)` and `base = filepath.Base(dst)`.
2. If `parent != "/" && parent != "."`, exec `mkdir -p <parent>` inside the container via `r.Exec(...)` to ensure the parent exists. Reuses the new `Exec` method we just wrote — no helper duplication.
3. Build a single-entry in-memory tar archive (`bytes.Buffer` + `archive/tar`) with `Name = base`, mode `0644`, size derived by reading `content` into a buffer first (tar headers require Size up-front).
4. Call `cli.CopyToContainer(ctx, id, parent, tarBuf, container.CopyToContainerOptions{})`. Cancellation propagates through `ctx` because the SDK call is blocking on the same context.

**Precondition**: container must be running (mkdir requires exec, exec requires running). Document on the method.

### CopyFrom

`CopyFromContainer` returns `(io.ReadCloser, container.PathStat, error)` where the ReadCloser yields a tar stream. Per the issue acceptance criteria, the caller wants raw file bytes, not a tarball. Approach:

1. Call `cli.CopyFromContainer(ctx, id, src)`.
2. On error, check `cerrdefs.IsNotFound(err)` — wrap as `fmt.Errorf("docker copy_from: %w", err)` so callers see the underlying not-found via `errors.Is`. Document this on the method godoc; do NOT introduce `ErrFileNotFound` (out of scope per issue).
3. Wrap the tar stream in a small `tarFileReader` adapter:
   - Constructor calls `tar.NewReader(rc).Next()` to advance to the first entry header (returning early-close + error if Next() fails).
   - `Read(p)` delegates to the tar reader (which yields entry body bytes, EOF after entry).
   - `Close()` closes the underlying ReadCloser.

**Cancellation**: `CopyFromContainer` honors `ctx` for the initial RPC; after the call returns, the stream is detached from the original ctx. Per the Runtime interface godoc ("Cancelling ctx aborts the copy stream"), we should ensure stream-reads also respect cancellation. The simplest way: have `tarFileReader` carry the ctx and check `ctx.Err()` at the top of each `Read`. This satisfies the interface contract without spawning goroutines.

## Implementation plan

### Step 1: Add Exec to `docker/docker.go`

Replace the stub with a real implementation. New imports: `bytes`, `github.com/docker/docker/pkg/stdcopy`. The `dc` alias already covers the exec-options and exec-inspect types.

Key snippets the developer will write:

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
        hr.Close() // unblock drain
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

### Step 2: Add CopyTo to `docker/docker.go`

Replace the stub. New imports: `archive/tar`, `bytes`, `io`, `path` (use `path.Dir`/`path.Base` — POSIX semantics, not host filesystem).

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

### Step 3: Add CopyFrom and `tarFileReader` to `docker/docker.go`

Replace the stub. New helper type at file bottom. The `cerrdefs` alias is already imported (used by `Remove`).

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

`cerrdefs.IsNotFound` is *not* called explicitly — by wrapping the original Docker error with `%w`, callers can already chain `errors.Is(err, ...)` themselves, and the godoc tells them the not-found case is detectable via `cerrdefs.IsNotFound`.

### Step 4: Validation

Run from repo root:

```bash
cd /home/jaime/tau/container/docker
go build ./...
go vet ./...
go test ./tests/... -count=1
```

If the daemon is up, all 8 new tests + the 5 existing lifecycle tests run. If not, all skip with the existing `skipIfNoDaemon` plumbing.

Optional coverage check (per Objective 3 acceptance criterion):
```bash
go test -coverpkg=github.com/tailored-agentic-units/container/docker ./tests/...
```

## Implementation guide vs. AI-owned phases

Per the dev-workflow convention (and reinforced by user feedback during plan review): the **implementation guide** the developer executes contains only Steps 1–3 (the three method bodies and `tarFileReader`) plus a `Validation Criteria` checklist. Tests are NOT in the guide — they are AI-owned in Phase 5.

After developer execution, the AI handles:
- **Phase 5 — Tests**: write `docker/tests/io_test.go` with the test matrix below.
- **Phase 6 — Validation**: `go build`, `go vet`, `go test` from the docker sub-module.
- **Phase 7 — Documentation**: godoc comments on the three methods documenting cancellation, preconditions, and the not-found wrapping behavior of `CopyFrom`.
- **Phase 8 — Closeout**: session summary, archive guide, CHANGELOG entry, PR.

### Planned test matrix (Phase 5, post-implementation)

| Test | What it covers |
|------|----------------|
| `TestExec_StdoutCapture` | `echo hello`, AttachStdout, expect Stdout=="hello\n", ExitCode==0, Stderr nil |
| `TestExec_StderrCapture` | `sh -c 'echo oops 1>&2'`, AttachStderr, expect Stderr=="oops\n", Stdout nil |
| `TestExec_NoAttach_NilBuffers` | `true`, no Attach* flags, expect Stdout/Stderr both nil, ExitCode==0 |
| `TestExec_NonZeroExit` | `sh -c 'exit 7'`, expect ExitCode==7 |
| `TestExec_CtxCancel` | `sleep 30`, cancel ctx after 100ms, expect error wrapping `context.Canceled` |
| `TestCopyTo_RoundTrip` | Write `/workspace/hello.txt` containing "hi\n"; `CopyFrom` it; assert bytes equal |
| `TestCopyTo_NestedPath` | Write `/a/b/c/file.txt`; `CopyFrom` it; assert bytes equal |
| `TestCopyFrom_AbsentFile` | `CopyFrom` `/does/not/exist`; assert `errors.Is` via `cerrdefs.IsNotFound` |

## Critical files

- `/home/jaime/tau/container/docker/docker.go` — replace 3 stub bodies, add `tarFileReader`, add imports (`archive/tar`, `bytes`, `path`, `github.com/docker/docker/pkg/stdcopy`)
- `/home/jaime/tau/container/docker/tests/io_test.go` — new file, written in Phase 5 by AI
- (no changes to root module, no changes to existing tests, no go.mod changes — `stdcopy` and `archive/tar` are transitively available)

## Verification

End-to-end manual smoke (developer can run after implementation):
```bash
cd /home/jaime/tau/container/docker && go test ./tests/ -run TestCopyTo_RoundTrip -v
```
Should observe a passing round-trip if Docker is reachable, or a clean skip if not.

## Open questions / risks

- **Stdin attach without a reader**: Phase 1 `ExecOptions` has no `Stdin io.Reader` field. Setting `AttachStdin: true` and immediately `CloseWrite()` is a defensible no-op for now (the daemon allocates the pipe; we close our side immediately). If a future caller needs to pipe stdin in, the field will be added then. Documented in this plan; no test exercises stdin since there's nothing to feed.
- **`Exec` ctx-cancel race**: closing `hr` while `StdCopy` is mid-read can return either the ctx.Err path or a "use of closed network connection" error from the goroutine. We `<-drainErr` to drain the goroutine's error and discard it, returning the ctx error. This matches the documented contract ("error wrapping ctx.Err()").
- **`CopyTo` parent existence**: relying on `mkdir -p` via Exec means CopyTo has a hidden dependency on a working shell + mkdir in the container image. Alpine has both. Document the precondition on the method.
