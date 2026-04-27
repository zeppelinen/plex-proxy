package health

import (
	"context"
	"net/http"
	"sync/atomic"
)

type State struct {
	ready atomic.Bool
}

func (s *State) SetReady(ready bool) {
	s.ready.Store(ready)
}

func (s *State) Ready() bool {
	return s.ready.Load()
}

func NewServer(addr string, state *State) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !state.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	return &http.Server{Addr: addr, Handler: mux}
}

func Shutdown(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
