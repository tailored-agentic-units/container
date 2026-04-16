package container_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tailored-agentic-units/container"
)

// Compile-time assertion that Runtime declares ExecStream with the expected
// signature. The inline interface is satisfied only if Runtime's method set
// contains ExecStream(ctx, id, ExecStreamOptions) (*ExecSession, error).
var _ interface {
	ExecStream(ctx context.Context, id string, opts container.ExecStreamOptions) (*container.ExecSession, error)
} = container.Runtime(nil)

func TestExecStreamOptions_ZeroValue(t *testing.T) {
	var o container.ExecStreamOptions

	if len(o.Cmd) != 0 {
		t.Errorf("Cmd: got %v, want empty", o.Cmd)
	}
	if len(o.Env) != 0 {
		t.Errorf("Env: got %v, want empty", o.Env)
	}
	if o.WorkingDir != "" {
		t.Errorf("WorkingDir: got %q, want empty", o.WorkingDir)
	}
	if o.Tty {
		t.Error("Tty: got true, want false")
	}
}

func TestExecSession_ZeroValue_CloseIsNoOp(t *testing.T) {
	var s container.ExecSession
	if err := s.Close(); err != nil {
		t.Errorf("Close on zero value: got %v, want nil", err)
	}
}

func TestExecSession_ZeroValue_WaitReturnsError(t *testing.T) {
	var s container.ExecSession
	code, err := s.Wait()
	if err == nil {
		t.Fatal("Wait on zero value: expected error, got nil")
	}
	if code != 0 {
		t.Errorf("exit code: got %d, want 0", code)
	}
}

func TestExecSession_ZeroValue_NilStreams(t *testing.T) {
	var s container.ExecSession
	if s.Stdin != nil {
		t.Error("Stdin: got non-nil, want nil")
	}
	if s.Stdout != nil {
		t.Error("Stdout: got non-nil, want nil")
	}
	if s.Stderr != nil {
		t.Error("Stderr: got non-nil, want nil")
	}
}

func TestExecSession_CallbacksInvoked(t *testing.T) {
	wantCode := 42
	wantCloseErr := errors.New("close err sentinel")

	s := container.ExecSession{
		WaitFn:  func() (int, error) { return wantCode, nil },
		CloseFn: func() error { return wantCloseErr },
	}

	code, err := s.Wait()
	if err != nil {
		t.Errorf("Wait: unexpected err %v", err)
	}
	if code != wantCode {
		t.Errorf("Wait code: got %d, want %d", code, wantCode)
	}

	if err := s.Close(); !errors.Is(err, wantCloseErr) {
		t.Errorf("Close: got %v, want %v", err, wantCloseErr)
	}
}
