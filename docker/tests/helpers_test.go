package docker_test

import (
	"context"
	"io"
	"testing"
	"time"

	di "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

const testImage = "alpine:3.21"

func skipIfNoDaemon(t *testing.T) *client.Client {
	t.Helper()
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Skipf("docker client init failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		t.Skipf("docker daemon unreachable: %v", err)
	}
	return cli
}

func ensureImage(t *testing.T, cli *client.Client, ref string) {
	t.Helper()
	if _, err := cli.ImageInspect(context.Background(), ref); err == nil {
		return
	}
	rc, err := cli.ImagePull(context.Background(), ref, di.PullOptions{})
	if err != nil {
		t.Skipf("image pull failed: %v", err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		t.Skipf("image pull drain failed: %v", err)
	}
}
