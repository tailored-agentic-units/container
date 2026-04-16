package container_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tailored-agentic-units/container"
)

// primeLineRE matches the prime payload's printf step and captures the
// sentinel. The payload shell.go writes is:
//
//	stty -echo -opost; PS1=''; PS2=''; printf '\n<sentinel>\n'
//
// where \n is the literal two-byte sequence backslash-n (bash's printf format),
// not an actual newline. In the Go regex (raw string), \\n matches that
// literal \n sequence.
var primeLineRE = regexp.MustCompile(`printf '\\n(tau\.[a-f0-9-]+)\\n'`)

// scriptedResponse is a single Run's simulated bash output.
type scriptedResponse struct {
	output []byte
	exit   int
}

// fakeBash drives a scripted bash-shaped conversation over the io.Pipes
// that back a fake ExecSession. It reads the prime payload from stdin to
// learn the Shell's sentinel, then for each Run emits <output>\n<sent>\n<exit>\n
// — exactly the byte pattern a real bash with PS1='' would produce given
// the framing payload shell.go writes.
type fakeBash struct {
	stdinR  *io.PipeReader
	stdoutW *io.PipeWriter
	script  []scriptedResponse

	mu           sync.Mutex
	sentinel     string
	observedCmds []string
	primed       bool
	done         chan struct{}
}

func (f *fakeBash) run() {
	defer close(f.done)
	defer f.stdoutW.Close()

	scanner := bufio.NewScanner(f.stdinR)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	// Prime phase: find the printf line, extract sentinel, echo it back.
	for scanner.Scan() {
		line := scanner.Text()
		if m := primeLineRE.FindStringSubmatch(line); m != nil {
			f.mu.Lock()
			f.sentinel = m[1]
			f.primed = true
			f.mu.Unlock()
			if _, err := io.WriteString(f.stdoutW, "\n"+m[1]+"\n"); err != nil {
				return
			}
			break
		}
	}

	// Command phase: read two lines per Run (cmd, printf framing),
	// emit the scripted response.
	idx := 0
	var pending *string
	for scanner.Scan() {
		line := scanner.Text()
		if pending == nil {
			s := line
			pending = &s
			continue
		}
		// Second line is the framing printf — recognize but ignore.
		f.mu.Lock()
		f.observedCmds = append(f.observedCmds, *pending)
		sent := f.sentinel
		f.mu.Unlock()
		pending = nil

		if idx >= len(f.script) {
			return
		}
		resp := f.script[idx]
		idx++

		if _, err := f.stdoutW.Write(resp.output); err != nil {
			return
		}
		if _, err := fmt.Fprintf(f.stdoutW, "\n%s\n%d\n", sent, resp.exit); err != nil {
			return
		}
	}
}

// newFakeSession wires up a fake ExecSession + fakeBash over io.Pipes.
// closeCount is incremented on CloseFn invocation (for idempotency tests).
type fakeSessionHandle struct {
	sess       *container.ExecSession
	fb         *fakeBash
	closeCount *int
	stdinW     *io.PipeWriter
	stdoutR    *io.PipeReader
}

func newFakeSession(script []scriptedResponse) *fakeSessionHandle {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	fb := &fakeBash{
		stdinR:  stdinR,
		stdoutW: stdoutW,
		script:  script,
		done:    make(chan struct{}),
	}
	go fb.run()

	var closeCount int
	var closeMu sync.Mutex

	sess := &container.ExecSession{
		Stdin:  stdinW,
		Stdout: stdoutR,
		Stderr: shellEOFReader{},
		WaitFn: func() (int, error) {
			<-fb.done
			return 0, nil
		},
		CloseFn: func() error {
			closeMu.Lock()
			closeCount++
			closeMu.Unlock()
			// Closing both pipe ends unblocks any in-flight reads/writes.
			stdinW.Close()
			stdoutR.Close()
			stdoutW.Close()
			return nil
		},
	}

	return &fakeSessionHandle{
		sess:       sess,
		fb:         fb,
		closeCount: &closeCount,
		stdinW:     stdinW,
		stdoutR:    stdoutR,
	}
}

type shellEOFReader struct{}

func (shellEOFReader) Read(p []byte) (int, error) { return 0, io.EOF }

// fakeRuntime implements container.Runtime with only ExecStream wired up.
// All other methods panic — the tests only exercise ExecStream.
type fakeRuntime struct {
	capturedOpts container.ExecStreamOptions
	capturedID   string
	session      *container.ExecSession
	execErr      error
}

func (f *fakeRuntime) ExecStream(ctx context.Context, id string, opts container.ExecStreamOptions) (*container.ExecSession, error) {
	f.capturedID = id
	f.capturedOpts = opts
	if f.execErr != nil {
		return nil, f.execErr
	}
	return f.session, nil
}

func (f *fakeRuntime) Create(ctx context.Context, opts container.CreateOptions) (*container.Container, error) {
	panic("fakeRuntime.Create not expected")
}
func (f *fakeRuntime) Start(ctx context.Context, id string) error {
	panic("fakeRuntime.Start not expected")
}
func (f *fakeRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	panic("fakeRuntime.Stop not expected")
}
func (f *fakeRuntime) Remove(ctx context.Context, id string, force bool) error {
	panic("fakeRuntime.Remove not expected")
}
func (f *fakeRuntime) Exec(ctx context.Context, id string, opts container.ExecOptions) (*container.ExecResult, error) {
	panic("fakeRuntime.Exec not expected")
}
func (f *fakeRuntime) CopyTo(ctx context.Context, id string, dst string, content io.Reader) error {
	panic("fakeRuntime.CopyTo not expected")
}
func (f *fakeRuntime) CopyFrom(ctx context.Context, id string, src string) (io.ReadCloser, error) {
	panic("fakeRuntime.CopyFrom not expected")
}
func (f *fakeRuntime) Inspect(ctx context.Context, id string) (*container.ContainerInfo, error) {
	panic("fakeRuntime.Inspect not expected")
}

func newShellWithScript(t *testing.T, opts container.ShellOptions, script []scriptedResponse) (*container.Shell, *fakeRuntime, *fakeSessionHandle) {
	t.Helper()
	fsh := newFakeSession(script)
	rt := &fakeRuntime{session: fsh.sess}

	sh, err := container.NewShell(context.Background(), rt, "fake-container", opts)
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}
	t.Cleanup(func() { _ = sh.Close() })

	return sh, rt, fsh
}

func TestNewShell_DefaultsShellPath(t *testing.T) {
	_, rt, _ := newShellWithScript(t, container.ShellOptions{}, nil)

	want := []string{container.DefaultShellPath, "-i"}
	if len(rt.capturedOpts.Cmd) != len(want) {
		t.Fatalf("Cmd length: got %d, want %d", len(rt.capturedOpts.Cmd), len(want))
	}
	for i, v := range want {
		if rt.capturedOpts.Cmd[i] != v {
			t.Errorf("Cmd[%d]: got %q, want %q", i, rt.capturedOpts.Cmd[i], v)
		}
	}
}

func TestNewShell_CustomShellPath(t *testing.T) {
	_, rt, _ := newShellWithScript(t, container.ShellOptions{ShellPath: "/bin/sh"}, nil)

	if rt.capturedOpts.Cmd[0] != "/bin/sh" {
		t.Errorf("Cmd[0]: got %q, want %q", rt.capturedOpts.Cmd[0], "/bin/sh")
	}
}

func TestNewShell_ForwardsOptions(t *testing.T) {
	opts := container.ShellOptions{
		WorkingDir: "/workspace",
		Env:        map[string]string{"FOO": "bar"},
	}
	_, rt, _ := newShellWithScript(t, opts, nil)

	if rt.capturedOpts.WorkingDir != "/workspace" {
		t.Errorf("WorkingDir: got %q, want %q", rt.capturedOpts.WorkingDir, "/workspace")
	}
	if rt.capturedOpts.Env["FOO"] != "bar" {
		t.Errorf("Env[FOO]: got %q, want %q", rt.capturedOpts.Env["FOO"], "bar")
	}
	if !rt.capturedOpts.Tty {
		t.Error("Tty: got false, want true")
	}
	if rt.capturedID != "fake-container" {
		t.Errorf("containerID: got %q, want %q", rt.capturedID, "fake-container")
	}
}

func TestNewShell_ExecStreamError_Propagates(t *testing.T) {
	fsh := newFakeSession(nil)
	wantErr := errors.New("exec-stream boom")
	rt := &fakeRuntime{session: fsh.sess, execErr: wantErr}

	sh, err := container.NewShell(context.Background(), rt, "fake", container.ShellOptions{})
	if sh != nil {
		t.Error("NewShell returned non-nil Shell on ExecStream error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err does not wrap wantErr: %v", err)
	}
}

func TestShell_Run_BasicOutput(t *testing.T) {
	sh, _, _ := newShellWithScript(t, container.ShellOptions{}, []scriptedResponse{
		{output: []byte("hello\n"), exit: 0},
	})

	out, code, err := sh.Run(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if string(out) != "hello\n" {
		t.Errorf("output: got %q, want %q", out, "hello\n")
	}
}

func TestShell_Run_NonZeroExit(t *testing.T) {
	sh, _, _ := newShellWithScript(t, container.ShellOptions{}, []scriptedResponse{
		{output: nil, exit: 7},
	})

	out, code, err := sh.Run(context.Background(), "false")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 7 {
		t.Errorf("exit code: got %d, want 7", code)
	}
	if len(out) != 0 {
		t.Errorf("output: got %q, want empty", out)
	}
}

func TestShell_Run_EmptyOutput(t *testing.T) {
	sh, _, _ := newShellWithScript(t, container.ShellOptions{}, []scriptedResponse{
		{output: []byte{}, exit: 0},
	})

	out, code, err := sh.Run(context.Background(), "true")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if len(out) != 0 {
		t.Errorf("output: got %q, want empty", out)
	}
}

func TestShell_Run_OutputWithoutTrailingNewline(t *testing.T) {
	// Simulates `printf hello` — no trailing \n in the cmd output.
	// The printf framing's leading \n still forces the sentinel onto
	// its own line; the parser should return "hello" exactly.
	sh, _, _ := newShellWithScript(t, container.ShellOptions{}, []scriptedResponse{
		{output: []byte("hello"), exit: 0},
	})

	out, _, err := sh.Run(context.Background(), "printf hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "hello" {
		t.Errorf("output: got %q, want %q", out, "hello")
	}
}

func TestShell_Run_PreservesLegitimateBlankLines(t *testing.T) {
	// Output with a legitimate trailing blank line — e.g., `printf 'a\n\n'`.
	// We strip only ONE trailing \n (the framing artifact), preserving
	// caller-authored blank lines.
	sh, _, _ := newShellWithScript(t, container.ShellOptions{}, []scriptedResponse{
		{output: []byte("a\n\n"), exit: 0},
	})

	out, _, err := sh.Run(context.Background(), "printf 'a\\n\\n'")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "a\n\n" {
		t.Errorf("output: got %q, want %q", out, "a\n\n")
	}
}

func TestShell_Run_EmbeddedSentinelSubstring(t *testing.T) {
	// Output contains a tau.-prefixed string that LOOKS like a sentinel
	// but isn't this Shell's actual sentinel. Full-line exact-match
	// semantics mean this line passes through as output.
	near := "tau.aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\n"
	sh, _, _ := newShellWithScript(t, container.ShellOptions{}, []scriptedResponse{
		{output: []byte(near), exit: 0},
	})

	out, _, err := sh.Run(context.Background(), "echo fake-sentinel")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != near {
		t.Errorf("output: got %q, want %q", out, near)
	}
}

func TestShell_Run_MultipleInSequence(t *testing.T) {
	sh, _, fsh := newShellWithScript(t, container.ShellOptions{}, []scriptedResponse{
		{output: []byte("first\n"), exit: 0},
		{output: []byte("second\n"), exit: 2},
	})

	out1, code1, err := sh.Run(context.Background(), "echo first")
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if string(out1) != "first\n" || code1 != 0 {
		t.Errorf("Run 1: got (%q, %d), want (%q, 0)", out1, code1, "first\n")
	}

	out2, code2, err := sh.Run(context.Background(), "echo second")
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if string(out2) != "second\n" || code2 != 2 {
		t.Errorf("Run 2: got (%q, %d), want (%q, 2)", out2, code2, "second\n")
	}

	fsh.fb.mu.Lock()
	got := append([]string(nil), fsh.fb.observedCmds...)
	fsh.fb.mu.Unlock()

	if len(got) != 2 || got[0] != "echo first" || got[1] != "echo second" {
		t.Errorf("observed cmds: got %v, want [echo first, echo second]", got)
	}
}

func TestShell_Close_Idempotent(t *testing.T) {
	sh, _, fsh := newShellWithScript(t, container.ShellOptions{}, nil)

	if err := sh.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := sh.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if *fsh.closeCount != 1 {
		t.Errorf("CloseFn invocations: got %d, want 1", *fsh.closeCount)
	}
}

func TestShell_Run_AfterClose_ReturnsError(t *testing.T) {
	sh, _, _ := newShellWithScript(t, container.ShellOptions{}, nil)

	if err := sh.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, _, err := sh.Run(context.Background(), "echo hi")
	if err == nil {
		t.Fatal("Run after Close: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("err %q does not mention 'closed'", err)
	}
}

func TestShell_Run_ExitParseFailure(t *testing.T) {
	// Craft a handcrafted fake that emits a non-numeric exit line, to
	// exercise the strconv.Atoi error path.
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer stdoutW.Close()
		scanner := bufio.NewScanner(stdinR)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		// Prime
		var sentinel string
		for scanner.Scan() {
			if m := primeLineRE.FindStringSubmatch(scanner.Text()); m != nil {
				sentinel = m[1]
				io.WriteString(stdoutW, "\n"+sentinel+"\n")
				break
			}
		}
		// One Run: valid output, invalid exit
		scanner.Scan() // cmd
		scanner.Scan() // framing printf
		fmt.Fprintf(stdoutW, "output\n\n%s\nNOT_A_NUMBER\n", sentinel)
	}()

	sess := &container.ExecSession{
		Stdin:   stdinW,
		Stdout:  stdoutR,
		Stderr:  shellEOFReader{},
		WaitFn:  func() (int, error) { <-done; return 0, nil },
		CloseFn: func() error { stdinW.Close(); stdoutR.Close(); stdoutW.Close(); return nil },
	}
	rt := &fakeRuntime{session: sess}

	sh, err := container.NewShell(context.Background(), rt, "fake", container.ShellOptions{})
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}
	t.Cleanup(func() { _ = sh.Close() })

	_, _, err = sh.Run(context.Background(), "echo")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse exit code") {
		t.Errorf("err %q does not mention 'parse exit code'", err)
	}
}

func TestShell_Close_DuringRun_NoDeadlock(t *testing.T) {
	// A fake-bash that primes but never responds to a Run command.
	// The Run call blocks in readUntilSentinel; Close (from another
	// goroutine) must tear down the session without deadlocking on s.mu.
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	done := make(chan struct{})

	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdinR)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			if m := primeLineRE.FindStringSubmatch(scanner.Text()); m != nil {
				io.WriteString(stdoutW, "\n"+m[1]+"\n")
				break
			}
		}
		// Drain subsequent stdin so Shell's Run write doesn't block forever;
		// but never emit a response so readUntilSentinel stays blocked.
		_, _ = io.Copy(io.Discard, stdinR)
	}()

	sess := &container.ExecSession{
		Stdin:  stdinW,
		Stdout: stdoutR,
		Stderr: shellEOFReader{},
		WaitFn: func() (int, error) { <-done; return 0, nil },
		CloseFn: func() error {
			// Closing writer side of stdout unblocks Shell's Read with EOF.
			stdoutW.Close()
			stdinW.Close()
			stdoutR.Close()
			return nil
		},
	}
	rt := &fakeRuntime{session: sess}

	sh, err := container.NewShell(context.Background(), rt, "fake", container.ShellOptions{})
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, _, err := sh.Run(context.Background(), "sleep forever")
		runErr <- err
	}()

	// Give Run time to block on the read.
	time.Sleep(50 * time.Millisecond)

	closeDone := make(chan error, 1)
	go func() { closeDone <- sh.Close() }()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked")
	}

	select {
	case err := <-runErr:
		if err == nil {
			t.Error("Run returned nil error after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not unblock after Close")
	}
}

// Compile-time guard that fakeRuntime satisfies container.Runtime.
var _ container.Runtime = (*fakeRuntime)(nil)
