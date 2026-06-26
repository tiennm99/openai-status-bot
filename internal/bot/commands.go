package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/tiennm99/openai-status-bot/internal/poller"
	"github.com/tiennm99/openai-status-bot/internal/redisstore"
	"github.com/tiennm99/openai-status-bot/internal/telegram"
)

func (b *Bot) replySubscribe(ctx context.Context, message telegram.Message, fields []string) {
	sub := redisstore.NewSubscriber(message.Chat.ID, message.MessageThreadID)
	current, subscribed, err := b.store.GetSubscriber(ctx, sub)
	if err != nil {
		b.logger.Error("get subscription", "error", err)
		b.reply(ctx, message, "Could not load subscription right now.")
		return
	}
	if len(fields) < 2 {
		b.reply(ctx, message, formatSubscribeUsage(currentOrTarget(current, sub, subscribed), subscribed))
		return
	}

	args := fields[1:]
	arg := strings.ToLower(args[0])
	if arg == redisstore.SubscriptionTypeComponent && len(args) > 1 {
		b.updateComponentFilter(ctx, message, sub, strings.Join(args[1:], " "), subscribed)
		return
	}

	var types []string
	switch arg {
	case redisstore.SubscriptionTypeIncident:
		types = []string{redisstore.SubscriptionTypeIncident}
	case redisstore.SubscriptionTypeComponent:
		types = []string{redisstore.SubscriptionTypeComponent}
	case "all":
		types = redisstore.DefaultSubscriptionTypes()
	default:
		b.reply(ctx, message, formatSubscribeUsage(currentOrTarget(current, sub, subscribed), subscribed))
		return
	}

	updated, err := b.store.UpdateSubscriberTypes(ctx, sub, types)
	if err != nil {
		b.logger.Error("update subscription", "error", err)
		b.reply(ctx, message, "Could not update subscription right now.")
		return
	}
	if !updated {
		b.reply(ctx, message, "Not subscribed yet. Use /start first.")
		return
	}
	b.reply(ctx, message, fmt.Sprintf("Subscription updated: <code>%s</code>", escape(strings.Join(types, ", "))))
}

func (b *Bot) updateComponentFilter(ctx context.Context, message telegram.Message, sub redisstore.Subscriber, componentArg string, subscribed bool) {
	if !subscribed {
		b.reply(ctx, message, "Not subscribed yet. Use /start first.")
		return
	}
	componentArg = strings.TrimSpace(componentArg)
	if strings.EqualFold(componentArg, "all") {
		current, exists, err := b.store.GetSubscriber(ctx, sub)
		if err != nil {
			b.logger.Error("load subscription", "error", err)
			b.reply(ctx, message, "Could not update component filter right now.")
			return
		}
		if !exists {
			b.reply(ctx, message, "Not subscribed yet. Use /start first.")
			return
		}
		updated, err := b.store.UpdateSubscriberSettings(ctx, sub, withSubscriptionType(current.Types, redisstore.SubscriptionTypeComponent), nil)
		if err != nil {
			b.logger.Error("clear component filter", "error", err)
			b.reply(ctx, message, "Could not update component filter right now.")
			return
		}
		if !updated {
			b.reply(ctx, message, "Not subscribed yet. Use /start first.")
			return
		}
		b.reply(ctx, message, "Component filter cleared. Receiving all component updates.")
		return
	}

	summary, err := b.statusClient.FetchSummary(ctx)
	if err != nil {
		b.logger.Error("fetch components", "error", err)
		b.reply(ctx, message, "Could not fetch OpenAI components right now.")
		return
	}
	resolution := resolveComponent(summary.Components, componentArg)
	if !resolution.Found {
		b.reply(ctx, message, fmt.Sprintf("Component <code>%s</code> not found.", escape(componentArg)))
		return
	}
	if resolution.Ambiguous {
		b.reply(ctx, message, formatAmbiguousComponents(componentArg, resolution.Matches))
		return
	}
	component := resolution.Component

	current, exists, err := b.store.GetSubscriber(ctx, sub)
	if err != nil {
		b.logger.Error("load component filters", "error", err)
		b.reply(ctx, message, "Could not update component filter right now.")
		return
	}
	if !exists {
		b.reply(ctx, message, "Not subscribed yet. Use /start first.")
		return
	}
	components := append([]string{}, current.Components...)
	if !containsComponent(components, component.ID) {
		components = append(components, component.ID)
	}
	types := withSubscriptionType(current.Types, redisstore.SubscriptionTypeComponent)
	updated, err := b.store.UpdateSubscriberSettings(ctx, sub, types, components)
	if err != nil {
		b.logger.Error("update component filter", "error", err)
		b.reply(ctx, message, "Could not update component filter right now.")
		return
	}
	if !updated {
		b.reply(ctx, message, "Not subscribed yet. Use /start first.")
		return
	}
	b.reply(ctx, message, fmt.Sprintf("Subscribed to component: <code>%s</code>\nActive filters: <code>%s</code>", escape(poller.ComponentLabel(component, poller.DuplicateComponentNames(summary.Components)[component.Name])), escape(componentFilterLabels(summary.Components, components))))
}

func withSubscriptionType(types []string, subscriptionType string) []string {
	updated := append([]string{}, types...)
	if !containsComponent(updated, subscriptionType) {
		updated = append(updated, subscriptionType)
	}
	return updated
}

func (b *Bot) replyHistory(ctx context.Context, message telegram.Message, count int) {
	incidents, err := b.statusClient.FetchIncidents(ctx)
	if err != nil {
		b.logger.Error("fetch incidents", "error", err)
		b.reply(ctx, message, "Could not fetch OpenAI incident history right now.")
		return
	}
	b.reply(ctx, message, formatHistory(incidents.Incidents, count))
}

func (b *Bot) replyUptime(ctx context.Context, message telegram.Message) {
	summary, err := b.statusClient.FetchSummary(ctx)
	if err != nil {
		b.logger.Error("fetch uptime", "error", err)
		b.reply(ctx, message, "Could not fetch OpenAI uptime right now.")
		return
	}
	b.reply(ctx, message, formatUptime(summary))
}

func (b *Bot) replyInfo(ctx context.Context, message telegram.Message) {
	sub := redisstore.NewSubscriber(message.Chat.ID, message.MessageThreadID)
	current, subscribed, err := b.store.GetSubscriber(ctx, sub)
	if err != nil {
		b.logger.Error("get subscription info", "error", err)
		b.reply(ctx, message, "Could not load subscription right now.")
		return
	}
	b.reply(ctx, message, formatInfo(currentOrTarget(current, sub, subscribed), subscribed))
}
