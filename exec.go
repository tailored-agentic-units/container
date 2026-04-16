package container

import (
	"errors"
	"io"
)

// ExecStreamOptions carries the parameters for Runtime.ExecStream. Unlike
// ExecOptions, there are no AttachStdin/Stdout/Stderr flags — ExecStream
// always returns a live session whose Stdin, Stdout, and Stderr streams are
// exposed through the returned *ExecSession.
type ExecStreamOptions struct {
	// Cmd is the command and arguments to execute. Required.
	Cmd []string
	// Env is the environment variables to set for the exec'd process. These
	// merge with the container's environment.
	Env map[string]string
	// WorkingDir sets the working directory for the exec'd process. Empty uses
	// the container's working directory.
	WorkingDir string
	// Tty requests a PTY for the exec'd process. When true, the runtime
	// merges the process's stderr onto stdout (standard PTY behavior) and
	// ExecSession.Stderr yields io.EOF immediately — callers that need
	// separated streams must leave Tty false.
	Tty bool
}

// ExecSession is the handle returned by Runtime.ExecStream for a live,
// streaming exec. Callers interact with the process through Stdin, Stdout,
// and Stderr, then call Wait to block for the exit code or Close to
// terminate the process early.
//
// Runtime implementations populate the exported fields — including WaitFn
// and CloseFn, which back Wait and Close. The zero value is safe: Close
// returns nil, and Wait returns a descriptive error instead of panicking on
// a nil callback.
type ExecSession struct {
	// Stdin writes bytes into the process's stdin. Closing Stdin signals
	// EOF to the process without tearing down Stdout/Stderr.
	Stdin io.WriteCloser
	// Stdout reads bytes from the process's stdout. In TTY mode, stderr is
	// merged into this stream (see ExecStreamOptions.Tty).
	Stdout io.Reader
	// Stderr reads bytes from the process's stderr. In TTY mode, this stream
	// yields io.EOF immediately because the PTY merges stderr onto Stdout.
	Stderr io.Reader

	// WaitFn is invoked by Wait. Runtimes populate this when constructing
	// the session; a nil WaitFn causes Wait to return a descriptive error.
	WaitFn func() (int, error)
	// CloseFn is invoked by Close. Runtimes populate this when constructing
	// the session; a nil CloseFn causes Close to return nil.
	CloseFn func() error
}

// Wait blocks until the exec'd process exits and returns its exit code, or
// until Close short-circuits the session. The zero-value ExecSession
// returns a descriptive error rather than panicking.
func (s *ExecSession) Wait() (int, error) {
	if s.WaitFn == nil {
		return 0, errors.New("container: ExecSession not initialized")
	}
	return s.WaitFn()
}

// Close terminates the exec'd process and releases resources associated
// with the session. Close is safe to call multiple times; runtimes MUST
// guard CloseFn so repeated invocations are idempotent. The zero-value
// ExecSession returns nil.
func (s *ExecSession) Close() error {
	if s.CloseFn == nil {
		return nil
	}
	return s.CloseFn()
}
