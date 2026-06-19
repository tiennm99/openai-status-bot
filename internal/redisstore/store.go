package redisstore

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

const (
	subscribersKey       = "openai-status:subscribers"
	componentStatusesKey = "openai-status:component-statuses"
	incidentUpdatesKey   = "openai-status:incident-updates"
	initializedKey       = "openai-status:initialized"
)

type Subscriber struct {
	ChatID   int64
	ThreadID *int
}

func NewSubscriber(chatID int64, threadID *int) Subscriber {
	var copiedThreadID *int
	if threadID != nil {
		value := *threadID
		copiedThreadID = &value
	}
	return Subscriber{ChatID: chatID, ThreadID: copiedThreadID}
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
		return Subscriber{ChatID: chatID}, nil
	}

	threadID, err := strconv.Atoi(parts[1])
	if err != nil {
		return Subscriber{}, fmt.Errorf("invalid thread ID: %w", err)
	}
	return NewSubscriber(chatID, &threadID), nil
}

type Store struct {
	client redis.UniversalClient
}

func New(client redis.UniversalClient) *Store {
	return &Store{client: client}
}

func (s *Store) AddSubscriber(ctx context.Context, sub Subscriber) error {
	return s.client.SAdd(ctx, subscribersKey, sub.Key()).Err()
}

func (s *Store) RemoveSubscriber(ctx context.Context, sub Subscriber) error {
	return s.client.SRem(ctx, subscribersKey, sub.Key()).Err()
}

func (s *Store) ListSubscribers(ctx context.Context) ([]Subscriber, error) {
	keys, err := s.client.SMembers(ctx, subscribersKey).Result()
	if err != nil {
		return nil, err
	}

	subscribers := make([]Subscriber, 0, len(keys))
	for _, key := range keys {
		sub, err := ParseSubscriberKey(key)
		if err != nil {
			return nil, err
		}
		subscribers = append(subscribers, sub)
	}
	return subscribers, nil
}

func (s *Store) IsInitialized(ctx context.Context) (bool, error) {
	count, err := s.client.Exists(ctx, initializedKey).Result()
	return count > 0, err
}

func (s *Store) SetInitialized(ctx context.Context) error {
	return s.client.Set(ctx, initializedKey, "1", 0).Err()
}

func (s *Store) ComponentStatuses(ctx context.Context) (map[string]string, error) {
	return s.client.HGetAll(ctx, componentStatusesKey).Result()
}

func (s *Store) SaveComponentStatus(ctx context.Context, componentID, status string) error {
	return s.client.HSet(ctx, componentStatusesKey, componentID, status).Err()
}

func (s *Store) HasIncidentUpdate(ctx context.Context, updateID string) (bool, error) {
	return s.client.SIsMember(ctx, incidentUpdatesKey, updateID).Result()
}

func (s *Store) MarkIncidentUpdate(ctx context.Context, updateID string) error {
	return s.client.SAdd(ctx, incidentUpdatesKey, updateID).Err()
}
