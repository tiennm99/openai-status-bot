package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, Path, nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if res.Body.String() != "ok\n" {
		t.Fatalf("body = %q, want ok", res.Body.String())
	}
}
