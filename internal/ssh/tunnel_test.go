package ssh

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
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
