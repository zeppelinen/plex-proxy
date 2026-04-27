package proxy

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

type contextKey string

const originalHostKey contextKey = "originalHost"

func New(target *url.URL, plexHost string) http.Handler {
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
	rp.FlushInterval = -1
	return rp
}

func NewDynamic(targetURL func() *url.URL, plexHost string) http.Handler {
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
		FlushInterval:  -1,
	}
	return rp
}

func TargetURL(scheme, addr string) (*url.URL, error) {
	return url.Parse(scheme + "://" + addr)
}

func isWebsocket(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket")
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
