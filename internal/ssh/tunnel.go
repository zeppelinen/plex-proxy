package ssh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type Config struct {
	Target            string
	ConfigFile        string
	IdentityFile      string
	LocalListen       string
	RemoteAddr        string
	ExtraArgs         []string
	ConnectTimeout    time.Duration
	RestartMinBackoff time.Duration
	RestartMaxBackoff time.Duration
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &CommandError{Name: name, Output: string(output), Err: err}
	}
	return nil
}

type CommandError struct {
	Name   string
	Output string
	Err    error
}

func (e *CommandError) Error() string {
	if output := strings.TrimSpace(e.Output); output != "" {
		return fmt.Sprintf("%s failed: %v: %s", e.Name, e.Err, output)
	}
	return fmt.Sprintf("%s failed: %v", e.Name, e.Err)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

type Supervisor struct {
	Config  Config
	Runner  CommandRunner
	Logger  *slog.Logger
	Ready   func(bool)
	current atomic.Value
}

func (s *Supervisor) LocalAddr() string {
	if value := s.current.Load(); value != nil {
		if addr, ok := value.(string); ok && addr != "" {
			return addr
		}
	}
	return s.Config.LocalListen
}

func (s *Supervisor) Run(ctx context.Context) error {
	if s.Runner == nil {
		s.Runner = ExecRunner{}
	}
	backoff := s.Config.RestartMinBackoff
	if backoff <= 0 {
		backoff = time.Second
	}
	maxBackoff := s.Config.RestartMaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}
	runConfig := s.Config
	configFile, cleanup, err := configFileForRun(runConfig.ConfigFile)
	if err != nil {
		return err
	}
	defer cleanup()
	runConfig.ConfigFile = configFile
	for {
		localAddr, err := reserveAddr(runConfig.LocalListen)
		if err != nil {
			return err
		}
		s.current.Store(localAddr)
		args := BuildArgs(runConfig, localAddr)
		if s.Logger != nil {
			s.Logger.Info("starting ssh tunnel", "local", localAddr, "remote", runConfig.RemoteAddr)
		}
		err = s.Runner.Run(ctx, "ssh", args...)
		if s.Ready != nil {
			s.Ready(false)
		}
		if ctx.Err() != nil {
			return nil
		}
		if s.Logger != nil {
			s.logExit(err, backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (s *Supervisor) logExit(err error, backoff time.Duration) {
	attrs := []any{"error", err, "backoff", backoff}
	if code, ok := ExitCode(err); ok {
		attrs = append(attrs, "exit_code", code)
		if code == 255 {
			attrs = append(attrs, "ssh_hint", "OpenSSH returned 255; check ssh.target, authentication, ssh config, and network connectivity")
		}
	}
	s.Logger.Warn("ssh tunnel exited", attrs...)
}

func ExitCode(err error) (int, bool) {
	var exitCoder interface {
		ExitCode() int
	}
	if errors.As(err, &exitCoder) {
		return exitCoder.ExitCode(), true
	}
	return 0, false
}

func configFileForRun(path string) (string, func(), error) {
	if path == "" {
		defaultPath, err := defaultSSHConfigFile()
		if err != nil {
			return "", func() {}, nil
		}
		path = defaultPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", func() {}, nil
		}
		return "", nil, err
	}
	filtered := stripForwardingDirectives(string(data))
	tmp, err := os.CreateTemp(filepath.Dir(path), ".plex-proxy-ssh-config-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.Remove(tmp.Name())
	}
	if _, err := tmp.WriteString(filtered); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return tmp.Name(), cleanup, nil
}

func defaultSSHConfigFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

func stripForwardingDirectives(data string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(data, "\n") {
		if isConfigForwardingDirective(line) {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

func isConfigForwardingDirective(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	keyword := strings.ToLower(fields[0])
	if before, _, ok := strings.Cut(keyword, "="); ok {
		keyword = before
	}
	switch keyword {
	case "localforward", "remoteforward", "dynamicforward":
		return true
	default:
		return false
	}
}

func BuildArgs(c Config, localAddr string) []string {
	args := []string{"-N", "-L", localAddr + ":" + c.RemoteAddr, "-o", "ExitOnForwardFailure=yes"}
	if c.ConnectTimeout > 0 {
		args = append(args, "-o", "ConnectTimeout="+strconv.Itoa(int(c.ConnectTimeout.Seconds())))
	}
	if c.ConfigFile != "" {
		args = append(args, "-F", c.ConfigFile)
	}
	if c.IdentityFile != "" {
		args = append(args, "-i", c.IdentityFile)
	}
	args = append(args, c.ExtraArgs...)
	args = append(args, c.Target)
	return args
}

func reserveAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if port == "0" {
		return reserveAnyPort(host)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return reserveAnyPort(host)
		}
		return "", err
	}
	defer ln.Close()
	return addr, nil
}

func reserveAnyPort(host string) (string, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return "", err
	}
	defer ln.Close()
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return "", errors.New("reserved listener is not tcp")
	}
	return net.JoinHostPort(host, fmt.Sprint(tcp.Port)), nil
}
