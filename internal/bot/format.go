package bot

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/poller"
)

const statusURL = "https://status.openai.com/"

const helpText = `<b>OpenAI Status Bot</b>

/start - subscribe this chat or topic
/stop - unsubscribe this chat or topic
/status [component] - show current OpenAI status
/components - show all component statuses
/subscribe &lt;incident|component|all&gt; - set notification types
/subscribe component &lt;name|id|all&gt; - filter component updates
/history [count] - show recent incidents, default 5, max 10
/uptime - show component health overview
/info - show chat and subscription details
/help - show this help`

func formatStatus(summary openai.Summary) string {
	lines := []string{
		"<b>OpenAI Status</b>",
		"",
		fmt.Sprintf("Overall: <code>%s</code>", escape(summary.Status.Description)),
	}

	duplicates := poller.DuplicateComponentNames(summary.Components)
	degraded := make([]string, 0)
	for _, component := range summary.Components {
		if component.Group || component.Status == "operational" {
			continue
		}
		degraded = append(degraded, fmt.Sprintf("- %s: <code>%s</code>", escape(poller.ComponentLabel(component, duplicates[component.Name])), escape(poller.StatusLabel(component.Status))))
	}

	if len(degraded) == 0 {
		lines = append(lines, "", "All listed components are operational.")
	} else {
		lines = append(lines, "", "Affected components:")
		lines = append(lines, degraded...)
	}
	lines = append(lines, "", fmt.Sprintf("<a href=\"%s\">View full status page</a>", statusURL))

	return strings.Join(lines, "\n")
}

func formatComponentStatus(component openai.Component, duplicate bool) string {
	return fmt.Sprintf(
		"<b>%s</b>\n\nStatus: <code>%s</code>\nLast change: %s UTC\n\n<a href=\"%s\">View full status page</a>",
		escape(poller.ComponentLabel(component, duplicate)),
		escape(poller.StatusLabel(component.Status)),
		escape(formatTime(component.UpdatedAt)),
		statusURL,
	)
}

func formatComponents(summary openai.Summary) string {
	lines := []string{"<b>OpenAI Components</b>", ""}
	duplicates := poller.DuplicateComponentNames(summary.Components)
	for _, component := range summary.Components {
		if component.Group {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: <code>%s</code>", escape(poller.ComponentLabel(component, duplicates[component.Name])), escape(poller.StatusLabel(component.Status))))
	}
	return truncateMessage(strings.Join(lines, "\n"))
}

func formatHistory(incidents []openai.Incident, count int) string {
	if len(incidents) == 0 {
		return "No recent incidents found."
	}
	if count > len(incidents) {
		count = len(incidents)
	}

	lines := []string{"<b>Recent OpenAI Incidents</b>", ""}
	for i := 0; i < count; i++ {
		incident := incidents[i]
		link := incident.Shortlink
		if link == "" && incident.ID != "" {
			link = statusURL + "incidents/" + incident.ID
		}
		entry := fmt.Sprintf(
			"%d. <b>[%s]</b> %s\n   Created: %s UTC\n   Status: <code>%s</code>",
			i+1,
			escape(strings.ToUpper(emptyDefault(incident.Impact, "unknown"))),
			escape(incident.Name),
			escape(formatDate(incident.CreatedAt)),
			escape(poller.StatusLabel(incident.Status)),
		)
		if link != "" {
			entry += fmt.Sprintf("\n   <a href=\"%s\">Details</a>", escape(link))
		}
		lines = append(lines, entry)
	}
	lines = append(lines, "", fmt.Sprintf("<a href=\"%shistory\">View full history</a>", statusURL))
	return truncateMessage(strings.Join(lines, "\n\n"))
}

func formatUptime(summary openai.Summary) string {
	lines := []string{"<b>OpenAI Component Health</b>", ""}
	duplicates := poller.DuplicateComponentNames(summary.Components)
	for _, component := range summary.Components {
		if component.Group {
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"%s\n   Status: <code>%s</code>\n   Last change: %s UTC",
			escape(poller.ComponentLabel(component, duplicates[component.Name])),
			escape(poller.StatusLabel(component.Status)),
			escape(formatTime(component.UpdatedAt)),
		))
	}
	lines = append(lines, "", "Uptime percentage is not available from the public Statuspage API.")
	lines = append(lines, fmt.Sprintf("<a href=\"%s\">View full status page</a>", statusURL))
	return truncateMessage(strings.Join(lines, "\n"))
}

func parseHistoryCount(fields []string) int {
	const (
		defaultCount = 5
		maxCount     = 10
	)
	if len(fields) < 2 {
		return defaultCount
	}
	count, err := strconv.Atoi(fields[1])
	if err != nil || count < 1 {
		return defaultCount
	}
	if count > maxCount {
		return maxCount
	}
	return count
}

func truncateMessage(value string) string {
	const telegramLimit = 3900
	runes := []rune(value)
	if len(runes) <= telegramLimit {
		return value
	}
	return strings.TrimSpace(string(runes[:telegramLimit-3])) + "..."
}

func formatDate(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return emptyDefault(value, "unknown")
	}
	return parsed.UTC().Format("Jan 2, 2006")
}

func formatTime(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return emptyDefault(value, "unknown")
	}
	return parsed.UTC().Format("Jan 2, 2006 15:04")
}

func emptyDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func escape(value string) string {
	return html.EscapeString(value)
}
