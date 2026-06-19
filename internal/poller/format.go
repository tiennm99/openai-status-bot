package poller

import (
	"fmt"
	"strings"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
)

func FormatComponentChange(component openai.Component, previousStatus string) string {
	return fmt.Sprintf(
		"OpenAI component status changed\n\nComponent: %s\nStatus: %s -> %s\n\n%s",
		component.Name,
		StatusLabel(previousStatus),
		StatusLabel(component.Status),
		"https://status.openai.com/",
	)
}

func FormatIncidentUpdate(incident openai.Incident, update openai.IncidentUpdate) string {
	body := strings.TrimSpace(update.Body)
	if body == "" {
		body = "No update message provided."
	}

	return fmt.Sprintf(
		"OpenAI incident update\n\n%s\nStatus: %s\nImpact: %s\n\n%s\n\n%s",
		incident.Name,
		StatusLabel(update.Status),
		StatusLabel(incident.Impact),
		truncate(body, 2600),
		"https://status.openai.com/incidents/"+incident.ID,
	)
}

func StatusLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "Unknown"
	case "operational":
		return "Operational"
	case "degraded_performance":
		return "Degraded performance"
	case "partial_outage":
		return "Partial outage"
	case "major_outage":
		return "Major outage"
	case "under_maintenance":
		return "Under maintenance"
	case "none":
		return "None"
	case "minor":
		return "Minor"
	case "major":
		return "Major"
	case "critical":
		return "Critical"
	case "investigating":
		return "Investigating"
	case "identified":
		return "Identified"
	case "monitoring":
		return "Monitoring"
	case "resolved":
		return "Resolved"
	default:
		return strings.ReplaceAll(value, "_", " ")
	}
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-3]) + "..."
}
