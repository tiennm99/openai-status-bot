package poller

import (
	"context"

	"github.com/tiennm99/openai-status-bot/internal/redisstore"
	"github.com/tiennm99/openai-status-bot/internal/telegram"
)

func (r *Runner) notifySubscribers(ctx context.Context, event notificationEvent, subscribers []redisstore.Subscriber, removed map[string]bool) error {
	for _, subscriber := range subscribers {
		subscriberKey := subscriber.Key()
		if removed[subscriberKey] {
			continue
		}
		if !subscriber.Accepts(event.eventType, event.componentID, event.componentName) {
			continue
		}
		if event.deliveryKey != "" {
			delivered, err := r.store.HasDelivered(ctx, event.deliveryKey, subscriberKey)
			if err != nil {
				return err
			}
			if delivered {
				continue
			}
		}
		if err := r.notifier.SendMessage(ctx, subscriber, event.text); err != nil {
			if telegram.IsTerminalSendError(err) {
				if removeErr := r.store.RemoveSubscriber(ctx, subscriber); removeErr != nil {
					return removeErr
				}
				removed[subscriberKey] = true
				r.logger.Info("removed unreachable telegram subscriber", "subscriber", subscriberKey, "error", err)
				continue
			}
			r.logger.Warn("send telegram message", "subscriber", subscriberKey, "error", err)
			return err
		}
		if event.deliveryKey != "" {
			if err := r.store.MarkDelivered(ctx, event.deliveryKey, subscriberKey); err != nil {
				r.logger.Warn("mark telegram delivery", "subscriber", subscriberKey, "event", event.deliveryKey, "error", err)
			}
		}
	}
	return nil
}
