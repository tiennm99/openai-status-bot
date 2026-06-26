package mongostore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Subscriber struct {
	ChatID     int64
	ThreadID   *int
	Types      []string
	Components []string
}

// subscriberDoc is the stored shape of a subscriber. Identity lives in _id (the
// subscriber key), mirroring the Redis design where the key held identity and a
// separate hash held settings; chatID/threadID are denormalized for inspection.
type subscriberDoc struct {
	ID         string   `bson:"_id"`
	ChatID     int64    `bson:"chatID"`
	ThreadID   *int     `bson:"threadID,omitempty"`
	Types      []string `bson:"types"`
	Components []string `bson:"components"`
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

// AddSubscriber upserts the subscriber. An existing subscriber keeps its
// settings on re-/start (only chatID/threadID are refreshed); a new subscriber
// is seeded with default types and no component filter.
func (s *Store) AddSubscriber(ctx context.Context, sub Subscriber) error {
	// Identity lives in _id; chatID/threadID are denormalized for inspection.
	// Omit threadID when absent so the stored shape matches subscriberDoc's
	// omitempty decode (no explicit null).
	set := bson.M{"chatID": sub.ChatID}
	if sub.ThreadID != nil {
		set["threadID"] = *sub.ThreadID
	}
	update := bson.M{
		"$set": set,
		"$setOnInsert": bson.M{
			"types":      DefaultSubscriptionTypes(),
			"components": []string{},
		},
	}
	_, err := s.subscribers.UpdateOne(ctx, bson.M{"_id": sub.Key()}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (s *Store) RemoveSubscriber(ctx context.Context, sub Subscriber) error {
	_, err := s.subscribers.DeleteOne(ctx, bson.M{"_id": sub.Key()})
	return err
}

func (s *Store) ListSubscribers(ctx context.Context) ([]Subscriber, error) {
	cursor, err := s.subscribers.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var subscribers []Subscriber
	for cursor.Next(ctx) {
		var doc subscriberDoc
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		sub, err := s.subscriberFromDoc(ctx, doc)
		if err != nil {
			return nil, err
		}
		subscribers = append(subscribers, sub)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return subscribers, nil
}

func (s *Store) GetSubscriber(ctx context.Context, sub Subscriber) (Subscriber, bool, error) {
	var doc subscriberDoc
	if err := s.subscribers.FindOne(ctx, bson.M{"_id": sub.Key()}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return Subscriber{}, false, nil
		}
		return Subscriber{}, false, err
	}
	resolved, err := s.subscriberFromDoc(ctx, doc)
	if err != nil {
		return Subscriber{}, false, err
	}
	return resolved, true, nil
}

func (s *Store) UpdateSubscriberTypes(ctx context.Context, sub Subscriber, types []string) (bool, error) {
	return s.setSubscriberFields(ctx, sub.Key(), bson.M{"types": normalizeTypes(types)})
}

func (s *Store) UpdateSubscriberComponents(ctx context.Context, sub Subscriber, components []string) (bool, error) {
	return s.setSubscriberFields(ctx, sub.Key(), bson.M{"components": normalizeComponents(components)})
}

func (s *Store) UpdateSubscriberSettings(ctx context.Context, sub Subscriber, types, components []string) (bool, error) {
	return s.setSubscriberFields(ctx, sub.Key(), bson.M{
		"types":      normalizeTypes(types),
		"components": normalizeComponents(components),
	})
}

// setSubscriberFields applies a $set only when the subscriber exists.
// MatchedCount==0 means "not subscribed", replacing the Redis check-then-set
// Lua script.
func (s *Store) setSubscriberFields(ctx context.Context, key string, fields bson.M) (bool, error) {
	result, err := s.subscribers.UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$set": fields})
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}

// subscriberFromDoc derives identity from the document _id (so a malformed key
// self-heals as it did under Redis) and normalizes the stored settings.
func (s *Store) subscriberFromDoc(ctx context.Context, doc subscriberDoc) (Subscriber, error) {
	sub, err := ParseSubscriberKey(doc.ID)
	if err != nil {
		if _, removeErr := s.subscribers.DeleteOne(ctx, bson.M{"_id": doc.ID}); removeErr != nil {
			return Subscriber{}, fmt.Errorf("remove malformed subscriber key %q: %w", doc.ID, removeErr)
		}
		return Subscriber{}, fmt.Errorf("malformed subscriber key %q: %w", doc.ID, err)
	}
	sub.Types = normalizeTypes(doc.Types)
	sub.Components = normalizeComponents(doc.Components)
	return sub, nil
}
