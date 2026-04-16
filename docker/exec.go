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

// execInspectPollInterval is how often the wait loop polls
// ContainerExecInspect to detect process exit. 50ms keeps Wait responsive
// without flooding the daemon; Close short-circuits the poll via the done
// channel.
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

// ExecStream starts opts.Cmd inside the running container identified by id
// and returns an ExecSession with live Stdin, Stdout, and Stderr streams.
// The container must be in the running state. When opts.Tty is true, Docker
// merges the process's stderr onto stdout via the PTY and the session's
// Stderr yields io.EOF; otherwise a demux goroutine splits stdcopy-framed
// bytes into separate Stdout and Stderr pipes. Cancelling ctx aborts the
// in-flight ContainerExecCreate/ContainerExecAttach calls; once the session
// is returned, ctx has no further effect and callers use Close to terminate
// early. Wait blocks until the process exits naturally or Close short-
// circuits it.
func (r *dockerRuntime) ExecStream(
	ctx context.Context,
	id string,
	opts container.ExecStreamOptions,
) (*container.ExecSession, error) {
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

	hr, err := r.cli.ContainerExecAttach(
		ctx, create.ID, dc.ExecAttachOptions{Tty: opts.Tty},
	)
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

func (s *execStdin) Write(p []byte) (int, error) {
	return s.hr.Conn.Write(p)
}

func (s *execStdin) Close() error {
	return s.hr.CloseWrite()
}

type eofReader struct{}

func (eofReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}
