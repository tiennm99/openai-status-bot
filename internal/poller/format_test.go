package poller

import (
	"strings"
	"testing"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
)

func TestStatusLabel(t *testing.T) {
	tests := map[string]string{
		"operational":          "Operational",
		"degraded_performance": "Degraded Performance",
		"partial_outage":       "Partial Outage",
		"minor":                "Minor",
		"":                     "Unknown",
	}

	for input, expected := range tests {
		if got := StatusLabel(input); got != expected {
			t.Fatalf("StatusLabel(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestFormatIncidentUpdateIncludesCoreFields(t *testing.T) {
	message := FormatIncidentUpdate(
		openai.Incident{
			ID:     "inc_123",
			Name:   "Codex outage",
			Impact: "major",
		},
		openai.IncidentUpdate{
			Status: "identified",
			Body:   "We found the issue.",
		},
	)

	for _, want := range []string{"Codex outage", "Identified", "Major", "We found the issue.", "inc_123"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q: %s", want, message)
		}
	}
}
