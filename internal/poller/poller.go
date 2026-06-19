package poller

import (
	"context"
	"log/slog"
	"time"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/redisstore"
)

type StatusClient interface {
	FetchSummary(ctx context.Context) (openai.Summary, error)
	FetchIncidents(ctx context.Context) (openai.IncidentsResponse, error)
}

type Store interface {
	ComponentStatuses(ctx context.Context) (map[string]string, error)
	ClearDelivery(ctx context.Context, eventKey string) error
	HasDelivered(ctx context.Context, eventKey, subscriberKey string) (bool, error)
	HasIncidentUpdateVersion(ctx context.Context, updateID, version string) (bool, error)
	IsInitialized(ctx context.Context) (bool, error)
	ListSubscribers(ctx context.Context) ([]redisstore.Subscriber, error)
	MarkDelivered(ctx context.Context, eventKey, subscriberKey string) error
	MarkIncidentUpdateVersion(ctx context.Context, updateID, version string) error
	PendingComponentEvents(ctx context.Context) (map[string]redisstore.PendingComponentEvent, error)
	RemovePendingComponentEvent(ctx context.Context, componentID string) error
	RemoveSubscriber(ctx context.Context, sub redisstore.Subscriber) error
	SavePendingComponentEvent(ctx context.Context, event redisstore.PendingComponentEvent) error
	SaveComponentStatus(ctx context.Context, componentID, status string) error
	SetInitialized(ctx context.Context) error
}

type Notifier interface {
	SendMessage(ctx context.Context, sub redisstore.Subscriber, text string) error
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

	events, beforeDelivery, checkpoints, err := r.collectEvents(ctx, summary, incidents, initialized)
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
		for _, event := range events {
			if err := r.notifySubscribers(ctx, event, subscribers, removedSubscribers); err != nil {
				return err
			}
		}
	}

	for _, save := range checkpoints {
		if err := save(ctx); err != nil {
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

func (r *Runner) checkAndLog(ctx context.Context) {
	if err := r.CheckOnce(ctx); err != nil && ctx.Err() == nil {
		r.logger.Error("poll openai status", "error", err)
	}
}
