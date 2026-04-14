package container

import (
	"fmt"
	"sync"
)

// Factory constructs a Runtime instance. Factories are parameterless: each
// runtime sub-module closes over its own configuration in a wrapper before
// passing the resulting Factory to Register. This keeps the root module free
// of runtime-specific config types.
type Factory func() (Runtime, error)

type registry struct {
	factories map[string]Factory
	mu        sync.RWMutex
}

var register = &registry{
	factories: make(map[string]Factory),
}

// Register associates name with factory in the package-level runtime
// registry. Re-registering an existing name silently overwrites the previous
// factory (last write wins). Register is safe for concurrent use and is
// expected to be invoked from a sub-module's exported Register helper rather
// than from package init — see the project convention against init()-time
// auto-registration.
func Register(name string, factory Factory) {
	register.mu.Lock()
	defer register.mu.Unlock()
	register.factories[name] = factory
}

// Create looks up the factory previously registered under name and invokes
// it. If no factory is registered, Create returns an error wrapping
// ErrRuntimeNotFound with the requested name; callers can use errors.Is to
// detect the miss. Errors returned by the factory itself propagate
// unwrapped. Create is safe for concurrent use.
func Create(name string) (Runtime, error) {
	register.mu.RLock()
	factory, exists := register.factories[name]
	register.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)
	}

	return factory()
}

// ListRuntimes returns the names of every registered runtime in
// non-deterministic order. The returned slice is a snapshot; subsequent
// Register calls do not mutate it. ListRuntimes is safe for concurrent use.
func ListRuntimes() []string {
	register.mu.RLock()
	defer register.mu.RUnlock()

	names := make([]string, 0, len(register.factories))
	for name := range register.factories {
		names = append(names, name)
	}
	return names
}
