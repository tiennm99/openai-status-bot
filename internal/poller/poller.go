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
	HasIncidentUpdate(ctx context.Context, updateID string) (bool, error)
	IsInitialized(ctx context.Context) (bool, error)
	ListSubscribers(ctx context.Context) ([]redisstore.Subscriber, error)
	MarkIncidentUpdate(ctx context.Context, updateID string) error
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

	messages, err := r.collectComponentMessages(ctx, summary, initialized)
	if err != nil {
		return err
	}
	incidentMessages, err := r.collectIncidentMessages(ctx, incidents, initialized)
	if err != nil {
		return err
	}
	messages = append(messages, incidentMessages...)

	if initialized && len(messages) > 0 {
		r.notifyAll(ctx, messages)
	}
	if !initialized {
		if err := r.store.SetInitialized(ctx); err != nil {
			return err
		}
		r.logger.Info("seeded status baseline")
	}
	return nil
}

func (r *Runner) collectComponentMessages(ctx context.Context, summary openai.Summary, initialized bool) ([]string, error) {
	knownStatuses, err := r.store.ComponentStatuses(ctx)
	if err != nil {
		return nil, err
	}

	messages := make([]string, 0)
	for _, component := range summary.Components {
		if component.ID == "" || component.Group {
			continue
		}
		previousStatus, found := knownStatuses[component.ID]
		if initialized && found && previousStatus != component.Status {
			messages = append(messages, FormatComponentChange(component, previousStatus))
		}
		if err := r.store.SaveComponentStatus(ctx, component.ID, component.Status); err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func (r *Runner) collectIncidentMessages(ctx context.Context, response openai.IncidentsResponse, initialized bool) ([]string, error) {
	messages := make([]string, 0)
	for _, incident := range response.Incidents {
		for i := len(incident.IncidentUpdates) - 1; i >= 0; i-- {
			update := incident.IncidentUpdates[i]
			if update.ID == "" {
				continue
			}
			seen, err := r.store.HasIncidentUpdate(ctx, update.ID)
			if err != nil {
				return nil, err
			}
			if initialized && !seen {
				messages = append(messages, FormatIncidentUpdate(incident, update))
			}
			if !seen {
				if err := r.store.MarkIncidentUpdate(ctx, update.ID); err != nil {
					return nil, err
				}
			}
		}
	}
	return messages, nil
}

func (r *Runner) notifyAll(ctx context.Context, messages []string) {
	subscribers, err := r.store.ListSubscribers(ctx)
	if err != nil {
		r.logger.Error("list subscribers", "error", err)
		return
	}
	if len(subscribers) == 0 {
		return
	}

	for _, message := range messages {
		for _, subscriber := range subscribers {
			if err := r.notifier.SendMessage(ctx, subscriber, message); err != nil {
				r.logger.Warn("send telegram message", "subscriber", subscriber.Key(), "error", err)
			}
		}
	}
}

func (r *Runner) checkAndLog(ctx context.Context) {
	if err := r.CheckOnce(ctx); err != nil && ctx.Err() == nil {
		r.logger.Error("poll openai status", "error", err)
	}
}
