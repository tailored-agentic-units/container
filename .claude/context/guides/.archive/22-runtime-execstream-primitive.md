# 22 — Runtime.ExecStream primitive and Docker implementation

## Problem Context

`Runtime.Exec` is one-shot: it captures stdout/stderr bytes into a `*ExecResult` that's only returned *after* the process exits. The Phase 2 persistent `Shell` (sub-issue #23) needs live bidirectional streams — stdin to push commands into a long-lived `bash -i`, stdout to read back output framed by a PS1 sentinel, and a way to kill the session without killing the container.

This issue adds the streaming primitive that the Shell will sit on: `Runtime.ExecStream(ctx, id, opts) (*ExecSession, error)`. `ExecSession` exposes `Stdin io.WriteCloser`, `Stdout io.Reader`, `Stderr io.Reader`, `Wait() (int, error)`, `Close() error`. This sub-issue has no dependencies within Phase 2; it unblocks sub-issue #23.

## Architecture Approach

- **Separate method, not a flag on `ExecOptions`.** `Exec` returns `*ExecResult` bytes; `ExecStream` returns a live `*ExecSession`. The two contracts don't mix well as flags on the same options struct. Matches OCI's `exec` vs. `attach` verb separation (decision 1 in `_project/objective.md`).
- **`ExecSession` carries function-value fields.** Stdin/Stdout/Stderr are exported `io` fields; `WaitFn`/`CloseFn` are exported function-value fields that the runtime populates. `Wait`/`Close` methods call them with nil-checks so the zero value is safe. This avoids a 5-parameter constructor while keeping the user-facing API method-shaped.
- **Docker-side state lives on an `execStream` struct, not closures.** `execStream` owns `cli`, `execID`, `hr`, and the lifecycle primitives (`sync.WaitGroup` for the demux goroutine, `sync.Once` + `closeErr` for idempotent close, `done chan` to short-circuit the poll). `Runtime.ExecStream` becomes a short high-level sequence (create → attach → build struct → wire streams → return session); `wait`, `close`, and `wireStreams` are plain methods on the struct.
- **Stdin is half-closable.** An `execStdin` wrapper maps `Close` on the `Stdin` handle to `HijackedResponse.CloseWrite()` — signals EOF to the container process without tearing down stdout/stderr.
- **TTY-mode stderr contract.** When `ExecStreamOptions.Tty=true`, Docker merges stderr onto stdout via the PTY. `ExecSession.Stderr` in TTY mode is an `eofReader` that yields `io.EOF` immediately. Documented in godoc.
- **Non-TTY demux goroutine.** When `Tty=false`, `wireStreams` spawns a goroutine running `stdcopy.StdCopy` against `hr.Reader`, writing into two `io.Pipe` pairs whose read ends become `Stdout`/`Stderr`.
- **Lifecycle.** `ctx` passed to `ExecStream` aborts the `ContainerExecCreate`/`ContainerExecAttach` calls only; once the session is returned, `Close` is the escape hatch, and `Wait` blocks until natural exit or `Close`. A file-level `const execInspectPollInterval = 50 * time.Millisecond` governs `ContainerExecInspect` polling in `wait`; a `done` channel lets `close` short-circuit that poll.

## Implementation

### Step 1: Add `ExecStream` to the `Runtime` interface

**File:** `runtime.go` (existing — incremental edit)

Add the method below `Exec` in the interface, preserving the existing godoc block style. The `time` and `io` imports are already present.

```go
// ExecStream starts opts.Cmd inside the running container identified by id
// and returns an ExecSession handle exposing live Stdin, Stdout, Stderr
// streams plus Wait and Close. Cancelling ctx aborts the in-flight
// ContainerExecCreate/ContainerExecAttach API calls and returns an error
// wrapping ctx.Err(); once the session is returned, ctx has no further
// effect and callers use Close to terminate early. Wait blocks until the
// process exits naturally or Close short-circuits it.
ExecStream(ctx context.Context, id string, opts ExecStreamOptions) (*ExecSession, error)
```

### Step 2: Define `ExecStreamOptions` and `ExecSession` in the root module

**File:** `exec.go` (new)

```go
package container

import (
	"errors"
	"io"
)

type ExecStreamOptions struct {
	Cmd        []string
	Env        map[string]string
	WorkingDir string
	Tty        bool
}

type ExecSession struct {
	Stdin  io.WriteCloser
	Stdout io.Reader
	Stderr io.Reader

	WaitFn  func() (int, error)
	CloseFn func() error
}

func (s *ExecSession) Wait() (int, error) {
	if s.WaitFn == nil {
		return 0, errors.New("container: ExecSession not initialized")
	}
	return s.WaitFn()
}

func (s *ExecSession) Close() error {
	if s.CloseFn == nil {
		return nil
	}
	return s.CloseFn()
}
```

### Step 3: Implement `ExecStream` in the Docker sub-module

**File:** `docker/exec.go` (new)

Note: `types.HijackedResponse` lives in `github.com/docker/docker/api/types`; `dc.ExecAttachOptions` and `dc.ExecOptions` are in `github.com/docker/docker/api/types/container` (already aliased as `dc` in the sub-module).

```go
package docker

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	dc "github.com/docker/docker/api/types/container"

	"github.com/tailored-agentic-units/container"
)

const execInspectPollInterval = 50 * time.Millisecond

type execStream struct {
	cli    *client.Client
	execID string
	hr     types.HijackedResponse

	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
	done      chan struct{}
}

func (r *dockerRuntime) ExecStream(ctx context.Context, id string, opts container.ExecStreamOptions) (*container.ExecSession, error) {
	create, err := r.cli.ContainerExecCreate(ctx, id, dc.ExecOptions{
		Cmd:          opts.Cmd,
		Env:          buildEnv(opts.Env),
		WorkingDir:   opts.WorkingDir,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          opts.Tty,
	})
	if err != nil {
		return nil, fmt.Errorf("docker exec_stream: create: %w", err)
	}

	hr, err := r.cli.ContainerExecAttach(ctx, create.ID, dc.ExecAttachOptions{Tty: opts.Tty})
	if err != nil {
		return nil, fmt.Errorf("docker exec_stream: attach: %w", err)
	}

	es := &execStream{
		cli:    r.cli,
		execID: create.ID,
		hr:     hr,
		done:   make(chan struct{}),
	}

	stdout, stderr := es.wireStreams(opts.Tty)
	return &container.ExecSession{
		Stdin:   &execStdin{hr: hr},
		Stdout:  stdout,
		Stderr:  stderr,
		WaitFn:  es.wait,
		CloseFn: es.close,
	}, nil
}

// wireStreams returns Stdout/Stderr readers for the session. TTY mode exposes
// hr.Reader directly and yields EOF on Stderr (the PTY merges streams);
// non-TTY mode spawns a demux goroutine copying stdcopy-framed bytes into
// two io.Pipe pairs.
func (e *execStream) wireStreams(tty bool) (stdout, stderr io.Reader) {
	if tty {
		return e.hr.Reader, eofReader{}
	}

	stdoutPr, stdoutPw := io.Pipe()
	stderrPr, stderrPw := io.Pipe()

	e.wg.Go(func() {
		_, err := stdcopy.StdCopy(stdoutPw, stderrPw, e.hr.Reader)
		if err != nil {
			stdoutPw.CloseWithError(err)
			stderrPw.CloseWithError(err)
			return
		}
		stdoutPw.Close()
		stderrPw.Close()
	})

	return stdoutPr, stderrPr
}

func (e *execStream) close() error {
	e.closeOnce.Do(func() {
		close(e.done)
		e.closeErr = e.hr.Conn.Close()
		e.wg.Wait()
	})
	return e.closeErr
}

func (e *execStream) wait() (int, error) {
	ticker := time.NewTicker(execInspectPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.done:
			code, _, err := e.inspect()
			return code, err
		case <-ticker.C:
			code, running, err := e.inspect()
			if err != nil {
				return 0, err
			}
			if !running {
				return code, nil
			}
		}
	}
}

func (e *execStream) inspect() (exitCode int, running bool, err error) {
	insp, err := e.cli.ContainerExecInspect(context.Background(), e.execID)
	if err != nil {
		return 0, false, fmt.Errorf("docker exec_stream: inspect: %w", err)
	}
	return insp.ExitCode, insp.Running, nil
}

type execStdin struct {
	hr types.HijackedResponse
}

func (s *execStdin) Write(p []byte) (int, error) { return s.hr.Conn.Write(p) }
func (s *execStdin) Close() error                { return s.hr.CloseWrite() }

type eofReader struct{}

func (eofReader) Read(p []byte) (int, error) { return 0, io.EOF }
```

## Validation Criteria

- [ ] `go build ./...` succeeds in the root module
- [ ] `(cd docker && go build ./...)` succeeds
- [ ] `go vet ./...` clean in both modules
- [ ] `Runtime` interface has `ExecStream`; `dockerRuntime` satisfies it (compile-time)
- [ ] `ExecSession` zero value: `Close()` returns nil, `Wait()` returns a descriptive error, `Stdin/Stdout/Stderr` nil
- [ ] `execStdin.Close()` calls `HijackedResponse.CloseWrite()`, not the full conn close
- [ ] Non-TTY mode: separate `Stdout`/`Stderr` via `stdcopy.StdCopy`
- [ ] TTY mode: `Stderr` yields EOF; `Stdout = hr.Reader`
- [ ] `close` is idempotent (guarded by `sync.Once`); terminates a long-running process
- [ ] `wait` short-circuits when `close` is called first
- [ ] `ctx` cancellation during `ExecStream` returns an error wrapping `ctx.Err()`
- [ ] `execInspectPollInterval` named constant (not a magic `50 * time.Millisecond` literal inside `wait`)
