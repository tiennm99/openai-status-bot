package poller

import (
	"fmt"
	"html"
	"strings"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
)

func FormatComponentChange(component openai.Component, previousStatus string, duplicate bool) string {
	return fmt.Sprintf(
		"<b>OpenAI component update</b>\n\nComponent: <b>%s</b>\nStatus: <code>%s</code> -&gt; <code>%s</code>\n\n<a href=\"https://status.openai.com/\">View full status page</a>",
		escape(ComponentLabel(component, duplicate)),
		escape(StatusLabel(previousStatus)),
		escape(StatusLabel(component.Status)),
	)
}

func FormatIncidentUpdate(incident openai.Incident, update openai.IncidentUpdate) string {
	body := strings.TrimSpace(update.Body)
	if body == "" {
		body = "No update message provided."
	}

	link := incident.Shortlink
	if link == "" && incident.ID != "" {
		link = "https://status.openai.com/incidents/" + incident.ID
	}

	return fmt.Sprintf(
		"<b>OpenAI incident update</b>\n\n<b>%s</b>\nStatus: <code>%s</code>\nImpact: <code>%s</code>\n\n%s\n\n<a href=\"%s\">View incident</a>",
		escape(incident.Name),
		escape(StatusLabel(update.Status)),
		escape(StatusLabel(incident.Impact)),
		escape(truncate(body, 2600)),
		escape(link),
	)
}

func StatusLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "Unknown"
	case "operational":
		return "Operational"
	case "degraded_performance":
		return "Degraded Performance"
	case "partial_outage":
		return "Partial Outage"
	case "major_outage":
		return "Major Outage"
	case "under_maintenance":
		return "Under Maintenance"
	case "none":
		return "None"
	case "minor":
		return "Minor"
	case "major":
		return "Major"
	case "critical":
		return "Critical"
	case "maintenance":
		return "Maintenance"
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

// DuplicateComponentNames reports which non-group component names appear more
// than once, so labels can disambiguate them with a short ID suffix.
func DuplicateComponentNames(components []openai.Component) map[string]bool {
	counts := map[string]int{}
	for _, component := range components {
		if component.Group {
			continue
		}
		counts[component.Name]++
	}
	duplicates := map[string]bool{}
	for name, count := range counts {
		duplicates[name] = count > 1
	}
	return duplicates
}

// ComponentLabel renders a component's display name, appending a short ID when
// the name collides with another component.
func ComponentLabel(component openai.Component, duplicate bool) string {
	if !duplicate || component.ID == "" {
		return component.Name
	}
	return fmt.Sprintf("%s (ID: %s)", component.Name, ShortID(component.ID))
}

func ShortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}

func escape(value string) string {
	return html.EscapeString(value)
}
