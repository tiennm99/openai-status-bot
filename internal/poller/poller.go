package poller

import (
	"context"
	"log/slog"
	"time"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/mongostore"
)

type StatusClient interface {
	FetchSummary(ctx context.Context) (openai.Summary, error)
	FetchIncidents(ctx context.Context) (openai.IncidentsResponse, error)
}

type Store interface {
	ComponentStatuses(ctx context.Context) (map[string]string, error)
	ClearDelivery(ctx context.Context, eventKey string) error
	DeliveredSubscribers(ctx context.Context, eventKey string) (map[string]bool, error)
	HasIncidentUpdateVersion(ctx context.Context, updateID, version string) (bool, error)
	IsInitialized(ctx context.Context) (bool, error)
	ListSubscribers(ctx context.Context) ([]mongostore.Subscriber, error)
	MarkDelivered(ctx context.Context, eventKey, subscriberKey string) error
	MarkIncidentUpdateVersion(ctx context.Context, updateID, version string) error
	PendingComponentEvents(ctx context.Context) (map[string]mongostore.PendingComponentEvent, error)
	RemovePendingComponentEvent(ctx context.Context, componentID string) error
	RemoveSubscriber(ctx context.Context, sub mongostore.Subscriber) error
	SavePendingComponentEvent(ctx context.Context, event mongostore.PendingComponentEvent) error
	SaveComponentStatus(ctx context.Context, componentID, status string) error
	SetInitialized(ctx context.Context) error
}

type Notifier interface {
	SendMessage(ctx context.Context, sub mongostore.Subscriber, text string) error
}

type Runner struct {
	statusClient StatusClient
	store        Store
	notifier     Notifier
	interval     time.Duration
	logger       *slog.Logger
}

type notificationEvent struct {
	eventType     string
	componentID   string
	componentName string
	deliveryKey   string
	sortTime      string
	text          string
	// checkpoints are the post-delivery store writes for this event. They run
	// only after the event has fully delivered to every accepting subscriber,
	// so an unrelated event's delivery failure cannot block them.
	checkpoints []checkpoint
}

type checkpoint func(ctx context.Context) error

func NewRunner(statusClient StatusClient, store Store, notifier Notifier, interval time.Duration, logger *slog.Logger) *Runner {
	return &Runner{
		statusClient: statusClient,
		store:        store,
		notifier:     notifier,
		interval:     interval,
		logger:       logger,
	}
}

func (r *Runner) Run(ctx context.Context) {
	r.checkAndLog(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkAndLog(ctx)
		}
	}
}

func (r *Runner) CheckOnce(ctx context.Context) error {
	initialized, err := r.store.IsInitialized(ctx)
	if err != nil {
		return err
	}

	summary, err := r.statusClient.FetchSummary(ctx)
	if err != nil {
		return err
	}
	incidents, err := r.statusClient.FetchIncidents(ctx)
	if err != nil {
		return err
	}

	events, beforeDelivery, baseline, err := r.collectEvents(ctx, summary, incidents, initialized)
	if err != nil {
		return err
	}

	for _, save := range beforeDelivery {
		if err := save(ctx); err != nil {
			return err
		}
	}

	if initialized && len(events) > 0 {
		subscribers, err := r.store.ListSubscribers(ctx)
		if err != nil {
			return err
		}
		removedSubscribers := map[string]bool{}
		failedSubscribers := map[string]bool{}
		deliveryErr := &deliveryError{}
		for _, event := range events {
			failures, err := r.notifySubscribers(ctx, event, subscribers, removedSubscribers, failedSubscribers)
			if err != nil {
				return err
			}
			if failures != nil {
				// Event not fully delivered; defer its checkpoints so it is
				// re-collected and retried on the next poll. Other events are
				// unaffected.
				deliveryErr.addAll(failures)
				continue
			}
			if err := runCheckpoints(ctx, event.checkpoints); err != nil {
				return err
			}
		}
		if err := runCheckpoints(ctx, baseline); err != nil {
			return err
		}
		if deliveryErr.count > 0 {
			return deliveryErr
		}
		return nil
	}

	// Seeding, or nothing to deliver: run baseline writes and flush any event
	// checkpoints (e.g. stale pending markers carried over from a prior run).
	if err := runCheckpoints(ctx, baseline); err != nil {
		return err
	}
	for _, event := range events {
		if err := runCheckpoints(ctx, event.checkpoints); err != nil {
			return err
		}
	}
	if !initialized {
		if err := r.store.SetInitialized(ctx); err != nil {
			return err
		}
		r.logger.Info("seeded status baseline")
	}
	return nil
}

func runCheckpoints(ctx context.Context, checkpoints []checkpoint) error {
	for _, save := range checkpoints {
		if err := save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) checkAndLog(ctx context.Context) {
	if err := r.CheckOnce(ctx); err != nil && ctx.Err() == nil {
		r.logger.Error("poll openai status", "error", err)
	}
}
