package proxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyRewritesHeaders(t *testing.T) {
	var gotHost, gotRealIP, gotForwarded, gotOrigin, gotReferer string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotRealIP = r.Header.Get("X-Real-IP")
		gotForwarded = r.Header.Get("X-Forwarded-For")
		gotOrigin = r.Header.Get("Origin")
		gotReferer = r.Header.Get("Referer")
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	target, err := TargetURL("http", strings.TrimPrefix(backend.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://proxy/web", nil)
	req.RemoteAddr = "192.0.2.50:1234"
	req.Header.Set("Origin", "http://old")
	req.Header.Set("Referer", "http://old/")
	rec := httptest.NewRecorder()
	New(target, "plex.local:32400").ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if gotHost != "plex.local:32400" || gotRealIP != "192.0.2.50" || gotForwarded != "192.0.2.50" {
		t.Fatalf("headers host=%q real=%q fwd=%q", gotHost, gotRealIP, gotForwarded)
	}
	if gotOrigin != "http://plex.local:32400" || gotReferer != "http://plex.local:32400/" {
		t.Fatalf("origin=%q referer=%q", gotOrigin, gotReferer)
	}
}

func TestProxyStreamsResponse(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("not flushable")
		}
		_, _ = w.Write([]byte("chunk"))
		flusher.Flush()
	}))
	defer backend.Close()
	target, _ := TargetURL("http", strings.TrimPrefix(backend.URL, "http://"))
	server := httptest.NewServer(New(target, "plex.local:32400"))
	defer server.Close()
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "chunk" {
		t.Fatalf("body = %q", body)
	}
}

func TestWebsocketHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" || r.Header.Get("Connection") != "Upgrade" {
			t.Fatalf("upgrade headers not preserved: %v", r.Header)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	target, _ := TargetURL("http", strings.TrimPrefix(backend.URL, "http://"))
	req := httptest.NewRequest(http.MethodGet, "http://proxy", nil)
	req.RemoteAddr = net.JoinHostPort("127.0.0.1", "1")
	req.Header.Set("Upgrade", "websocket")
	rec := httptest.NewRecorder()
	New(target, "plex.local:32400").ServeHTTP(rec, req)
}
