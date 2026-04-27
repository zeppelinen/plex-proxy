package gdm

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestAdvertisementResponse(t *testing.T) {
	ad := Advertisement{Host: "192.0.2.10", Port: 32400, Name: "Plex", MachineID: "id", Version: "1.2.3"}
	resp := string(ad.Response(time.Unix(10, 0)))
	for _, want := range []string{"Content-Type: plex/media-server", "Resource-Identifier: id", "Name: Plex", "Port: 32400", "Host: 192.0.2.10"} {
		if !strings.Contains(resp, want) {
			t.Fatalf("response missing %q:\n%s", want, resp)
		}
	}
}

func TestIsDiscovery(t *testing.T) {
	if !IsDiscovery([]byte("M-SEARCH * HTTP/1.1\r\n\r\n")) {
		t.Fatal("m-search should be discovery")
	}
	if IsDiscovery([]byte("hello")) {
		t.Fatal("hello should not be discovery")
	}
}

func TestUDPDiscoveryRoundTrip(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() {
		buf := make([]byte, 1024)
		n, remote, err := conn.ReadFromUDP(buf)
		if err == nil && IsDiscovery(buf[:n]) {
			_, _ = conn.WriteToUDP(Advertisement{Host: "127.0.0.1", Port: 32400, Name: "Plex"}.Response(time.Now()), remote)
		}
	}()
	client, err := net.DialUDP("udp4", nil, conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("M-SEARCH * HTTP/1.1\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1024)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buf[:n]), "plex/media-server") {
		t.Fatalf("unexpected response: %s", string(buf[:n]))
	}
}
