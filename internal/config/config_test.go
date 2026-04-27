package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
ssh:
  target: user@example.com
plex:
  remote_host: 127.0.0.1
  server_name: Test Plex
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Plex.RemotePort != 32400 {
		t.Fatalf("remote port = %d", cfg.Plex.RemotePort)
	}
	if cfg.Proxy.Listen != DefaultHTTPListen {
		t.Fatalf("listen = %s", cfg.Proxy.Listen)
	}
	if !cfg.GDM.Enabled {
		t.Fatal("gdm should default enabled")
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("PLEX_PROXY_SSH_TARGET", "env-host")
	t.Setenv("PLEX_PROXY_REMOTE_HOST", "plex.local")
	t.Setenv("PLEX_PROXY_SERVER_NAME", "Env Plex")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SSH.Target != "env-host" || cfg.Plex.RemoteHost != "plex.local" || cfg.Plex.ServerName != "Env Plex" {
		t.Fatalf("env overrides not applied: %+v", cfg)
	}
}

func TestValidateRequiresFields(t *testing.T) {
	cfg := Defaults()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
}
