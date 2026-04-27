package proxy

import (
	"bytes"
	"io"
	"log/slog"
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

func TestAccessLogEnabled(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer backend.Close()
	target, _ := TargetURL("http", strings.TrimPrefix(backend.URL, "http://"))

	req := httptest.NewRequest(http.MethodPost, "http://proxy/library?x=1", nil)
	req.RemoteAddr = net.JoinHostPort("192.0.2.51", "5000")
	req.Header.Set("User-Agent", "plex-test")
	rec := httptest.NewRecorder()
	NewWithOptions(target, "plex.local:32400", Options{Log: logger, AccessLog: true}).ServeHTTP(rec, req)

	got := logs.String()
	for _, want := range []string{`msg="proxy access"`, `method=POST`, `path="/library?x=1"`, `remote_addr=192.0.2.51:5000`, `user_agent=plex-test`, `status=201`, `bytes=7`} {
		if !strings.Contains(got, want) {
			t.Fatalf("access log missing %q:\n%s", want, got)
		}
	}
}

func TestAccessLogCanBeDisabled(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	target, _ := TargetURL("http", strings.TrimPrefix(backend.URL, "http://"))

	req := httptest.NewRequest(http.MethodGet, "http://proxy/", nil)
	rec := httptest.NewRecorder()
	NewWithOptions(target, "plex.local:32400", Options{Log: logger, AccessLog: false}).ServeHTTP(rec, req)

	if logs.Len() != 0 {
		t.Fatalf("unexpected access log: %s", logs.String())
	}
}

func TestProxyErrorLogIncludesRequestDetails(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	target, _ := TargetURL("http", "127.0.0.1:1")

	req := httptest.NewRequest(http.MethodGet, "http://proxy/transcode?session=1", nil)
	req.RemoteAddr = net.JoinHostPort("192.0.2.52", "5001")
	req.Header.Set("User-Agent", "plex-client")
	rec := httptest.NewRecorder()
	NewWithOptions(target, "plex.local:32400", Options{Log: logger, AccessLog: false}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rec.Code)
	}
	got := logs.String()
	for _, want := range []string{`level=ERROR`, `msg="proxy request failed"`, `method=GET`, `path="/transcode?session=1"`, `host=proxy`, `remote_addr=192.0.2.52:5001`, `user_agent=plex-client`, `error=`} {
		if !strings.Contains(got, want) {
			t.Fatalf("error log missing %q:\n%s", want, got)
		}
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

func TestProxyRewritesServersEndpointToClientHost(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/servers" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/xml;charset=utf-8")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="1">
<Server name="plex" host="192.168.1.5" address="192.168.1.5" port="32400" machineIdentifier="id" version="1.2.3" />
</MediaContainer>`))
	}))
	defer backend.Close()
	target, _ := TargetURL("http", strings.TrimPrefix(backend.URL, "http://"))

	req := httptest.NewRequest(http.MethodGet, "http://proxy.local:32401/servers", nil)
	req.Host = "proxy.local:32401"
	req.RemoteAddr = net.JoinHostPort("192.0.2.50", "1234")
	rec := httptest.NewRecorder()
	New(target, "plex.local:32400").ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{`host="proxy.local"`, `address="proxy.local"`, `port="32401"`, `machineIdentifier="id"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q:\n%s", want, body)
		}
	}
}

func TestServersXMLRewriteKeepsFallbackPort(t *testing.T) {
	body := []byte(`<MediaContainer size="1"><Server name="plex" host="192.168.1.5" address="192.168.1.5" port="32400" /></MediaContainer>`)
	rewritten, changed := rewriteServersXML(body, "proxy.local")
	if !changed {
		t.Fatal("expected rewrite")
	}
	got := string(rewritten)
	for _, want := range []string{`host="proxy.local"`, `address="proxy.local"`, `port="32400"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("response missing %q:\n%s", want, got)
		}
	}
}
