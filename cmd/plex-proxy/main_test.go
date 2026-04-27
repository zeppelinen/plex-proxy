package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelpCommands(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		var out bytes.Buffer
		withOutput(&out, func() {
			if err := run(args); err != nil {
				t.Fatalf("run(%v): %v", args, err)
			}
		})
		got := out.String()
		for _, want := range []string{"Usage: plex-proxy <command>", "serve", "config validate", ".config/plex-proxy/config.yaml"} {
			if !strings.Contains(got, want) {
				t.Fatalf("help output for %v missing %q:\n%s", args, want, got)
			}
		}
	}
}

func TestServeHelpFlag(t *testing.T) {
	var out bytes.Buffer
	withOutput(&out, func() {
		err := run([]string{"serve", "--help"})
		if err != flag.ErrHelp {
			t.Fatalf("err = %v", err)
		}
	})
	got := out.String()
	for _, want := range []string{"Usage: plex-proxy serve", "-config", ".config/plex-proxy/config.yaml"} {
		if !strings.Contains(got, want) {
			t.Fatalf("serve help missing %q:\n%s", want, got)
		}
	}
}

func TestValidateUsesDefaultConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "plex-proxy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`
ssh:
  target: default-host
plex:
  remote_host: plex.default
  server_name: Default Plex
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	withOutput(&out, func() {
		if err := run([]string{"config", "validate"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out.String(), "config ok") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestConfigLocationUsesDefaultConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := configLocation("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "plex-proxy", "config.yaml")
	if got != want {
		t.Fatalf("location = %q, want %q", got, want)
	}
}

func TestConfigLocationAbsolutePath(t *testing.T) {
	got, err := configLocation("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("location is not absolute: %q", got)
	}
	if filepath.Base(got) != "config.yaml" {
		t.Fatalf("location = %q", got)
	}
}

func TestNewLoggerCanLogConfigFile(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger("text", &buf)
	logger.Info("loading config", "config_file", "/etc/plex-proxy/config.yaml")
	got := buf.String()
	if !strings.Contains(got, "loading config") || !strings.Contains(got, "config_file=/etc/plex-proxy/config.yaml") {
		t.Fatalf("log = %q", got)
	}
}

func withOutput(out *bytes.Buffer, fn func()) {
	oldStdout := stdout
	oldStderr := stderr
	stdout = out
	stderr = out
	defer func() {
		stdout = oldStdout
		stderr = oldStderr
	}()
	fn()
}
