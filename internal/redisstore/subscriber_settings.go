package redisstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type subscriberSettings struct {
	Types      []string `json:"types"`
	Components []string `json:"components"`
}

var updateSubscriberSettingsScript = redis.NewScript(`
if redis.call("SISMEMBER", KEYS[1], ARGV[1]) == 0 then
	return 0
end
redis.call("HSET", KEYS[2], ARGV[1], ARGV[2])
return 1
`)

func (s *Store) loadExistingSubscriberSettings(ctx context.Context, key string) (subscriberSettings, bool, error) {
	exists, err := s.client.SIsMember(ctx, subscribersKey, key).Result()
	if err != nil || !exists {
		return subscriberSettings{}, false, err
	}
	settings, err := s.loadSubscriberSettings(ctx, key)
	return settings, true, err
}

func (s *Store) loadSubscriberSettings(ctx context.Context, key string) (subscriberSettings, error) {
	value, err := s.client.HGet(ctx, subscriberSettingsKey, key).Result()
	if err == redis.Nil {
		return s.resolveSubscriberSettings(ctx, key, "", false)
	}
	if err != nil {
		return subscriberSettings{}, err
	}
	return s.resolveSubscriberSettings(ctx, key, value, true)
}

// resolveSubscriberSettings turns a raw settings hash value into decoded
// settings. It lets callers that already hold the value (e.g. a batched
// HGETALL in ListSubscribers) reuse the same decode/default/self-heal logic as
// the single-key HGET path. Absent or corrupt values fall back to defaults,
// dropping the corrupt field so it heals on next write.
func (s *Store) resolveSubscriberSettings(ctx context.Context, key, value string, present bool) (subscriberSettings, error) {
	if !present {
		return defaultSubscriberSettings(), nil
	}
	settings, err := decodeSubscriberSettings(key, value)
	if err == nil {
		return settings, nil
	}
	if removeErr := s.client.HDel(ctx, subscriberSettingsKey, key).Err(); removeErr != nil {
		return subscriberSettings{}, fmt.Errorf("clear corrupt subscriber settings %q: %w", key, removeErr)
	}
	return defaultSubscriberSettings(), nil
}

func decodeSubscriberSettings(key, value string) (subscriberSettings, error) {
	var settings subscriberSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return subscriberSettings{}, fmt.Errorf("decode subscriber settings %q: %w", key, err)
	}
	settings.Types = normalizeTypes(settings.Types)
	settings.Components = normalizeComponents(settings.Components)
	return settings, nil
}

func (s *Store) saveSubscriberSettings(ctx context.Context, key string, settings subscriberSettings) error {
	settings.Types = normalizeTypes(settings.Types)
	settings.Components = normalizeComponents(settings.Components)
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.client.HSet(ctx, subscriberSettingsKey, key, payload).Err()
}

func (s *Store) saveExistingSubscriberSettings(ctx context.Context, key string, settings subscriberSettings) (bool, error) {
	settings.Types = normalizeTypes(settings.Types)
	settings.Components = normalizeComponents(settings.Components)
	payload, err := json.Marshal(settings)
	if err != nil {
		return false, err
	}
	updated, err := updateSubscriberSettingsScript.Run(ctx, s.client, []string{subscribersKey, subscriberSettingsKey}, key, string(payload)).Int()
	if err != nil {
		return false, err
	}
	return updated == 1, nil
}

func defaultSubscriberSettings() subscriberSettings {
	return subscriberSettings{Types: DefaultSubscriptionTypes(), Components: []string{}}
}
