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
		return subscriberSettings{Types: DefaultSubscriptionTypes(), Components: []string{}}, nil
	}
	if err != nil {
		return subscriberSettings{}, err
	}

	return decodeSubscriberSettings(key, value)
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
