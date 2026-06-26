package bot

import (
	"fmt"
	"strings"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/poller"
)

type componentResolution struct {
	Component openai.Component
	Matches   []openai.Component
	Found     bool
	Ambiguous bool
}

func resolveComponent(components []openai.Component, query string) componentResolution {
	query = strings.TrimSpace(query)
	if query == "" {
		return componentResolution{}
	}
	for _, component := range components {
		if component.Group {
			continue
		}
		if strings.EqualFold(component.ID, query) {
			return componentResolution{Component: component, Found: true}
		}
	}

	exact := matchingComponents(components, query, func(name, query string) bool {
		return strings.EqualFold(name, query)
	})
	if len(exact) == 1 {
		return componentResolution{Component: exact[0], Found: true}
	}
	if len(exact) > 1 {
		return componentResolution{Matches: exact, Found: true, Ambiguous: true}
	}

	contains := matchingComponents(components, query, func(name, query string) bool {
		return strings.Contains(strings.ToLower(name), strings.ToLower(query))
	})
	if len(contains) == 1 {
		return componentResolution{Component: contains[0], Found: true}
	}
	if len(contains) > 1 {
		return componentResolution{Matches: contains, Found: true, Ambiguous: true}
	}
	return componentResolution{}
}

func matchingComponents(components []openai.Component, query string, matches func(name, query string) bool) []openai.Component {
	result := make([]openai.Component, 0)
	for _, component := range components {
		if component.Group {
			continue
		}
		if matches(component.Name, query) {
			result = append(result, component)
		}
	}
	return result
}

func formatAmbiguousComponents(query string, matches []openai.Component) string {
	duplicates := poller.DuplicateComponentNames(matches)
	lines := []string{fmt.Sprintf("Component <code>%s</code> is ambiguous. Use one of these IDs:", escape(query)), ""}
	for _, component := range matches {
		lines = append(lines, fmt.Sprintf("- %s: <code>%s</code>", escape(poller.ComponentLabel(component, duplicates[component.Name])), escape(component.ID)))
	}
	return truncateMessage(strings.Join(lines, "\n"))
}

func componentFilterLabels(components []openai.Component, ids []string) string {
	if len(ids) == 0 {
		return "all"
	}
	duplicates := poller.DuplicateComponentNames(components)
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		label := id
		for _, component := range components {
			if component.ID == id {
				label = poller.ComponentLabel(component, duplicates[component.Name])
				break
			}
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}
