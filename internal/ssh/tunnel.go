package ssh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strconv"
	"sync/atomic"
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
	return cmd.Run()
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
	for {
		localAddr, err := reserveAddr(s.Config.LocalListen)
		if err != nil {
			return err
		}
		s.current.Store(localAddr)
		args := BuildArgs(s.Config, localAddr)
		if s.Logger != nil {
			s.Logger.Info("starting ssh tunnel", "local", localAddr, "remote", s.Config.RemoteAddr)
		}
		err = s.Runner.Run(ctx, "ssh", args...)
		if s.Ready != nil {
			s.Ready(false)
		}
		if ctx.Err() != nil {
			return nil
		}
		if s.Logger != nil {
			s.Logger.Warn("ssh tunnel exited", "error", err, "backoff", backoff)
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
	if port != "0" {
		return addr, nil
	}
	ln, err := net.Listen("tcp", addr)
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
