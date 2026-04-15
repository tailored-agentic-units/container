package docker_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/tailored-agentic-units/container"
)

func startSleeper(t *testing.T, rt container.Runtime) *container.Container {
	t.Helper()
	c := createSleeper(t, rt, nil)
	if err := rt.Start(context.Background(), c.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return c
}

func TestExec_StdoutCapture(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	res, err := rt.Exec(context.Background(), c.ID, container.ExecOptions{
		Cmd:          []string{"echo", "hello"},
		AttachStdout: true,
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", res.ExitCode)
	}
	if got := string(res.Stdout); got != "hello\n" {
		t.Errorf("Stdout: got %q, want %q", got, "hello\n")
	}
	if res.Stderr != nil {
		t.Errorf("Stderr: got %v, want nil", res.Stderr)
	}
}

func TestExec_StderrCapture(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	res, err := rt.Exec(context.Background(), c.ID, container.ExecOptions{
		Cmd:          []string{"sh", "-c", "echo oops 1>&2"},
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", res.ExitCode)
	}
	if got := string(res.Stderr); got != "oops\n" {
		t.Errorf("Stderr: got %q, want %q", got, "oops\n")
	}
	if res.Stdout != nil {
		t.Errorf("Stdout: got %v, want nil", res.Stdout)
	}
}

func TestExec_NoAttach_NilBuffers(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	res, err := rt.Exec(context.Background(), c.ID, container.ExecOptions{
		Cmd: []string{"true"},
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", res.ExitCode)
	}
	if res.Stdout != nil {
		t.Errorf("Stdout: got %v, want nil", res.Stdout)
	}
	if res.Stderr != nil {
		t.Errorf("Stderr: got %v, want nil", res.Stderr)
	}
}

func TestExec_NonZeroExit(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	res, err := rt.Exec(context.Background(), c.ID, container.ExecOptions{
		Cmd: []string{"sh", "-c", "exit 7"},
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("ExitCode: got %d, want 7", res.ExitCode)
	}
}

func TestExec_CtxCancel(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := rt.Exec(ctx, c.ID, container.ExecOptions{
		Cmd:          []string{"sleep", "30"},
		AttachStdout: true,
	})
	if err == nil {
		t.Fatal("Exec with cancelled ctx: expected error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("err is not ctx.Err: %v", err)
	}
}

func TestCopyTo_RoundTrip(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	const payload = "hi\n"
	if err := rt.CopyTo(context.Background(), c.ID, "/workspace/hello.txt", strings.NewReader(payload)); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	rc, err := rt.CopyFrom(context.Background(), c.ID, "/workspace/hello.txt")
	if err != nil {
		t.Fatalf("CopyFrom: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, []byte(payload)) {
		t.Errorf("round-trip: got %q, want %q", got, payload)
	}
}

func TestCopyTo_NestedPath(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	const payload = "deep\n"
	if err := rt.CopyTo(context.Background(), c.ID, "/a/b/c/file.txt", strings.NewReader(payload)); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	rc, err := rt.CopyFrom(context.Background(), c.ID, "/a/b/c/file.txt")
	if err != nil {
		t.Fatalf("CopyFrom: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, []byte(payload)) {
		t.Errorf("round-trip: got %q, want %q", got, payload)
	}
}

func TestCopyFrom_AbsentFile(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	_, err := rt.CopyFrom(context.Background(), c.ID, "/does/not/exist")
	if err == nil {
		t.Fatal("CopyFrom on absent file: expected error, got nil")
	}
	if !cerrdefs.IsNotFound(err) {
		t.Errorf("err is not NotFound: %v", err)
	}
}
