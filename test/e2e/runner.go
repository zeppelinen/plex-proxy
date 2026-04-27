//go:build e2ehelper

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	deadline := time.Now().Add(90 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if err := checkHTTP(); err != nil {
			last = err
			time.Sleep(time.Second)
			continue
		}
		if err := checkGDM(); err != nil {
			last = err
			time.Sleep(time.Second)
			continue
		}
		return
	}
	fmt.Fprintln(os.Stderr, last)
	os.Exit(1)
}

func checkHTTP() error {
	resp, err := http.Get("http://proxy:32400/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	if body["ok"] != "true" {
		return fmt.Errorf("unexpected body: %v", body)
	}
	if body["host"] != "plex:32400" {
		return fmt.Errorf("unexpected host: %v", body)
	}
	if body["x-forwarded-proto"] != "http" {
		return fmt.Errorf("missing forwarded proto: %v", body)
	}
	return nil
}

func checkGDM() error {
	addr, err := net.ResolveUDPAddr("udp4", "proxy:32410")
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("M-SEARCH * HTTP/1.1\r\n\r\n")); err != nil {
		return err
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	resp := string(buf[:n])
	for _, want := range []string{"Content-Type: plex/media-server", "Name: E2E Plex", "Port: 32400", "Host: proxy"} {
		if !strings.Contains(resp, want) {
			return fmt.Errorf("gdm response missing %q: %s", want, resp)
		}
	}
	return nil
}
