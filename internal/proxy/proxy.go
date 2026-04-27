package proxy

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type contextKey string

const originalHostKey contextKey = "originalHost"

func New(target *url.URL, plexHost string) http.Handler {
	return NewWithOptions(target, plexHost, Options{})
}

type Options struct {
	Log       *slog.Logger
	AccessLog bool
}

func NewWithOptions(target *url.URL, plexHost string, opts Options) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(target)
	original := rp.Director
	rp.Director = func(req *http.Request) {
		captureOriginalHost(req)
		original(req)
		req.Host = plexHost
		req.Header.Set("Host", plexHost)
		req.Header.Set("X-Forwarded-Proto", "http")
		if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			req.Header.Set("X-Real-IP", host)
			req.Header.Del("X-Forwarded-For")
		}
		if req.Header.Get("Origin") != "" {
			req.Header.Set("Origin", target.Scheme+"://"+plexHost)
		}
		if req.Header.Get("Referer") != "" {
			req.Header.Set("Referer", target.Scheme+"://"+plexHost+"/")
		}
		if isWebsocket(req) {
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
		}
	}
	rp.ModifyResponse = rewriteServersResponse
	rp.ErrorHandler = errorHandler(opts.Log)
	rp.FlushInterval = -1
	return withAccessLog(rp, opts)
}

func NewDynamic(targetURL func() *url.URL, plexHost string) http.Handler {
	return NewDynamicWithOptions(targetURL, plexHost, Options{})
}

func NewDynamicWithOptions(targetURL func() *url.URL, plexHost string, opts Options) http.Handler {
	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			captureOriginalHost(req)
			target := targetURL()
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = plexHost
			req.Header.Set("Host", plexHost)
			req.Header.Set("X-Forwarded-Proto", "http")
			if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
				req.Header.Set("X-Real-IP", host)
				req.Header.Del("X-Forwarded-For")
			}
			if req.Header.Get("Origin") != "" {
				req.Header.Set("Origin", target.Scheme+"://"+plexHost)
			}
			if req.Header.Get("Referer") != "" {
				req.Header.Set("Referer", target.Scheme+"://"+plexHost+"/")
			}
			if isWebsocket(req) {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}
		},
		ModifyResponse: rewriteServersResponse,
		ErrorHandler:   errorHandler(opts.Log),
		FlushInterval:  -1,
	}
	return withAccessLog(rp, opts)
}

func TargetURL(scheme, addr string) (*url.URL, error) {
	return url.Parse(scheme + "://" + addr)
}

func isWebsocket(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket")
}

func errorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, req *http.Request, err error) {
		if logger != nil {
			host, _ := req.Context().Value(originalHostKey).(string)
			if host == "" {
				host = req.Host
			}
			logger.Error("proxy request failed",
				"method", req.Method,
				"path", req.URL.RequestURI(),
				"host", host,
				"upstream_host", req.Host,
				"remote_addr", req.RemoteAddr,
				"user_agent", req.UserAgent(),
				"error", err,
			)
		}
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
	}
}

func withAccessLog(next http.Handler, opts Options) http.Handler {
	if !opts.AccessLog {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		method := req.Method
		path := req.URL.RequestURI()
		host := req.Host
		remoteAddr := req.RemoteAddr
		userAgent := req.UserAgent()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, req)
		if opts.Log != nil {
			opts.Log.Info("proxy access",
				"method", method,
				"path", path,
				"host", host,
				"remote_addr", remoteAddr,
				"user_agent", userAgent,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	n, err := r.ResponseWriter.Write(body)
	r.bytes += int64(n)
	return n, err
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func captureOriginalHost(req *http.Request) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	*req = *req.WithContext(context.WithValue(req.Context(), originalHostKey, host))
}

func rewriteServersResponse(resp *http.Response) error {
	if resp.Request == nil || resp.Request.URL == nil || resp.Request.URL.Path != "/servers" {
		return nil
	}
	originalHost, _ := resp.Request.Context().Value(originalHostKey).(string)
	if originalHost == "" {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	rewritten, changed := rewriteServersXML(body, originalHost)
	if !changed {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}
	resp.Body = io.NopCloser(bytes.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	resp.Header.Del("Content-Encoding")
	return nil
}

type serversResponse struct {
	XMLName xml.Name `xml:"MediaContainer"`
	Size    string   `xml:"size,attr,omitempty"`
	Servers []server `xml:"Server"`
}

type server struct {
	XMLName xml.Name `xml:"Server"`
	Name    string   `xml:"name,attr,omitempty"`
	Host    string   `xml:"host,attr,omitempty"`
	Address string   `xml:"address,attr,omitempty"`
	Port    string   `xml:"port,attr,omitempty"`
	Machine string   `xml:"machineIdentifier,attr,omitempty"`
	Version string   `xml:"version,attr,omitempty"`
}

func rewriteServersXML(body []byte, originalHost string) ([]byte, bool) {
	var parsed serversResponse
	if err := xml.Unmarshal(body, &parsed); err != nil || parsed.XMLName.Local != "MediaContainer" || len(parsed.Servers) == 0 {
		return nil, false
	}
	host, port := splitHostPort(originalHost, parsed.Servers[0].Port)
	if host == "" {
		return nil, false
	}
	changed := false
	for i := range parsed.Servers {
		if parsed.Servers[i].Host != host {
			parsed.Servers[i].Host = host
			changed = true
		}
		if parsed.Servers[i].Address != host {
			parsed.Servers[i].Address = host
			changed = true
		}
		if port != "" && parsed.Servers[i].Port != port {
			parsed.Servers[i].Port = port
			changed = true
		}
	}
	if !changed {
		return body, false
	}
	out, err := xml.Marshal(parsed)
	if err != nil {
		return nil, false
	}
	return append([]byte(xml.Header), out...), true
}

func splitHostPort(authority, fallbackPort string) (string, string) {
	host, port, err := net.SplitHostPort(authority)
	if err == nil {
		return strings.Trim(host, "[]"), port
	}
	if strings.Count(authority, ":") == 1 {
		parts := strings.Split(authority, ":")
		if parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1]
		}
	}
	return strings.Trim(authority, "[]"), fallbackPort
}
