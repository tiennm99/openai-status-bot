package poller

import (
	"context"
	"fmt"

	"github.com/tiennm99/openai-status-bot/internal/mongostore"
	"github.com/tiennm99/openai-status-bot/internal/telegram"
)

type deliveryError struct {
	count int
	first error
}

func (e *deliveryError) Error() string {
	if e.count == 1 {
		return fmt.Sprintf("telegram delivery failed for 1 subscriber send: %v", e.first)
	}
	return fmt.Sprintf("telegram delivery failed for %d subscriber sends; first error: %v", e.count, e.first)
}

func (e *deliveryError) Unwrap() error {
	return e.first
}

func (e *deliveryError) add(err error) {
	if err == nil {
		return
	}
	if e.count == 0 {
		e.first = err
	}
	e.count++
}

func (e *deliveryError) addAll(other *deliveryError) {
	if other == nil || other.count == 0 {
		return
	}
	if e.count == 0 {
		e.first = other.first
	}
	e.count += other.count
}

func (r *Runner) notifySubscribers(ctx context.Context, event notificationEvent, subscribers []mongostore.Subscriber, removed, failed map[string]bool) (*deliveryError, error) {
	deliveryFailures := &deliveryError{}
	var delivered map[string]bool
	if event.deliveryKey != "" {
		var err error
		delivered, err = r.store.DeliveredSubscribers(ctx, event.deliveryKey)
		if err != nil {
			return nil, err
		}
	}
	for _, subscriber := range subscribers {
		subscriberKey := subscriber.Key()
		if removed[subscriberKey] {
			continue
		}
		if !subscriber.Accepts(event.eventType, event.componentID, event.componentName) {
			continue
		}
		if delivered[subscriberKey] {
			continue
		}
		if failed[subscriberKey] {
			// This subscriber already hit a retryable failure earlier in the
			// poll. Skip it to avoid hammering, but record this event as
			// incomplete so its checkpoint is deferred and it retries next poll
			// instead of being marked delivered to a subscriber that never got it.
			deliveryFailures.add(fmt.Errorf("deferred %s after earlier failure", subscriberKey))
			continue
		}
		if err := r.notifier.SendMessage(ctx, subscriber, event.text); err != nil {
			if telegram.IsTerminalSendError(err) {
				if removeErr := r.store.RemoveSubscriber(ctx, subscriber); removeErr != nil {
					return nil, removeErr
				}
				removed[subscriberKey] = true
				r.logger.Info("removed unreachable telegram subscriber", "subscriber", subscriberKey, "error", err)
				continue
			}
			r.logger.Warn("send telegram message", "subscriber", subscriberKey, "error", err)
			failed[subscriberKey] = true
			deliveryFailures.add(fmt.Errorf("send to %s: %w", subscriberKey, err))
			continue
		}
		if event.deliveryKey != "" {
			if err := r.store.MarkDelivered(ctx, event.deliveryKey, subscriberKey); err != nil {
				r.logger.Warn("mark telegram delivery", "subscriber", subscriberKey, "event", event.deliveryKey, "error", err)
			}
		}
	}
	if deliveryFailures.count == 0 {
		return nil, nil
	}
	return deliveryFailures, nil
}
