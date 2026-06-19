package redisstore

import "github.com/redis/go-redis/v9"

const (
	subscribersKey        = "openai-status:subscribers"
	subscriberSettingsKey = "openai-status:subscriber-settings"
	componentStatusesKey  = "openai-status:component-statuses"
	pendingComponentsKey  = "openai-status:pending-component-events"
	incidentUpdatesKey    = "openai-status:incident-updates"
	incidentVersionsKey   = "openai-status:incident-update-versions"
	initializedKey        = "openai-status:initialized"
	telegramOffsetKey     = "openai-status:telegram-offset"
)

const (
	SubscriptionTypeIncident  = "incident"
	SubscriptionTypeComponent = "component"
)

type Store struct {
	client redis.UniversalClient
}

func New(client redis.UniversalClient) *Store {
	return &Store{client: client}
}
