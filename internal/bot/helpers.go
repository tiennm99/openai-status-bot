package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/tiennm99/openai-status-bot/internal/mongostore"
	"github.com/tiennm99/openai-status-bot/internal/poller"
)

func (b *App) replyStatus(ctx context.Context, message MessageContext, fields []string) {
	summary, err := b.statusClient.FetchSummary(ctx)
	if err != nil {
		b.logger.Error("fetch status", "error", err)
		b.reply(ctx, message, "Could not fetch OpenAI status right now.")
		return
	}
	if len(fields) > 1 {
		query := strings.Join(fields[1:], " ")
		resolution := resolveComponent(summary.Components, query)
		if !resolution.Found {
			b.reply(ctx, message, fmt.Sprintf("Component <code>%s</code> not found.", escape(query)))
			return
		}
		if resolution.Ambiguous {
			b.reply(ctx, message, formatAmbiguousComponents(query, resolution.Matches))
			return
		}
		duplicates := poller.DuplicateComponentNames(summary.Components)
		b.reply(ctx, message, formatComponentStatus(resolution.Component, duplicates[resolution.Component.Name]))
		return
	}
	b.reply(ctx, message, formatStatus(summary))
}

func (b *App) replyComponents(ctx context.Context, message MessageContext) {
	summary, err := b.statusClient.FetchSummary(ctx)
	if err != nil {
		b.logger.Error("fetch components", "error", err)
		b.reply(ctx, message, "Could not fetch OpenAI components right now.")
		return
	}
	b.reply(ctx, message, formatComponents(summary))
}

func (b *App) reply(ctx context.Context, message MessageContext, text string) {
	if err := b.sender.SendText(ctx, message.ChatID, message.ThreadID, text); err != nil {
		b.logger.Warn("send telegram reply", "chat_id", message.ChatID, "error", err)
	}
}

func currentOrTarget(current, target mongostore.Subscriber, subscribed bool) mongostore.Subscriber {
	if subscribed {
		return current
	}
	return target
}

func containsComponent(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
