package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCoreWSWriteTimeoutFromYAML(t *testing.T) {
	path := writeTestConfig(t, "core_connections:\n  ws:\n    write_timeout: 17s\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CoreConnections.WS.WriteTimeout != 17*time.Second {
		t.Fatalf("expected core_connections.ws.write_timeout=17s, got %s", cfg.CoreConnections.WS.WriteTimeout)
	}
}

func TestLoadCoreWSWriteTimeoutFromEnvOverride(t *testing.T) {
	t.Setenv("TRADER_CORE_CONNECTIONS_WS_WRITE_TIMEOUT", "9s")
	path := writeTestConfig(t, "core_connections:\n  ws:\n    write_timeout: 17s\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CoreConnections.WS.WriteTimeout != 9*time.Second {
		t.Fatalf("expected env override core_connections.ws.write_timeout=9s, got %s", cfg.CoreConnections.WS.WriteTimeout)
	}
}

func TestLoadRejectsCoreWSWriteTimeoutAboveMax(t *testing.T) {
	path := writeTestConfig(t, "core_connections:\n  ws:\n    write_timeout: 25h\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected Load() to fail for write_timeout > 24h")
	}
	if !strings.Contains(err.Error(), "invalid core_connections.ws.write_timeout") {
		t.Fatalf("expected write timeout validation error, got %v", err)
	}
}

func TestLoadRejectsCoreWSWriteTimeoutFromEnvWhenInvalid(t *testing.T) {
	t.Setenv("TRADER_CORE_CONNECTIONS_WS_WRITE_TIMEOUT", "0s")
	path := writeTestConfig(t, "core_connections:\n  ws:\n    write_timeout: 17s\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected Load() to fail for env write_timeout=0s")
	}
	if !strings.Contains(err.Error(), "invalid core_connections.ws.write_timeout") {
		t.Fatalf("expected write timeout validation error, got %v", err)
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
