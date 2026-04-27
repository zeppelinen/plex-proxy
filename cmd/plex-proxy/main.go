package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/zeppelinen/plex-proxy/internal/config"
	"github.com/zeppelinen/plex-proxy/internal/service"
	"github.com/zeppelinen/plex-proxy/internal/version"
)

var stdout io.Writer = os.Stdout
var stderr io.Writer = os.Stderr

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return serve(args)
	}
	switch args[0] {
	case "help", "-h", "--help":
		printHelp(stdout)
		return nil
	case "serve":
		return serve(args[1:])
	case "config":
		if len(args) > 1 && isHelpArg(args[1]) {
			printConfigHelp(stdout)
			return nil
		}
		if len(args) > 1 && args[1] == "validate" {
			return validate(args[2:])
		}
		return fmt.Errorf("usage: plex-proxy config validate -config config.yaml")
	case "version":
		fmt.Fprintf(stdout, "plex-proxy %s commit=%s date=%s\n", version.Version, version.Commit, version.Date)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stdout)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: plex-proxy serve [-config path]\n\n")
		fmt.Fprintf(fs.Output(), "Runs the Plex proxy. If -config is omitted, plex-proxy tries %s.\n\n", defaultConfigPathForHelp())
		fs.PrintDefaults()
	}
	configPath := fs.String("config", "", "path to YAML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	configSource, err := configLocation(*configPath)
	if err != nil {
		return err
	}
	bootstrapLogger := newLogger(config.Defaults().LogFormat, stdout)
	bootstrapLogger.Info("loading config", "config_file", configSource)
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogFormat, stdout)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return (&service.App{Config: cfg, Log: logger}).Run(ctx)
}

func newLogger(format string, out io.Writer) *slog.Logger {
	if format == "json" {
		return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func configLocation(path string) (string, error) {
	if path == "" {
		defaultPath, err := config.DefaultConfigFile()
		if err == nil {
			return defaultPath, nil
		}
		return "defaults and environment", nil
	}
	return filepath.Abs(path)
}

func validate(args []string) error {
	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	fs.SetOutput(stdout)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: plex-proxy config validate [-config path]\n\n")
		fmt.Fprintf(fs.Output(), "Validates configuration. If -config is omitted, plex-proxy tries %s.\n\n", defaultConfigPathForHelp())
		fs.PrintDefaults()
	}
	configPath := fs.String("config", "", "path to YAML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := config.Load(*configPath); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "config ok")
	return nil
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage: plex-proxy <command> [options]

Commands:
  serve              Run the Plex proxy
  config validate    Validate configuration
  version            Print version information
  help               Show this help

Default config path:
  %s

Examples:
  plex-proxy serve
  plex-proxy serve -config /etc/plex-proxy/config.yaml
  plex-proxy config validate
`, defaultConfigPathForHelp())
}

func printConfigHelp(w io.Writer) {
	fmt.Fprintf(w, `Usage: plex-proxy config <command> [options]

Commands:
  validate    Validate configuration

Default config path:
  %s
`, defaultConfigPathForHelp())
}

func isHelpArg(arg string) bool {
	return arg == "help" || arg == "-h" || arg == "--help"
}

func defaultConfigPathForHelp() string {
	path, err := config.DefaultConfigFile()
	if err != nil {
		return "$HOME/.config/plex-proxy/config.yaml"
	}
	return path
}
