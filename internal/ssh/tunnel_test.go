package ssh

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildArgs(t *testing.T) {
	args := BuildArgs(Config{
		Target:         "user@example.com",
		ConfigFile:     "/tmp/ssh_config",
		IdentityFile:   "/tmp/key",
		RemoteAddr:     "127.0.0.1:32400",
		ConnectTimeout: 5 * time.Second,
		ExtraArgs:      []string{"-J", "jump"},
	}, "127.0.0.1:40000")
	want := []string{"-N", "-L", "127.0.0.1:40000:127.0.0.1:32400", "-o", "ExitOnForwardFailure=yes", "-o", "ConnectTimeout=5", "-F", "/tmp/ssh_config", "-i", "/tmp/key", "-J", "jump", "user@example.com"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args\n got=%v\nwant=%v", args, want)
	}
}

type fakeRunner struct {
	calls int
}

func (f *fakeRunner) Run(ctx context.Context, _ string, _ ...string) error {
	f.calls++
	if f.calls == 2 {
		<-ctx.Done()
		return ctx.Err()
	}
	return errors.New("exit")
}

func TestSupervisorRestarts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	runner := &fakeRunner{}
	s := &Supervisor{
		Config: Config{
			Target:            "host",
			LocalListen:       "127.0.0.1:0",
			RemoteAddr:        "127.0.0.1:32400",
			RestartMinBackoff: time.Millisecond,
			RestartMaxBackoff: time.Millisecond,
		},
		Runner: runner,
	}
	if err := s.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if runner.calls < 2 {
		t.Fatalf("calls = %d", runner.calls)
	}
}

type codedExitError struct {
	code int
}

func (e codedExitError) Error() string {
	return "exit"
}

func (e codedExitError) ExitCode() int {
	return e.code
}

func TestExitCodeUnwrapsCommandError(t *testing.T) {
	err := &CommandError{Name: "ssh", Err: codedExitError{code: 255}}
	code, ok := ExitCode(err)
	if !ok {
		t.Fatal("expected exit code")
	}
	if code != 255 {
		t.Fatalf("code = %d", code)
	}
}

func TestCommandErrorIncludesTrimmedOutput(t *testing.T) {
	err := &CommandError{Name: "ssh", Err: codedExitError{code: 255}, Output: "\nPermission denied\n"}
	if got := err.Error(); !strings.Contains(got, "Permission denied") {
		t.Fatalf("error = %q", got)
	}
}

func TestSupervisorLogsSSH255Hint(t *testing.T) {
	var buf bytes.Buffer
	s := &Supervisor{
		Logger: slog.New(slog.NewTextHandler(&buf, nil)),
	}
	s.logExit(&CommandError{Name: "ssh", Err: codedExitError{code: 255}}, time.Second)
	logged := buf.String()
	for _, want := range []string{
		"exit_code=255",
		"ssh_hint=",
		"OpenSSH returned 255",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log missing %q: %s", want, logged)
		}
	}
}

func TestStripForwardingDirectives(t *testing.T) {
	input := strings.Join([]string{
		"Host plex",
		"  HostName plex.example.com",
		"  LocalForward 127.0.0.1:32400 127.0.0.1:32400",
		"  RemoteForward 9000 127.0.0.1:9000",
		"  DynamicForward=127.0.0.1:1080",
		"  IdentityFile ~/.ssh/id_ed25519",
		"  # LocalForward 127.0.0.1:1111 127.0.0.1:2222",
		"",
	}, "\n")
	got := stripForwardingDirectives(input)
	for _, removed := range []string{"LocalForward 127.0.0.1:32400", "RemoteForward 9000", "DynamicForward=127.0.0.1:1080"} {
		if strings.Contains(got, removed) {
			t.Fatalf("config still contains %q:\n%s", removed, got)
		}
	}
	for _, kept := range []string{"Host plex", "HostName plex.example.com", "IdentityFile ~/.ssh/id_ed25519", "# LocalForward 127.0.0.1:1111"} {
		if !strings.Contains(got, kept) {
			t.Fatalf("config missing %q:\n%s", kept, got)
		}
	}
}

func TestConfigFileForRunSanitizesExplicitConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("Host plex\n  HostName plex.example.com\n  LocalForward 1 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, cleanup, err := configFileForRun(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if got == path {
		t.Fatal("expected sanitized temp config")
	}
	if filepath.Dir(got) != dir {
		t.Fatalf("temp config dir = %q, want %q", filepath.Dir(got), dir)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "LocalForward") {
		t.Fatalf("config was not sanitized:\n%s", data)
	}
	if !strings.Contains(string(data), "HostName plex.example.com") {
		t.Fatalf("config lost host settings:\n%s", data)
	}
}

func TestReserveAddrKeepsAvailableFixedPort(t *testing.T) {
	addr, err := reserveAddr("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if _, port, err := net.SplitHostPort(addr); err != nil {
		t.Fatal(err)
	} else if port == "0" {
		t.Fatalf("port was not reserved: %q", addr)
	}

	got, err := reserveAddr(addr)
	if err != nil {
		t.Fatal(err)
	}
	if got != addr {
		t.Fatalf("addr = %q, want %q", got, addr)
	}
}

func TestReserveAddrFallsBackWhenFixedPortOccupied(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	occupied := ln.Addr().String()
	got, err := reserveAddr(occupied)
	if err != nil {
		t.Fatal(err)
	}
	if got == occupied {
		t.Fatalf("addr = %q, want different port", got)
	}
	gotHost, gotPort, err := net.SplitHostPort(got)
	if err != nil {
		t.Fatal(err)
	}
	occupiedHost, occupiedPort, err := net.SplitHostPort(occupied)
	if err != nil {
		t.Fatal(err)
	}
	if gotHost != occupiedHost {
		t.Fatalf("host = %q, want %q", gotHost, occupiedHost)
	}
	if gotPort == occupiedPort {
		t.Fatalf("port = %q, want different from occupied %q", gotPort, occupiedPort)
	}
	if parsed, err := strconv.Atoi(gotPort); err != nil || parsed <= 0 {
		t.Fatalf("port = %q, err = %v", gotPort, err)
	}
}
