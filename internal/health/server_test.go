package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerReturnsOKWhenCheckPasses(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, Path, nil)
	res := httptest.NewRecorder()

	Handler(func(context.Context) error { return nil }).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if res.Body.String() != "ok\n" {
		t.Fatalf("body = %q, want ok", res.Body.String())
	}
}

func TestHandlerNilCheckIsHealthy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, Path, nil)
	res := httptest.NewRecorder()

	Handler(nil).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
}

func TestHandlerReturnsUnavailableWhenCheckFails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, Path, nil)
	res := httptest.NewRecorder()

	Handler(func(context.Context) error { return errors.New("not ready") }).ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusServiceUnavailable)
	}
}
