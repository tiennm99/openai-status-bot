package bot

import (
	"strings"
	"testing"
)

func TestTruncateMessageKeepsHTMLValidOnNewlineBoundary(t *testing.T) {
	var b strings.Builder
	for b.Len() <= telegramMessageLimit {
		b.WriteString("- <b>Component</b>: <code>operational</code>\n")
	}
	out := truncateMessage(b.String())

	if len([]rune(out)) > telegramMessageLimit {
		t.Fatalf("len = %d, want <= %d", len([]rune(out)), telegramMessageLimit)
	}
	if !strings.HasSuffix(out, truncationNotice) {
		t.Fatalf("missing truncation notice: %q", out[len(out)-40:])
	}
	body := strings.TrimSuffix(out, truncationNotice)
	// Newline-boundary cut must leave equal numbers of opening/closing tags.
	for _, tag := range []string{"b", "code"} {
		if open, close := strings.Count(body, "<"+tag+">"), strings.Count(body, "</"+tag+">"); open != close {
			t.Fatalf("unbalanced <%s>: %d open, %d close", tag, open, close)
		}
	}
	if strings.Contains(body, "<co") && !strings.Contains(body, "<code>") {
		t.Fatal("body ends inside a tag")
	}
}

func TestNormalizeCommandStripsOwnBotUsername(t *testing.T) {
	command, fields := normalizeCommand("/history@OpenAIStatusBot 10", "OpenAIStatusBot")
	if command != "/history" {
		t.Fatalf("command = %q, want /history", command)
	}
	if len(fields) != 2 || fields[1] != "10" {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestNormalizeCommandIgnoresOtherBotUsername(t *testing.T) {
	command, _ := normalizeCommand("/start@OtherBot", "OpenAIStatusBot")
	if command != "" {
		t.Fatalf("command = %q, want empty", command)
	}
}

func TestNormalizeCommandIgnoresTargetedCommandWhenUsernameUnknown(t *testing.T) {
	command, _ := normalizeCommand("/start@OtherBot", "")
	if command != "" {
		t.Fatalf("command = %q, want empty", command)
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
