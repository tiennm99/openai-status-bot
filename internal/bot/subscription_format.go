package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tiennm99/openai-status-bot/internal/redisstore"
)

func formatInfo(sub redisstore.Subscriber, subscribed bool) string {
	chatID := strconv.FormatInt(sub.ChatID, 10)
	if sub.ThreadID != nil {
		chatID = fmt.Sprintf("%s:%d", chatID, *sub.ThreadID)
	}
	if !subscribed {
		return fmt.Sprintf("<b>Chat Info</b>\n\nChat ID: <code>%s</code>\n\nNot subscribed. Use /start to subscribe.", escape(chatID))
	}
	components := "all"
	if len(sub.Components) > 0 {
		components = strings.Join(sub.Components, ", ")
	}
	return fmt.Sprintf(
		"<b>Chat Info</b>\n\nChat ID: <code>%s</code>\n\nTypes: <code>%s</code>\nComponents: <code>%s</code>",
		escape(chatID),
		escape(strings.Join(sub.Types, ", ")),
		escape(components),
	)
}

func formatSubscribeUsage(sub redisstore.Subscriber, subscribed bool) string {
	current := "none (use /start first)"
	components := "all"
	if subscribed {
		current = strings.Join(sub.Types, ", ")
		if len(sub.Components) > 0 {
			components = strings.Join(sub.Components, ", ")
		}
	}
	return fmt.Sprintf(
		"<b>Usage:</b> /subscribe &lt;type&gt; [component]\n\nTypes: <code>incident</code>, <code>component</code>, <code>all</code>\nComponent filter: <code>/subscribe component api</code>\nClear filter: <code>/subscribe component all</code>\n\nCurrent types: <code>%s</code>\nComponents: <code>%s</code>",
		escape(current),
		escape(components),
	)
}
