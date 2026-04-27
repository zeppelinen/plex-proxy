package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultHTTPListen   = "0.0.0.0:32400"
	DefaultHealthListen = "127.0.0.1:8080"
	DefaultRemotePort   = 32400
	DefaultTunnelListen = "127.0.0.1:0"
)

type Config struct {
	SSH       SSHConfig       `yaml:"ssh"`
	Plex      PlexConfig      `yaml:"plex"`
	Proxy     ProxyConfig     `yaml:"proxy"`
	GDM       GDMConfig       `yaml:"gdm"`
	Health    HealthConfig    `yaml:"health"`
	Forward   []ForwardConfig `yaml:"forward"`
	LogFormat string          `yaml:"log_format"`
}

type SSHConfig struct {
	Target            string        `yaml:"target"`
	ConfigFile        string        `yaml:"config_file"`
	IdentityFile      string        `yaml:"identity_file"`
	LocalListen       string        `yaml:"local_listen"`
	ConnectTimeout    time.Duration `yaml:"connect_timeout"`
	RestartMinBackoff time.Duration `yaml:"restart_min_backoff"`
	RestartMaxBackoff time.Duration `yaml:"restart_max_backoff"`
	ExtraArgs         []string      `yaml:"extra_args"`
}

type PlexConfig struct {
	RemoteHost string `yaml:"remote_host"`
	RemotePort int    `yaml:"remote_port"`
	ServerName string `yaml:"server_name"`
	MachineID  string `yaml:"machine_id"`
	Version    string `yaml:"version"`
	Scheme     string `yaml:"scheme"`
}

type ProxyConfig struct {
	Listen string `yaml:"listen"`
}

type GDMConfig struct {
	Enabled       bool   `yaml:"enabled"`
	AdvertiseHost string `yaml:"advertise_host"`
	Ports         []int  `yaml:"ports"`
}

type HealthConfig struct {
	Listen string `yaml:"listen"`
}

type ForwardConfig struct {
	Name         string `yaml:"name"`
	Listen       string `yaml:"listen"`
	TargetHost   string `yaml:"target_host"`
	TargetPort   int    `yaml:"target_port"`
	Enabled      bool   `yaml:"enabled"`
	TunnelListen string `yaml:"tunnel_listen"`
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, err
		}
	}
	ApplyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Defaults() Config {
	return Config{
		SSH: SSHConfig{
			LocalListen:       DefaultTunnelListen,
			ConnectTimeout:    10 * time.Second,
			RestartMinBackoff: time.Second,
			RestartMaxBackoff: 30 * time.Second,
		},
		Plex: PlexConfig{
			RemotePort: DefaultRemotePort,
			Version:    "1.0.0",
			Scheme:     "http",
		},
		Proxy:  ProxyConfig{Listen: DefaultHTTPListen},
		GDM:    GDMConfig{Enabled: true, Ports: []int{32410, 32412, 32413, 32414}},
		Health: HealthConfig{Listen: DefaultHealthListen},
		Forward: []ForwardConfig{
			{Name: "dlna", Listen: "0.0.0.0:32469", TargetPort: 32469, Enabled: false, TunnelListen: DefaultTunnelListen},
		},
		LogFormat: "text",
	}
}

func ApplyEnv(cfg *Config) {
	setString(&cfg.SSH.Target, "PLEX_PROXY_SSH_TARGET")
	setString(&cfg.SSH.ConfigFile, "PLEX_PROXY_SSH_CONFIG_FILE")
	setString(&cfg.SSH.IdentityFile, "PLEX_PROXY_SSH_IDENTITY_FILE")
	setString(&cfg.Plex.RemoteHost, "PLEX_PROXY_REMOTE_HOST")
	setInt(&cfg.Plex.RemotePort, "PLEX_PROXY_REMOTE_PORT")
	setString(&cfg.Plex.ServerName, "PLEX_PROXY_SERVER_NAME")
	setString(&cfg.Plex.MachineID, "PLEX_PROXY_MACHINE_ID")
	setString(&cfg.Proxy.Listen, "PLEX_PROXY_LISTEN")
	setString(&cfg.GDM.AdvertiseHost, "PLEX_PROXY_ADVERTISE_HOST")
	setString(&cfg.Health.Listen, "PLEX_PROXY_HEALTH_LISTEN")
}

func (c Config) Validate() error {
	var errs []error
	if c.SSH.Target == "" {
		errs = append(errs, errors.New("ssh.target is required"))
	}
	if c.Plex.RemoteHost == "" {
		errs = append(errs, errors.New("plex.remote_host is required"))
	}
	if c.Plex.RemotePort <= 0 || c.Plex.RemotePort > 65535 {
		errs = append(errs, errors.New("plex.remote_port must be 1-65535"))
	}
	if c.Plex.ServerName == "" {
		errs = append(errs, errors.New("plex.server_name is required"))
	}
	if c.Plex.Scheme != "http" && c.Plex.Scheme != "https" {
		errs = append(errs, errors.New("plex.scheme must be http or https"))
	}
	for _, addr := range []string{c.Proxy.Listen, c.Health.Listen, c.SSH.LocalListen} {
		if err := validateAddr(addr); err != nil {
			errs = append(errs, err)
		}
	}
	for _, port := range c.GDM.Ports {
		if port <= 0 || port > 65535 {
			errs = append(errs, fmt.Errorf("gdm port %d must be 1-65535", port))
		}
	}
	for _, f := range c.Forward {
		if !f.Enabled {
			continue
		}
		if f.Listen == "" {
			errs = append(errs, fmt.Errorf("forward %q listen is required", f.Name))
		}
		if err := validateAddr(f.Listen); err != nil {
			errs = append(errs, err)
		}
		if f.TargetHost == "" && c.Plex.RemoteHost == "" {
			errs = append(errs, fmt.Errorf("forward %q target_host is required", f.Name))
		}
		if f.TargetPort <= 0 || f.TargetPort > 65535 {
			errs = append(errs, fmt.Errorf("forward %q target_port must be 1-65535", f.Name))
		}
	}
	return errors.Join(errs...)
}

func (c Config) RemotePlexAddr() string {
	return net.JoinHostPort(c.Plex.RemoteHost, strconv.Itoa(c.Plex.RemotePort))
}

func validateAddr(addr string) error {
	if addr == "" {
		return errors.New("listen address is required")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("invalid address %q: %w", addr, err)
	}
	return nil
}

func setString(target *string, key string) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		*target = value
	}
}

func setInt(target *int, key string) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			*target = parsed
		}
	}
}
