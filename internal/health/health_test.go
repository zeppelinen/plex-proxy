package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyEndpoint(t *testing.T) {
	state := &State{}
	srv := NewServer("127.0.0.1:0", state)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	state.SetReady(true)
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
