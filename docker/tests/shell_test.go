package docker_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/tailored-agentic-units/container"
)

// alpineShell is the default shell path used by integration tests. Alpine's
// base image provides busybox ash at /bin/sh; /bin/bash is not installed.
const alpineShell = "/bin/sh"

func newShell(t *testing.T) *container.Shell {
	t.Helper()
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sh, err := container.NewShell(context.Background(), rt, c.ID, container.ShellOptions{
		ShellPath: alpineShell,
	})
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}
	t.Cleanup(func() { _ = sh.Close() })
	return sh
}

func TestShell_Run_BasicEcho(t *testing.T) {
	sh := newShell(t)

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

func TestShell_Run_ExitCodeNonZero(t *testing.T) {
	sh := newShell(t)

	_, code, err := sh.Run(context.Background(), "false")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code: got %d, want 1", code)
	}
}

func TestShell_Run_CwdPersists(t *testing.T) {
	sh := newShell(t)

	if _, _, err := sh.Run(context.Background(), "cd /tmp"); err != nil {
		t.Fatalf("Run cd: %v", err)
	}

	out, code, err := sh.Run(context.Background(), "pwd")
	if err != nil {
		t.Fatalf("Run pwd: %v", err)
	}
	if code != 0 {
		t.Errorf("pwd exit code: got %d, want 0", code)
	}
	if string(out) != "/tmp\n" {
		t.Errorf("pwd output: got %q, want %q", out, "/tmp\n")
	}
}

func TestShell_Run_EnvPersists(t *testing.T) {
	sh := newShell(t)

	if _, _, err := sh.Run(context.Background(), "export FOO=bar"); err != nil {
		t.Fatalf("Run export: %v", err)
	}

	out, code, err := sh.Run(context.Background(), "echo $FOO")
	if err != nil {
		t.Fatalf("Run echo: %v", err)
	}
	if code != 0 {
		t.Errorf("echo exit code: got %d, want 0", code)
	}
	if string(out) != "bar\n" {
		t.Errorf("echo output: got %q, want %q", out, "bar\n")
	}
}

func TestShell_Run_StderrMergedOntoStdout(t *testing.T) {
	sh := newShell(t)

	// PTY merges stderr onto stdout. Whatever `echo err 1>&2` writes
	// should appear in the Run's output byte slice.
	out, code, err := sh.Run(context.Background(), "echo err 1>&2")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(string(out), "err") {
		t.Errorf("stderr not merged into output: got %q", out)
	}
}

func TestShell_Close_Idempotent(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sh, err := container.NewShell(context.Background(), rt, c.ID, container.ShellOptions{
		ShellPath: alpineShell,
	})
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}

	if err := sh.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := sh.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestShell_Close_RunAfterClose_ReturnsError(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sh, err := container.NewShell(context.Background(), rt, c.ID, container.ShellOptions{
		ShellPath: alpineShell,
	})
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}

	if err := sh.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, _, err := sh.Run(context.Background(), "echo hi"); err == nil {
		t.Error("Run after Close: expected error, got nil")
	}
}

func TestShell_CustomWorkingDir(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sh, err := container.NewShell(context.Background(), rt, c.ID, container.ShellOptions{
		ShellPath:  alpineShell,
		WorkingDir: "/tmp",
	})
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}
	t.Cleanup(func() { _ = sh.Close() })

	out, _, err := sh.Run(context.Background(), "pwd")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "/tmp\n" {
		t.Errorf("pwd: got %q, want %q", out, "/tmp\n")
	}
}

func TestShell_CustomEnv(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sh, err := container.NewShell(context.Background(), rt, c.ID, container.ShellOptions{
		ShellPath: alpineShell,
		Env:       map[string]string{"GREETING": "howdy"},
	})
	if err != nil {
		t.Fatalf("NewShell: %v", err)
	}
	t.Cleanup(func() { _ = sh.Close() })

	out, _, err := sh.Run(context.Background(), "echo $GREETING")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "howdy\n" {
		t.Errorf("output: got %q, want %q", out, "howdy\n")
	}
}

func TestShell_Concurrent_IndependentShells(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sh1, err := container.NewShell(context.Background(), rt, c.ID, container.ShellOptions{
		ShellPath: alpineShell,
	})
	if err != nil {
		t.Fatalf("NewShell 1: %v", err)
	}
	t.Cleanup(func() { _ = sh1.Close() })

	sh2, err := container.NewShell(context.Background(), rt, c.ID, container.ShellOptions{
		ShellPath: alpineShell,
	})
	if err != nil {
		t.Fatalf("NewShell 2: %v", err)
	}
	t.Cleanup(func() { _ = sh2.Close() })

	// Each shell sets a distinct value for the same variable name.
	if _, _, err := sh1.Run(context.Background(), "export WHICH=shell-one"); err != nil {
		t.Fatalf("sh1 export: %v", err)
	}
	if _, _, err := sh2.Run(context.Background(), "export WHICH=shell-two"); err != nil {
		t.Fatalf("sh2 export: %v", err)
	}

	// Drive both shells in parallel to surface any shared-state bugs.
	var wg sync.WaitGroup
	var out1, out2 []byte
	var err1, err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		out1, _, err1 = sh1.Run(context.Background(), "echo $WHICH")
	}()
	go func() {
		defer wg.Done()
		out2, _, err2 = sh2.Run(context.Background(), "echo $WHICH")
	}()
	wg.Wait()

	if err1 != nil {
		t.Errorf("sh1.Run: %v", err1)
	}
	if err2 != nil {
		t.Errorf("sh2.Run: %v", err2)
	}
	if string(out1) != "shell-one\n" {
		t.Errorf("sh1 output: got %q, want %q", out1, "shell-one\n")
	}
	if string(out2) != "shell-two\n" {
		t.Errorf("sh2 output: got %q, want %q", out2, "shell-two\n")
	}
}
