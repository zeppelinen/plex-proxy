package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func New(target *url.URL, plexHost string) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(target)
	original := rp.Director
	rp.Director = func(req *http.Request) {
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
	rp.FlushInterval = -1
	return rp
}

func NewDynamic(targetURL func() *url.URL, plexHost string) http.Handler {
	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
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
		FlushInterval: -1,
	}
	return rp
}

func TargetURL(scheme, addr string) (*url.URL, error) {
	return url.Parse(scheme + "://" + addr)
}

func isWebsocket(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket")
}
