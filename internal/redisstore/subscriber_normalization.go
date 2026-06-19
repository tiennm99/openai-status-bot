package redisstore

import "strings"

func normalizeTypes(types []string) []string {
	normalized := make([]string, 0, len(types))
	for _, value := range types {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case SubscriptionTypeIncident:
			if !containsFold(normalized, SubscriptionTypeIncident) {
				normalized = append(normalized, SubscriptionTypeIncident)
			}
		case SubscriptionTypeComponent:
			if !containsFold(normalized, SubscriptionTypeComponent) {
				normalized = append(normalized, SubscriptionTypeComponent)
			}
		}
	}
	if len(normalized) == 0 {
		return DefaultSubscriptionTypes()
	}
	return normalized
}

func normalizeComponents(components []string) []string {
	normalized := make([]string, 0, len(components))
	for _, component := range components {
		component = strings.TrimSpace(component)
		if component == "" || containsFold(normalized, component) {
			continue
		}
		normalized = append(normalized, component)
	}
	return normalized
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
