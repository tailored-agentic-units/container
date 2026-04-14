package container_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tailored-agentic-units/container"
)

type stubRuntime struct {
	id string
}

func (s *stubRuntime) Create(ctx context.Context, opts container.CreateOptions) (*container.Container, error) {
	return nil, nil
}
func (s *stubRuntime) Start(ctx context.Context, id string) error { return nil }
func (s *stubRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	return nil
}
func (s *stubRuntime) Remove(ctx context.Context, id string, force bool) error { return nil }
func (s *stubRuntime) Exec(ctx context.Context, id string, opts container.ExecOptions) (*container.ExecResult, error) {
	return nil, nil
}
func (s *stubRuntime) CopyTo(ctx context.Context, id string, dst string, content io.Reader) error {
	return nil
}
func (s *stubRuntime) CopyFrom(ctx context.Context, id string, src string) (io.ReadCloser, error) {
	return nil, nil
}
func (s *stubRuntime) Inspect(ctx context.Context, id string) (*container.ContainerInfo, error) {
	return nil, nil
}

func uniqueName(t *testing.T, suffix string) string {
	t.Helper()
	return fmt.Sprintf("test-%s-%s", strings.ReplaceAll(t.Name(), "/", "_"), suffix)
}

func TestRegister_AndCreate(t *testing.T) {
	name := uniqueName(t, "ok")
	called := false
	want := &stubRuntime{id: "want"}

	container.Register(name, func() (container.Runtime, error) {
		called = true
		return want, nil
	})

	got, err := container.Create(name)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !called {
		t.Error("factory was not invoked")
	}
	if got != want {
		t.Errorf("Create returned %v, want %v", got, want)
	}
}

func TestCreate_UnknownName(t *testing.T) {
	name := uniqueName(t, "missing")

	got, err := container.Create(name)
	if err == nil {
		t.Fatal("Create returned nil error for unknown name")
	}
	if got != nil {
		t.Errorf("Create returned non-nil Runtime for unknown name: %v", got)
	}
	if !errors.Is(err, container.ErrRuntimeNotFound) {
		t.Errorf("err is not ErrRuntimeNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), name) {
		t.Errorf("err message %q does not contain requested name %q", err.Error(), name)
	}
}

func TestRegister_Overwrite(t *testing.T) {
	name := uniqueName(t, "overwrite")
	first := &stubRuntime{id: "first"}
	second := &stubRuntime{id: "second"}

	container.Register(name, func() (container.Runtime, error) { return first, nil })
	container.Register(name, func() (container.Runtime, error) { return second, nil })

	got, err := container.Create(name)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got != second {
		t.Errorf("Create returned %v, want second factory's value %v", got, second)
	}
}

func TestListRuntimes(t *testing.T) {
	names := []string{
		uniqueName(t, "a"),
		uniqueName(t, "b"),
		uniqueName(t, "c"),
	}
	for _, n := range names {
		container.Register(n, func() (container.Runtime, error) { return &stubRuntime{}, nil })
	}

	listed := container.ListRuntimes()
	listedSet := make(map[string]bool, len(listed))
	for _, n := range listed {
		listedSet[n] = true
	}

	for _, want := range names {
		if !listedSet[want] {
			t.Errorf("ListRuntimes missing %q (got %v)", want, listed)
		}
	}
}

func TestFactory_Error(t *testing.T) {
	name := uniqueName(t, "factory-err")
	factoryErr := errors.New("boom")

	container.Register(name, func() (container.Runtime, error) {
		return nil, factoryErr
	})

	got, err := container.Create(name)
	if got != nil {
		t.Errorf("Create returned non-nil Runtime when factory failed: %v", got)
	}
	if !errors.Is(err, factoryErr) {
		t.Errorf("Create did not propagate factory error: got %v, want %v", err, factoryErr)
	}
}

func TestRegistry_Concurrent(t *testing.T) {
	const workers = 32
	const opsPerWorker = 64

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for op := range opsPerWorker {
				name := fmt.Sprintf("test-concurrent-%d-%d", w, op)
				container.Register(name, func() (container.Runtime, error) {
					return &stubRuntime{id: name}, nil
				})
				if _, err := container.Create(name); err != nil {
					t.Errorf("Create(%q) returned error: %v", name, err)
				}
				_ = container.ListRuntimes()
			}
		})
	}
	wg.Wait()
}
