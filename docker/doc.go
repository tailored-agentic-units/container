// Package docker implements the container.Runtime interface against the
// Docker Engine API. The sub-module carries its own go.mod so the root
// container module stays free of the Docker client SDK's transitive
// dependencies.
//
// Callers wire the runtime into the root registry by invoking Register once
// from application code (not package init, per project convention), then
// construct instances through container.Create:
//
//	docker.Register()
//	rt, err := container.Create("docker")
//
// The default factory builds a Docker client from the host environment
// (DOCKER_HOST, DOCKER_TLS_VERIFY, and friends) and negotiates the API
// version with the daemon. Containers created through this runtime carry
// the reserved labels tau.managed=true and tau.manifest.version=<v>.
package docker
