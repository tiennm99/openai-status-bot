package poller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/redisstore"
)

func (r *Runner) collectEvents(ctx context.Context, summary openai.Summary, incidents openai.IncidentsResponse, initialized bool) ([]notificationEvent, []checkpoint, []checkpoint, error) {
	componentEvents, componentBefore, componentAfter, err := r.collectComponentEvents(ctx, summary, initialized)
	if err != nil {
		return nil, nil, nil, err
	}
	incidentEvents, incidentAfter, err := r.collectIncidentEvents(ctx, incidents, initialized)
	if err != nil {
		return nil, nil, nil, err
	}

	events := append(componentEvents, incidentEvents...)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].sortTime == "" {
			return false
		}
		if events[j].sortTime == "" {
			return true
		}
		return events[i].sortTime < events[j].sortTime
	})
	after := append(componentAfter, incidentAfter...)
	return events, componentBefore, after, nil
}

func (r *Runner) collectComponentEvents(ctx context.Context, summary openai.Summary, initialized bool) ([]notificationEvent, []checkpoint, []checkpoint, error) {
	knownStatuses, err := r.store.ComponentStatuses(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	pending, err := r.store.PendingComponentEvents(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	duplicates := DuplicateComponentNames(componentsForDuplicateLabels(summary.Components, pending))

	events := make([]notificationEvent, 0)
	before := make([]checkpoint, 0)
	after := make([]checkpoint, 0, len(summary.Components)+len(pending))

	for _, pendingEvent := range pending {
		pendingEvent := pendingEvent
		component := pendingComponent(pendingEvent)
		events = append(events, notificationEvent{
			eventType:     redisstore.SubscriptionTypeComponent,
			componentID:   pendingEvent.ComponentID,
			componentName: pendingEvent.ComponentName,
			deliveryKey:   pendingEvent.DeliveryKey,
			sortTime:      pendingEvent.UpdatedAt,
			text:          FormatComponentChange(component, pendingEvent.PreviousStatus, duplicates[component.Name]),
		})
		after = append(after, r.resolveComponentCheckpoints(pendingEvent.ComponentID, pendingEvent.Status, pendingEvent.DeliveryKey)...)
	}

	for _, component := range summary.Components {
		if component.ID == "" || component.Group {
			continue
		}
		component := component
		if _, hasPending := pending[component.ID]; hasPending {
			continue
		}

		previousStatus, found := knownStatuses[component.ID]
		if !initialized {
			after = append(after, func(ctx context.Context) error {
				return r.store.SaveComponentStatus(ctx, component.ID, component.Status)
			})
			continue
		}
		if found && previousStatus == component.Status {
			after = append(after, func(ctx context.Context) error {
				return r.store.SaveComponentStatus(ctx, component.ID, component.Status)
			})
			continue
		}
		if !found {
			previousStatus = "unknown"
		}

		deliveryKey := fmt.Sprintf("component:%s:%s:%s", component.ID, component.Status, component.UpdatedAt)
		pendingEvent := redisstore.PendingComponentEvent{
			ComponentID:    component.ID,
			ComponentName:  component.Name,
			Status:         component.Status,
			UpdatedAt:      component.UpdatedAt,
			Position:       component.Position,
			PreviousStatus: previousStatus,
			DeliveryKey:    deliveryKey,
		}
		before = append(before, func(ctx context.Context) error {
			return r.store.SavePendingComponentEvent(ctx, pendingEvent)
		})
		events = append(events, notificationEvent{
			eventType:     redisstore.SubscriptionTypeComponent,
			componentID:   component.ID,
			componentName: component.Name,
			deliveryKey:   deliveryKey,
			sortTime:      component.UpdatedAt,
			text:          FormatComponentChange(component, previousStatus, duplicates[component.Name]),
		})
		after = append(after, r.resolveComponentCheckpoints(component.ID, component.Status, deliveryKey)...)
	}
	return events, before, after, nil
}

// resolveComponentCheckpoints builds the post-delivery writes shared by pending
// and freshly detected component events: persist the new status, drop the
// pending marker, and clear the delivery-state set.
func (r *Runner) resolveComponentCheckpoints(componentID, status, deliveryKey string) []checkpoint {
	return []checkpoint{
		func(ctx context.Context) error { return r.store.SaveComponentStatus(ctx, componentID, status) },
		func(ctx context.Context) error { return r.store.RemovePendingComponentEvent(ctx, componentID) },
		func(ctx context.Context) error { return r.store.ClearDelivery(ctx, deliveryKey) },
	}
}

func (r *Runner) collectIncidentEvents(ctx context.Context, response openai.IncidentsResponse, initialized bool) ([]notificationEvent, []checkpoint, error) {
	events := make([]notificationEvent, 0)
	checkpoints := make([]checkpoint, 0)
	for _, incident := range response.Incidents {
		incident := incident
		for _, update := range incident.IncidentUpdates {
			if update.ID == "" {
				continue
			}
			update := update
			version := IncidentUpdateVersion(update)
			seen, err := r.store.HasIncidentUpdateVersion(ctx, update.ID, version)
			if err != nil {
				return nil, nil, err
			}
			if !seen {
				checkpoints = append(checkpoints, func(ctx context.Context) error {
					return r.store.MarkIncidentUpdateVersion(ctx, update.ID, version)
				})
			}
			deliveryKey := fmt.Sprintf("incident:%s:%s", update.ID, version)
			if initialized && !seen {
				events = append(events, notificationEvent{
					eventType:   redisstore.SubscriptionTypeIncident,
					deliveryKey: deliveryKey,
					sortTime:    incidentUpdateSortTime(update),
					text:        FormatIncidentUpdate(incident, update),
				})
				checkpoints = append(checkpoints, func(ctx context.Context) error {
					return r.store.ClearDelivery(ctx, deliveryKey)
				})
			}
		}
	}
	return events, checkpoints, nil
}

func pendingComponent(event redisstore.PendingComponentEvent) openai.Component {
	return openai.Component{
		ID:        event.ComponentID,
		Name:      event.ComponentName,
		Status:    event.Status,
		UpdatedAt: event.UpdatedAt,
		Position:  event.Position,
	}
}

func componentsForDuplicateLabels(components []openai.Component, pending map[string]redisstore.PendingComponentEvent) []openai.Component {
	result := make([]openai.Component, 0, len(components)+len(pending))
	seen := map[string]bool{}
	add := func(component openai.Component) {
		if component.ID == "" {
			result = append(result, component)
			return
		}
		key := component.ID + "\x00" + component.Name
		if seen[key] {
			return
		}
		seen[key] = true
		result = append(result, component)
	}
	for _, component := range components {
		add(component)
	}
	for _, pendingEvent := range pending {
		add(pendingComponent(pendingEvent))
	}
	return result
}

func IncidentUpdateVersion(update openai.IncidentUpdate) string {
	parts := []string{update.Status, update.Body, update.DisplayAt, update.CreatedAt}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", sum[:])
}

func incidentUpdateSortTime(update openai.IncidentUpdate) string {
	for _, value := range []string{update.DisplayAt, update.CreatedAt, update.UpdatedAt} {
		if value != "" {
			return value
		}
	}
	return ""
}
