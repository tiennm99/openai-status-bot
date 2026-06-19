package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/summary.json" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"page": {"id": "page", "name": "OpenAI"},
			"status": {"description": "All Systems Operational", "indicator": "none"},
			"components": [{"id": "c1", "name": "Codex Web", "status": "operational"}],
			"incidents": []
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	summary, err := client.FetchSummary(context.Background())
	if err != nil {
		t.Fatalf("FetchSummary returned error: %v", err)
	}
	if summary.Status.Indicator != "none" || len(summary.Components) != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestFetchIncidentsReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	if _, err := client.FetchIncidents(context.Background()); err == nil {
		t.Fatal("expected HTTP error")
	}
}
