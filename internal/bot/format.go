package bot

import (
	"fmt"
	"strconv"
	"strings"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/poller"
)

const helpText = `OpenAI Status Bot

/start - subscribe this chat or topic
/stop - unsubscribe this chat or topic
/status - show current OpenAI status
/components - show all component statuses
/history [count] - show recent incidents, default 5, max 10
/help - show this help`

func formatStatus(summary openai.Summary) string {
	lines := []string{
		"OpenAI status",
		"",
		fmt.Sprintf("Overall: %s", summary.Status.Description),
	}

	degraded := make([]string, 0)
	for _, component := range summary.Components {
		if component.Group || component.Status == "operational" {
			continue
		}
		degraded = append(degraded, fmt.Sprintf("- %s: %s", component.Name, poller.StatusLabel(component.Status)))
	}

	if len(degraded) == 0 {
		lines = append(lines, "", "All listed components are operational.")
	} else {
		lines = append(lines, "", "Affected components:")
		lines = append(lines, degraded...)
	}
	lines = append(lines, "", "https://status.openai.com/")

	return strings.Join(lines, "\n")
}

func formatComponents(summary openai.Summary) string {
	lines := []string{"OpenAI components", ""}
	for _, component := range summary.Components {
		if component.Group {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", component.Name, poller.StatusLabel(component.Status)))
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

	lines := []string{"Recent OpenAI incidents", ""}
	for i := 0; i < count; i++ {
		incident := incidents[i]
		lines = append(lines, fmt.Sprintf(
			"%d. %s\n   Status: %s | Impact: %s",
			i+1,
			incident.Name,
			poller.StatusLabel(incident.Status),
			poller.StatusLabel(incident.Impact),
		))
	}
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

func normalizeCommand(text string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", nil
	}
	command := strings.ToLower(fields[0])
	if at := strings.Index(command, "@"); at >= 0 {
		command = command[:at]
	}
	return command, fields
}

func truncateMessage(value string) string {
	const telegramLimit = 3900
	if len(value) <= telegramLimit {
		return value
	}
	return strings.TrimSpace(value[:telegramLimit-3]) + "..."
}
