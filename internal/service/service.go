package service

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/zeppelinen/plex-proxy/internal/config"
	"github.com/zeppelinen/plex-proxy/internal/gdm"
	"github.com/zeppelinen/plex-proxy/internal/health"
	"github.com/zeppelinen/plex-proxy/internal/netutil"
	"github.com/zeppelinen/plex-proxy/internal/proxy"
	sshtunnel "github.com/zeppelinen/plex-proxy/internal/ssh"
	"github.com/zeppelinen/plex-proxy/internal/tcpforward"
)

type App struct {
	Config config.Config
	Log    *slog.Logger
	Health *health.State
}

func (a *App) Run(ctx context.Context) error {
	if a.Log == nil {
		a.Log = slog.Default()
	}
	if a.Health == nil {
		a.Health = &health.State{}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	tunnel := &sshtunnel.Supervisor{
		Config: sshtunnel.Config{
			Target:            a.Config.SSH.Target,
			ConfigFile:        a.Config.SSH.ConfigFile,
			IdentityFile:      a.Config.SSH.IdentityFile,
			LocalListen:       a.Config.SSH.LocalListen,
			RemoteAddr:        a.Config.RemotePlexAddr(),
			ExtraArgs:         a.Config.SSH.ExtraArgs,
			ConnectTimeout:    a.Config.SSH.ConnectTimeout,
			RestartMinBackoff: a.Config.SSH.RestartMinBackoff,
			RestartMaxBackoff: a.Config.SSH.RestartMaxBackoff,
		},
		Logger: a.Log,
		Ready:  a.Health.SetReady,
	}

	errc := make(chan error, 8)
	go func() { sendErr(errc, tunnel.Run(ctx)) }()
	go func() { sendErr(errc, a.runHTTPProxy(ctx, tunnel)) }()
	go func() { sendErr(errc, a.runHealth(ctx)) }()
	if a.Config.GDM.Enabled {
		go func() { sendErr(errc, a.runGDM(ctx)) }()
	}
	for _, fwd := range a.Config.Forward {
		if !fwd.Enabled {
			continue
		}
		fwd := fwd
		go func() {
			sendErr(errc, tcpforward.Forwarder{
				ListenAddr: fwd.Listen,
				TargetAddr: net.JoinHostPort(targetHost(fwd.TargetHost, a.Config.Plex.RemoteHost), strconv.Itoa(fwd.TargetPort)),
				Logger:     a.Log,
			}.Serve(ctx))
		}()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errc:
		cancel()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (a *App) runHTTPProxy(ctx context.Context, tunnel *sshtunnel.Supervisor) error {
	handler := proxy.NewDynamic(func() *url.URL {
		target, err := proxy.TargetURL(a.Config.Plex.Scheme, tunnel.LocalAddr())
		if err != nil {
			return &url.URL{Scheme: "http", Host: "127.0.0.1:1"}
		}
		return target
	}, a.Config.RemotePlexAddr())
	srv := &http.Server{Addr: a.Config.Proxy.Listen, Handler: handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	a.Log.Info("starting plex proxy", "listen", a.Config.Proxy.Listen)
	return srv.ListenAndServe()
}

func (a *App) runHealth(ctx context.Context) error {
	srv := health.NewServer(a.Config.Health.Listen, a.Health)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	a.Log.Info("starting health server", "listen", a.Config.Health.Listen)
	return srv.ListenAndServe()
}

func (a *App) runGDM(ctx context.Context) error {
	host := a.Config.GDM.AdvertiseHost
	if host == "" {
		found, err := netutil.FirstNonLoopbackIPv4()
		if err != nil {
			return err
		}
		host = found
	}
	_, port, err := net.SplitHostPort(a.Config.Proxy.Listen)
	if err != nil {
		return err
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil {
		return err
	}
	a.Log.Info("starting gdm responder", "host", host, "port", parsedPort)
	return gdm.Server{
		Ports: a.Config.GDM.Ports,
		Ad: gdm.Advertisement{
			Host:      host,
			Port:      parsedPort,
			Name:      a.Config.Plex.ServerName,
			MachineID: a.Config.Plex.MachineID,
			Version:   a.Config.Plex.Version,
		},
		Log: a.Log,
	}.Serve(ctx)
}

func sendErr(errc chan<- error, err error) {
	if err != nil {
		errc <- err
	}
}

func targetHost(candidate string, fallback string) string {
	if candidate != "" {
		return candidate
	}
	return fallback
}
