package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeppelinen/plex-proxy/internal/config"
	"github.com/zeppelinen/plex-proxy/internal/service"
	"github.com/zeppelinen/plex-proxy/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return serve(args)
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "config":
		if len(args) > 1 && args[1] == "validate" {
			return validate(args[2:])
		}
		return fmt.Errorf("usage: plex-proxy config validate -config config.yaml")
	case "version":
		fmt.Printf("plex-proxy %s commit=%s date=%s\n", version.Version, version.Commit, version.Date)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to YAML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if cfg.LogFormat == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return (&service.App{Config: cfg, Log: logger}).Run(ctx)
}

func validate(args []string) error {
	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to YAML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := config.Load(*configPath); err != nil {
		return err
	}
	fmt.Println("config ok")
	return nil
}
