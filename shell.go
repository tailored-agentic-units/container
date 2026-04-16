package container

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// DefaultShellPath is the shell executed when ShellOptions.ShellPath is
// empty. Callers targeting images without bash (for example minimal Alpine
// images shipping only busybox ash) should set ShellPath explicitly to
// /bin/sh or another shell that exists in the image.
const DefaultShellPath = "/bin/bash"

// ShellOptions carries the parameters for NewShell. The zero value runs
// DefaultShellPath with the container's default working directory and
// environment.
type ShellOptions struct {
	// WorkingDir sets the shell's initial working directory. Empty uses the
	// container's default.
	WorkingDir string
	// Env extends the container's environment for the shell process. These
	// are passed to ExecStream and merge with the container's own env.
	Env map[string]string
	// ShellPath is the absolute path to the shell binary inside the
	// container. Empty uses DefaultShellPath.
	ShellPath string
}

// Shell is a long-lived, PTY-attached interactive shell session inside a
// running container. It preserves working directory, environment, and shell
// history across successive Run calls — callers that need those semantics
// should prefer Shell over repeatedly invoking Runtime.Exec, which starts a
// fresh process each time.
//
// The zero value of Shell is not usable; construct instances via NewShell.
// Concurrent Run callers on a single *Shell are serialized internally so
// framing is never interleaved; independent *Shell instances on the same
// container run in parallel without any shared state.
type Shell struct {
	session  *ExecSession
	sentinel string
	reader   *bufio.Reader

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

// NewShell starts an interactive shell inside the container identified by
// containerID and returns a handle the caller drives via Run and tears down
// via Close. The container must be in the running state.
//
// NewShell calls Runtime.ExecStream with Tty=true, then primes the session
// by disabling terminal echo and output post-processing (so writes to stdin
// do not bounce back and \n is not transformed to \r\n) and clearing PS1
// and PS2 (so the interactive prompt emits no bytes between commands).
// Framing then relies entirely on sentinel markers Run emits via printf.
//
// Cancelling ctx aborts the initial ExecStream setup and priming; once
// NewShell has returned, ctx has no further effect and the shell is
// controlled by subsequent Run calls and Close.
func NewShell(
	ctx context.Context,
	rt Runtime,
	containerID string,
	opts ShellOptions,
) (*Shell, error) {
	shellPath := opts.ShellPath
	if shellPath == "" {
		shellPath = DefaultShellPath
	}

	session, err := rt.ExecStream(ctx, containerID, ExecStreamOptions{
		Cmd:        []string{shellPath, "-i"},
		Env:        opts.Env,
		WorkingDir: opts.WorkingDir,
		Tty:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("container: start shell: %w", err)
	}

	sh := &Shell{
		session:  session,
		sentinel: generateSentinel(),
		reader:   bufio.NewReader(session.Stdout),
	}

	if err := sh.prime(); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("container: prime shell: %w", err)
	}

	return sh, nil
}

// Close tears down the underlying ExecSession and marks the shell closed.
// Close is idempotent: subsequent calls return the error recorded on the
// first call and do not re-invoke the session's CloseFn. It is safe to call
// concurrently with an in-flight Run — closing the session unblocks the
// pending read, Run returns an error, and Close then flips the closed flag.
func (s *Shell) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.session.Close()
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
	})
	return s.closeErr
}

// Run executes cmd in the shell and returns the captured stdout, the
// shell's exit code for cmd, and an error. Because the shell runs under a
// PTY, stderr is merged onto stdout and both appear in the returned byte
// slice. The returned output reflects what cmd itself wrote; Run strips one
// trailing newline introduced by the framing protocol but preserves
// caller-authored newlines beyond that.
//
// Run serializes concurrent callers on a single *Shell — interleaving
// stdin writes would scramble the sentinel framing. Use separate *Shell
// instances on the same container for true parallelism.
//
// Cancelling ctx mid-Run closes the underlying session, which unblocks the
// in-flight read and surfaces as an error from Run; the *Shell becomes
// unusable afterwards. Callers needing finer-grained cancellation should
// compose shorter-lived shells around longer-lived contexts.
func (s *Shell) Run(ctx context.Context, cmd string) ([]byte, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, 0, errors.New("container: shell closed")
	}

	cancelDone := make(chan struct{})
	defer close(cancelDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.session.Close()
		case <-cancelDone:
		}
	}()

	payload := fmt.Sprintf("%s\nprintf '\\n%s\\n%%s\\n' \"$?\"\n", cmd, s.sentinel)
	if _, err := io.WriteString(s.session.Stdin, payload); err != nil {
		return nil, 0, fmt.Errorf("container: write cmd: %w", err)
	}

	var out bytes.Buffer
	if err := s.readUntilSentinel(&out); err != nil {
		return nil, 0, fmt.Errorf("container: read output: %w", err)
	}

	body := out.Bytes()
	if n := len(body); n > 0 && body[n-1] == '\n' {
		body = body[:n-1]
	}

	line, err := s.reader.ReadString('\n')
	if err != nil {
		return nil, 0, fmt.Errorf("container: read exit: %w", err)
	}
	code, err := strconv.Atoi(strings.TrimRight(line, "\r\n"))
	if err != nil {
		return nil, 0, fmt.Errorf("container: parse exit code %q: %w", line, err)
	}

	return body, code, nil
}

func (s *Shell) prime() error {
	payload := fmt.Sprintf("stty -echo -opost; PS1=''; PS2=''; printf '\\n%s\\n'\n", s.sentinel)
	if _, err := io.WriteString(s.session.Stdin, payload); err != nil {
		return fmt.Errorf("write prime: %w", err)
	}
	if err := s.readUntilSentinel(io.Discard); err != nil {
		return fmt.Errorf("wait for prime: %w", err)
	}
	return nil
}

func (s *Shell) readUntilSentinel(dst io.Writer) error {
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.TrimRight(line, "\r\n") == s.sentinel {
			return nil
		}
		if _, err := dst.Write([]byte(line)); err != nil {
			return err
		}
	}
}

func generateSentinel() string {
	return "tau." + uuid.Must(uuid.NewV7()).String()
}
