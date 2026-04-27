package gdm

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

const RequestPrefix = "M-SEARCH * HTTP/1.1"

type Advertisement struct {
	Host      string
	Port      int
	Name      string
	MachineID string
	Version   string
}

func (a Advertisement) Response(now time.Time) []byte {
	machineID := a.MachineID
	if machineID == "" {
		machineID = "plex-proxy"
	}
	version := a.Version
	if version == "" {
		version = "1.0.0"
	}
	lines := []string{
		"HTTP/1.0 200 OK",
		"Content-Type: plex/media-server",
		"Resource-Identifier: " + machineID,
		"Name: " + a.Name,
		"Port: " + fmt.Sprint(a.Port),
		"Updated-At: " + fmt.Sprint(now.Unix()),
		"Version: " + version,
		"Host: " + a.Host,
		"",
		"",
	}
	return []byte(strings.Join(lines, "\r\n"))
}

type Server struct {
	Ports []int
	Ad    Advertisement
	Log   *slog.Logger
}

func (s Server) Serve(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(s.Ports))
	for _, port := range s.Ports {
		port := port
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.servePort(ctx, port); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s Server) servePort(ctx context.Context, port int) error {
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: port}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	buf := make([]byte, 2048)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if !IsDiscovery(buf[:n]) {
			continue
		}
		if s.Log != nil {
			s.Log.Debug("gdm discovery request", "remote", remote.String(), "port", port)
		}
		_, _ = conn.WriteToUDP(s.Ad.Response(time.Now()), remote)
	}
}

func IsDiscovery(packet []byte) bool {
	packet = bytes.TrimSpace(packet)
	if len(packet) == 0 {
		return false
	}
	upper := strings.ToUpper(string(packet))
	return strings.HasPrefix(upper, RequestPrefix) || strings.Contains(upper, "MAN: \"SSDP:DISCOVER\"")
}
