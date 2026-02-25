package store_test

import (
	"context"
	"fmt"
	"net"
	"testing"
)

// isDockerAvailable checks whether the Docker daemon is reachable by attempting
// a brief TCP connection to the Docker socket or default Docker host.
func isDockerAvailable(ctx context.Context) bool {
	d := &net.Dialer{}
	// Try Unix socket (Linux/macOS).
	conn, err := d.DialContext(ctx, "unix", "/var/run/docker.sock")
	if err == nil {
		conn.Close()
		return true
	}
	// Try default TCP host (Docker Desktop on some systems).
	conn, err = d.DialContext(ctx, "tcp", "localhost:2375")
	if err == nil {
		conn.Close()
		return true
	}
	return false
}

// startPostgresContainer starts a PostgreSQL Docker container for testing.
// Returns the DSN string for connecting to it, or an error if startup fails.
// Set TEST_DATABASE_URL to avoid needing Docker.
func startPostgresContainer(t *testing.T) (string, error) {
	t.Helper()
	// Full testcontainers-go integration would go here once the dependency is added:
	//   go get github.com/testcontainers/testcontainers-go
	//   go get github.com/testcontainers/testcontainers-go/modules/postgres
	//
	// Until then, integration tests require TEST_DATABASE_URL to be set.
	return "", fmt.Errorf("testcontainers not configured; set TEST_DATABASE_URL to run integration tests")
}
