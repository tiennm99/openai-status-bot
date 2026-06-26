package redisstore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type Subscriber struct {
	ChatID     int64
	ThreadID   *int
	Types      []string
	Components []string
}

func NewSubscriber(chatID int64, threadID *int) Subscriber {
	var copiedThreadID *int
	if threadID != nil {
		value := *threadID
		copiedThreadID = &value
	}
	return Subscriber{
		ChatID:     chatID,
		ThreadID:   copiedThreadID,
		Types:      DefaultSubscriptionTypes(),
		Components: []string{},
	}
}

func (s Subscriber) Key() string {
	if s.ThreadID == nil {
		return strconv.FormatInt(s.ChatID, 10)
	}
	return fmt.Sprintf("%d:%d", s.ChatID, *s.ThreadID)
}

func ParseSubscriberKey(value string) (Subscriber, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 1 && len(parts) != 2 {
		return Subscriber{}, fmt.Errorf("invalid subscriber key %q", value)
	}

	chatID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Subscriber{}, fmt.Errorf("invalid chat ID: %w", err)
	}
	if len(parts) == 1 {
		return NewSubscriber(chatID, nil), nil
	}

	threadID, err := strconv.Atoi(parts[1])
	if err != nil {
		return Subscriber{}, fmt.Errorf("invalid thread ID: %w", err)
	}
	return NewSubscriber(chatID, &threadID), nil
}

func DefaultSubscriptionTypes() []string {
	return []string{SubscriptionTypeIncident, SubscriptionTypeComponent}
}

func (s Subscriber) Accepts(eventType, componentID, componentName string) bool {
	if !containsFold(s.Types, eventType) {
		return false
	}
	if eventType != SubscriptionTypeComponent || len(s.Components) == 0 {
		return true
	}
	return containsFold(s.Components, componentID) || containsFold(s.Components, componentName)
}

func (s *Store) AddSubscriber(ctx context.Context, sub Subscriber) error {
	key := sub.Key()
	settings, err := s.loadSubscriberSettings(ctx, key)
	if err != nil {
		return err
	}
	if err := s.client.SAdd(ctx, subscribersKey, key).Err(); err != nil {
		return err
	}
	return s.saveSubscriberSettings(ctx, key, settings)
}

func (s *Store) RemoveSubscriber(ctx context.Context, sub Subscriber) error {
	key := sub.Key()
	pipe := s.client.TxPipeline()
	pipe.SRem(ctx, subscribersKey, key)
	pipe.HDel(ctx, subscriberSettingsKey, key)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) ListSubscribers(ctx context.Context) ([]Subscriber, error) {
	keys, err := s.client.SMembers(ctx, subscribersKey).Result()
	if err != nil {
		return nil, err
	}

	// Fetch every subscriber's settings in one HGETALL instead of one HGET per
	// key. The settings hash mirrors the subscribers set, so its size is the
	// subscriber count rather than unbounded history.
	settingsByKey, err := s.client.HGetAll(ctx, subscriberSettingsKey).Result()
	if err != nil {
		return nil, err
	}

	subscribers := make([]Subscriber, 0, len(keys))
	for _, key := range keys {
		sub, err := ParseSubscriberKey(key)
		if err != nil {
			if removeErr := s.client.SRem(ctx, subscribersKey, key).Err(); removeErr != nil {
				return nil, fmt.Errorf("remove malformed subscriber key %q: %w", key, removeErr)
			}
			return nil, fmt.Errorf("malformed subscriber key %q: %w", key, err)
		}
		value, present := settingsByKey[key]
		settings, err := s.resolveSubscriberSettings(ctx, key, value, present)
		if err != nil {
			return nil, err
		}
		sub.Types = settings.Types
		sub.Components = settings.Components
		subscribers = append(subscribers, sub)
	}
	return subscribers, nil
}

func (s *Store) GetSubscriber(ctx context.Context, sub Subscriber) (Subscriber, bool, error) {
	key := sub.Key()
	exists, err := s.client.SIsMember(ctx, subscribersKey, key).Result()
	if err != nil || !exists {
		return Subscriber{}, false, err
	}
	return s.loadSubscriber(ctx, key)
}

func (s *Store) UpdateSubscriberTypes(ctx context.Context, sub Subscriber, types []string) (bool, error) {
	key := sub.Key()
	settings, exists, err := s.loadExistingSubscriberSettings(ctx, key)
	if err != nil || !exists {
		return false, err
	}
	settings.Types = normalizeTypes(types)
	return s.saveExistingSubscriberSettings(ctx, key, settings)
}

func (s *Store) UpdateSubscriberComponents(ctx context.Context, sub Subscriber, components []string) (bool, error) {
	key := sub.Key()
	settings, exists, err := s.loadExistingSubscriberSettings(ctx, key)
	if err != nil || !exists {
		return false, err
	}
	settings.Components = normalizeComponents(components)
	return s.saveExistingSubscriberSettings(ctx, key, settings)
}

func (s *Store) UpdateSubscriberSettings(ctx context.Context, sub Subscriber, types, components []string) (bool, error) {
	key := sub.Key()
	return s.saveExistingSubscriberSettings(ctx, key, subscriberSettings{
		Types:      types,
		Components: components,
	})
}

func (s *Store) loadSubscriber(ctx context.Context, key string) (Subscriber, bool, error) {
	sub, err := ParseSubscriberKey(key)
	if err != nil {
		if removeErr := s.client.SRem(ctx, subscribersKey, key).Err(); removeErr != nil {
			return Subscriber{}, false, fmt.Errorf("remove malformed subscriber key %q: %w", key, removeErr)
		}
		return Subscriber{}, false, fmt.Errorf("malformed subscriber key %q: %w", key, err)
	}
	settings, err := s.loadSubscriberSettings(ctx, key)
	if err != nil {
		return Subscriber{}, false, err
	}
	sub.Types = settings.Types
	sub.Components = settings.Components
	return sub, true, nil
}
