package docker_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/tailored-agentic-units/container"
)

func TestExecStream_StdinRoundTrip(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sess, err := rt.ExecStream(context.Background(), c.ID, container.ExecStreamOptions{
		Cmd: []string{"cat"},
	})
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}
	defer sess.Close()

	if _, err := io.WriteString(sess.Stdin, "hi\n"); err != nil {
		t.Fatalf("Stdin.Write: %v", err)
	}
	if err := sess.Stdin.Close(); err != nil {
		t.Fatalf("Stdin.Close: %v", err)
	}

	got, err := io.ReadAll(sess.Stdout)
	if err != nil {
		t.Fatalf("Stdout read: %v", err)
	}
	if string(got) != "hi\n" {
		t.Errorf("Stdout: got %q, want %q", got, "hi\n")
	}

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("ExitCode: got %d, want 0", code)
	}
}

func TestExecStream_ExitCodeZero(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sess, err := rt.ExecStream(context.Background(), c.ID, container.ExecStreamOptions{
		Cmd: []string{"true"},
	})
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}
	defer sess.Close()

	// Drain streams so the demux goroutine can complete.
	go io.Copy(io.Discard, sess.Stdout)
	go io.Copy(io.Discard, sess.Stderr)

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("ExitCode: got %d, want 0", code)
	}
}

func TestExecStream_ExitCodeNonZero(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sess, err := rt.ExecStream(context.Background(), c.ID, container.ExecStreamOptions{
		Cmd: []string{"sh", "-c", "exit 7"},
	})
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}
	defer sess.Close()

	go io.Copy(io.Discard, sess.Stdout)
	go io.Copy(io.Discard, sess.Stderr)

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 7 {
		t.Errorf("ExitCode: got %d, want 7", code)
	}
}

func TestExecStream_CloseTerminatesLongRunning(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sess, err := rt.ExecStream(context.Background(), c.ID, container.ExecStreamOptions{
		Cmd: []string{"sleep", "30"},
	})
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}

	start := time.Now()
	if err := sess.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Close took too long: %v", elapsed)
	}

	waitDone := make(chan struct{})
	go func() {
		_, _ = sess.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(3 * time.Second):
		t.Error("Wait did not return after Close")
	}
}

func TestExecStream_CloseIdempotent(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sess, err := rt.ExecStream(context.Background(), c.ID, container.ExecStreamOptions{
		Cmd: []string{"sleep", "30"},
	})
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}

	if err := sess.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestExecStream_CtxCancelDuringStart(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sess, err := rt.ExecStream(ctx, c.ID, container.ExecStreamOptions{
		Cmd: []string{"sleep", "30"},
	})
	if err == nil {
		if sess != nil {
			_ = sess.Close()
		}
		t.Fatal("ExecStream with cancelled ctx: expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err does not wrap ctx.Err: %v", err)
	}
}

func TestExecStream_NonTtyDemux(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sess, err := rt.ExecStream(context.Background(), c.ID, container.ExecStreamOptions{
		Cmd: []string{"sh", "-c", "echo out; echo err 1>&2"},
	})
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}
	defer sess.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Go(func() { _, _ = io.Copy(&stdoutBuf, sess.Stdout) })
	wg.Go(func() { _, _ = io.Copy(&stderrBuf, sess.Stderr) })

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("ExitCode: got %d, want 0", code)
	}
	wg.Wait()

	if stdoutBuf.String() != "out\n" {
		t.Errorf("Stdout: got %q, want %q", stdoutBuf.String(), "out\n")
	}
	if stderrBuf.String() != "err\n" {
		t.Errorf("Stderr: got %q, want %q", stderrBuf.String(), "err\n")
	}
}

func TestExecStream_TtyMerged(t *testing.T) {
	rt, _ := newRuntime(t)
	c := startSleeper(t, rt)

	sess, err := rt.ExecStream(context.Background(), c.ID, container.ExecStreamOptions{
		Cmd: []string{"sh", "-c", "echo out; echo err 1>&2"},
		Tty: true,
	})
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}
	defer sess.Close()

	// Stderr yields EOF immediately in TTY mode; a zero-length ReadAll confirms it.
	stderrBytes, err := io.ReadAll(sess.Stderr)
	if err != nil {
		t.Fatalf("Stderr read: %v", err)
	}
	if len(stderrBytes) != 0 {
		t.Errorf("Stderr in TTY mode: got %q, want empty", stderrBytes)
	}

	stdoutBytes, err := io.ReadAll(sess.Stdout)
	if err != nil {
		t.Fatalf("Stdout read: %v", err)
	}

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("ExitCode: got %d, want 0", code)
	}

	if !bytes.Contains(stdoutBytes, []byte("out")) {
		t.Errorf("Stdout missing 'out': %q", stdoutBytes)
	}
	if !bytes.Contains(stdoutBytes, []byte("err")) {
		t.Errorf("Stdout missing 'err': %q", stdoutBytes)
	}
}
