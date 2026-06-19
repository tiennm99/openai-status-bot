package bot

import "testing"

func TestNormalizeCommandStripsBotUsername(t *testing.T) {
	command, fields := normalizeCommand("/history@OpenAIStatusBot 10")
	if command != "/history" {
		t.Fatalf("command = %q, want /history", command)
	}
	if len(fields) != 2 || fields[1] != "10" {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestParseHistoryCount(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   int
	}{
		{name: "default", fields: []string{"/history"}, want: 5},
		{name: "valid", fields: []string{"/history", "3"}, want: 3},
		{name: "invalid", fields: []string{"/history", "abc"}, want: 5},
		{name: "max", fields: []string{"/history", "99"}, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseHistoryCount(tt.fields); got != tt.want {
				t.Fatalf("parseHistoryCount(%v) = %d, want %d", tt.fields, got, tt.want)
			}
		})
	}
}
