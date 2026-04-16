# 23 — Shell type wrapping ExecSession with PTY sentinel framing

## Problem Context

Objective #18 (Persistent Shell Foundation) builds a long-lived, PTY-attached shell on top of the `Runtime.ExecStream` primitive that landed in sub-issue #22. `ExecSession` alone isn't enough: an agent (or a human caller) driving the container needs stateful shell semantics — cwd and env preserved across commands, shell history, sourced rc files — without repeatedly paying the cost of a fresh `Exec` per command. Obj #20's `shell` built-in tool, and any future consumer that depends on rc-file-aware tools like `mise` or `direnv`, will wrap this `Shell` type.

The native Go API we're building here is the same surface the agent tool layer will expose. Simplicity matters: a human calling `shell.Run("ls -la")` and an agent-tool adapter calling it should both see a clean `(stdout, exitCode, error)` return, with no framing leakage.

## Architecture Approach

**PTY + silent-prompt sentinel framing.**

- Run the user's chosen shell (default `/bin/bash`) with `-i` under a PTY (`ExecStreamOptions.Tty = true`). Interactive mode is required for rc-file sourcing and `isatty()`-sensitive tool behavior.
- At startup, configure the terminal to be programmatically driveable: disable echo (`stty -echo`), disable output post-processing so `\n` isn't converted to `\r\n` on the slave's output (`stty -opost`), and set `PS1=''` / `PS2=''` so the interactive prompt emits zero bytes between commands.
- Frame each `Run` call with a per-`Shell` UUID sentinel emitted by `printf`. The payload is `<cmd>\nprintf '\n<sentinel>\n%s\n' "$?"\n`. Between the two bash commands there is no visible prompt (PS1 is empty), so the stream contains exactly `<cmd_output>\n<sentinel>\n<exit>\n`. Parsing is the literal reading of the issue spec: read until the sentinel line, then read the exit code from the next line.
- The leading `\n` in the printf format introduces one framing blank line before the sentinel (forcing the sentinel onto its own line even if `<cmd>` produced no trailing newline). Strip one trailing `\n` from the collected output to remove that framing artifact.
- Serialize concurrent `Run` callers on a single `*Shell` with a mutex — interleaving stdin writes would scramble the protocol. Independent `*Shell` instances on the same container are fully parallel.
- `Close` is idempotent via `sync.Once`; it tears down the underlying `ExecSession`, which unblocks any in-flight `ReadString` with EOF. No explicit drain goroutine needed.

## Implementation

### Step 1: Create `shell.go` at the root of the module

Create `/home/jaime/tau/container/shell.go` with the package declaration, imports, and the public constant and options type.

```go
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

const DefaultShellPath = "/bin/bash"

type ShellOptions struct {
	WorkingDir string
	Env        map[string]string
	ShellPath  string
}
```

### Step 2: Add the `Shell` type

Append the `Shell` struct to `shell.go`. State is fully unexported; the public method set is `Run` and `Close`.

```go
type Shell struct {
	sess     *ExecSession
	sentinel string
	reader   *bufio.Reader

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}
```

### Step 3: Implement `NewShell`

Append to `shell.go`. Resolves the shell path, generates a sentinel, starts a streaming exec, primes the shell, and returns the handle. On any priming error, the underlying `ExecSession` is closed before returning.

```go
func NewShell(ctx context.Context, rt Runtime, containerID string, opts ShellOptions) (*Shell, error) {
	shellPath := opts.ShellPath
	if shellPath == "" {
		shellPath = DefaultShellPath
	}

	sess, err := rt.ExecStream(ctx, containerID, ExecStreamOptions{
		Cmd:        []string{shellPath, "-i"},
		Env:        opts.Env,
		WorkingDir: opts.WorkingDir,
		Tty:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("container: start shell: %w", err)
	}

	sh := &Shell{
		sess:     sess,
		sentinel: generateSentinel(),
		reader:   bufio.NewReader(sess.Stdout),
	}

	if err := sh.prime(); err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("container: prime shell: %w", err)
	}

	return sh, nil
}
```

### Step 4: Implement `prime`

Append to `shell.go`. Sends a single compound bash line that disables echo and OPOST, empties PS1/PS2, and emits the sentinel as a prime marker. Any welcome banners, default prompts, or the echoed priming command itself are consumed as noise until the sentinel appears.

```go
func (s *Shell) prime() error {
	payload := fmt.Sprintf("stty -echo -opost; PS1=''; PS2=''; printf '\\n%s\\n'\n", s.sentinel)
	if _, err := io.WriteString(s.sess.Stdin, payload); err != nil {
		return fmt.Errorf("write prime: %w", err)
	}
	if err := s.readUntilSentinel(io.Discard); err != nil {
		return fmt.Errorf("wait for prime: %w", err)
	}
	return nil
}
```

### Step 5: Implement `Run`

Append to `shell.go`. Serializes concurrent callers via `s.mu`, checks the closed flag, writes the framing payload, reads until the sentinel, strips one trailing `\n` (the framing blank), then parses the exit code from the next line.

The ctx watchdog goroutine closes the session if `ctx` is cancelled; the in-flight read unblocks with EOF and the deferred `cancelDone` close stops the watchdog on normal return.

```go
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
			_ = s.sess.Close()
		case <-cancelDone:
		}
	}()

	payload := fmt.Sprintf("%s\nprintf '\\n%s\\n%%s\\n' \"$?\"\n", cmd, s.sentinel)
	if _, err := io.WriteString(s.sess.Stdin, payload); err != nil {
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
```

### Step 6: Implement `readUntilSentinel` and `Close`

Append to `shell.go`. `readUntilSentinel` reads full lines and compares against the sentinel (trailing `\r\n` trimmed) for exact full-line matches; partial or embedded sentinels in command output never trigger.

```go
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

func (s *Shell) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.closeErr = s.sess.Close()
	})
	return s.closeErr
}
```

### Step 7: Add the sentinel generator

Append to `shell.go`. Uses `github.com/google/uuid` for ecosystem consistency — `tau/agent`, `tau/kernel`, and `tau/orchestrate` all depend on it and follow the same UUID v7 convention (`uuid.Must(uuid.NewV7()).String()`). v7 is time-ordered: the leading 48 bits are a millisecond-precision timestamp, which makes sentinels easy to correlate in logs even across multiple Shell instances. The `tau.` prefix makes them easy to spot.

```go
func generateSentinel() string {
	return "tau." + uuid.Must(uuid.NewV7()).String()
}
```

Since `uuid.NewV7` only fails if the system CSPRNG is unavailable (pathological — same condition under which nothing else in the process would work), `uuid.Must` is the ecosystem-standard choice. This simplifies `NewShell`: no error to propagate from sentinel generation. Update the call site from step 3 accordingly — remove the `sentinel, err := generateSentinel()` error handling and use `sentinel := generateSentinel()`.

### Step 8: Wire google/uuid into `go.mod`

Run `go get github.com/google/uuid@v1.6.0` at the container root to pin the same version the rest of the TAU ecosystem uses. Then `go mod tidy` to normalize.

## Validation Criteria

- [ ] `shell.go` compiles cleanly: `go build ./...` at the container root succeeds.
- [ ] `go vet ./...` at the container root reports nothing.
- [ ] `go.mod` has `github.com/google/uuid v1.6.0` pinned, matching the rest of the TAU ecosystem. `go mod tidy` is clean after the add.
- [ ] Public surface matches the plan: `DefaultShellPath` const, `ShellOptions` struct with `WorkingDir`/`Env`/`ShellPath` fields, `Shell` type, `NewShell(ctx, rt, containerID, opts) (*Shell, error)`, `(*Shell).Run(ctx, cmd) ([]byte, int, error)`, `(*Shell).Close() error`.
- [ ] All `Shell` state is unexported; the only public entry points are `NewShell` and the two methods.
- [ ] No godoc comments yet — those land in Phase 7 (Documentation).
