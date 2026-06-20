package redisstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

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

type PendingComponentEvent struct {
	ComponentID    string `json:"component_id"`
	ComponentName  string `json:"component_name"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at"`
	Position       int    `json:"position"`
	PreviousStatus string `json:"previous_status"`
	DeliveryKey    string `json:"delivery_key"`
}

func (s *Store) PendingComponentEvents(ctx context.Context) (map[string]PendingComponentEvent, error) {
	values, err := s.client.HGetAll(ctx, pendingComponentsKey).Result()
	if err != nil {
		return nil, err
	}
	events := make(map[string]PendingComponentEvent, len(values))
	for componentID, value := range values {
		event, err := decodePendingComponentEvent(componentID, value)
		if err != nil {
			return nil, err
		}
		events[componentID] = event
	}
	return events, nil
}

func decodePendingComponentEvent(componentID, value string) (PendingComponentEvent, error) {
	var event PendingComponentEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		return PendingComponentEvent{}, fmt.Errorf("decode pending component event %q: %w", componentID, err)
	}
	if event.ComponentID == "" {
		event.ComponentID = componentID
	}
	return event, nil
}

func (s *Store) SavePendingComponentEvent(ctx context.Context, event PendingComponentEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.client.HSet(ctx, pendingComponentsKey, event.ComponentID, payload).Err()
}

func (s *Store) RemovePendingComponentEvent(ctx context.Context, componentID string) error {
	return s.client.HDel(ctx, pendingComponentsKey, componentID).Err()
}

func (s *Store) HasIncidentUpdate(ctx context.Context, updateID string) (bool, error) {
	return s.client.SIsMember(ctx, incidentUpdatesKey, updateID).Result()
}

func (s *Store) MarkIncidentUpdate(ctx context.Context, updateID string) error {
	return s.client.SAdd(ctx, incidentUpdatesKey, updateID).Err()
}

func (s *Store) HasIncidentUpdateVersion(ctx context.Context, updateID, version string) (bool, error) {
	stored, err := s.client.HGet(ctx, incidentVersionsKey, updateID).Result()
	if err == nil {
		return stored == version, nil
	}
	if err != redis.Nil {
		return false, err
	}
	legacySeen, err := s.HasIncidentUpdate(ctx, updateID)
	if err != nil {
		return false, err
	}
	if legacySeen {
		if err := s.client.HSet(ctx, incidentVersionsKey, updateID, version).Err(); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *Store) MarkIncidentUpdateVersion(ctx context.Context, updateID, version string) error {
	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, incidentVersionsKey, updateID, version)
	pipe.SAdd(ctx, incidentUpdatesKey, updateID)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) HasDelivered(ctx context.Context, eventKey, subscriberKey string) (bool, error) {
	return s.client.SIsMember(ctx, deliveryStateKey(eventKey), subscriberKey).Result()
}

func (s *Store) MarkDelivered(ctx context.Context, eventKey, subscriberKey string) error {
	key := deliveryStateKey(eventKey)
	pipe := s.client.TxPipeline()
	pipe.SAdd(ctx, key, subscriberKey)
	pipe.Expire(ctx, key, 7*24*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) ClearDelivery(ctx context.Context, eventKey string) error {
	return s.client.Del(ctx, deliveryStateKey(eventKey)).Err()
}

func (s *Store) TelegramOffset(ctx context.Context) (int64, error) {
	value, err := s.client.Get(ctx, telegramOffsetKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	offset, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		if deleteErr := s.client.Del(ctx, telegramOffsetKey).Err(); deleteErr != nil {
			return 0, fmt.Errorf("clear invalid telegram offset: %w", deleteErr)
		}
		return 0, nil
	}
	return offset, nil
}

func (s *Store) SaveTelegramOffset(ctx context.Context, offset int64) error {
	return s.client.Set(ctx, telegramOffsetKey, strconv.FormatInt(offset, 10), 0).Err()
}

func deliveryStateKey(eventKey string) string {
	sum := sha256.Sum256([]byte(eventKey))
	return "openai-status:event-delivery:" + hex.EncodeToString(sum[:])
}
