package bot

import "github.com/go-telegram/bot/models"

func MenuCommands() []models.BotCommand {
	return []models.BotCommand{
		{Command: "start", Description: "Subscribe this chat or topic"},
		{Command: "stop", Description: "Unsubscribe this chat or topic"},
		{Command: "status", Description: "Show current OpenAI status"},
		{Command: "components", Description: "Show all component statuses"},
		{Command: "subscribe", Description: "Set notification preferences"},
		{Command: "history", Description: "Show recent incidents"},
		{Command: "uptime", Description: "Show component health overview"},
		{Command: "info", Description: "Show chat and subscription details"},
		{Command: "help", Description: "Show command help"},
	}
}
