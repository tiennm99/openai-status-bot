package mongostore

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// deliveryTTLIndexExpireAfterSeconds makes Mongo expire each delivery marker at
// its own expiresAt timestamp. MarkDelivered sets expiresAt to now+7d, matching
// the Redis delivery marker retention window.
const deliveryTTLIndexExpireAfterSeconds = int32(0)

// EnsureIndexes creates the indexes the store relies on. It is idempotent, and
// replaces the earlier delivery TTL index shape that expired at expiresAt+7d.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	if err := s.ensureDeliveryTTLIndex(ctx); err != nil {
		return err
	}
	_, err := s.delivery.Indexes().CreateOne(ctx, mongo.IndexModel{
		// Speeds DeliveredSubscribers/ClearDelivery lookups by eventKey.
		Keys: bson.D{{Key: "eventKey", Value: 1}},
	})
	return err
}

func (s *Store) ensureDeliveryTTLIndex(ctx context.Context) error {
	cursor, err := s.delivery.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	needsCreate := true
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			return err
		}
		if !isExpiresAtIndex(index["key"]) {
			continue
		}
		if indexNumber(index["expireAfterSeconds"]) == int64(deliveryTTLIndexExpireAfterSeconds) {
			needsCreate = false
			continue
		}
		name, _ := index["name"].(string)
		if name == "" {
			continue
		}
		if err := s.delivery.Indexes().DropOne(ctx, name); err != nil {
			return err
		}
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	if !needsCreate {
		return nil
	}

	_, err = s.delivery.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(deliveryTTLIndexExpireAfterSeconds),
	})
	return err
}

func isExpiresAtIndex(key any) bool {
	switch key := key.(type) {
	case bson.M:
		return len(key) == 1 && indexNumber(key["expiresAt"]) == 1
	case map[string]any:
		return len(key) == 1 && indexNumber(key["expiresAt"]) == 1
	case bson.D:
		return len(key) == 1 && key[0].Key == "expiresAt" && indexNumber(key[0].Value) == 1
	default:
		return false
	}
}

func indexNumber(value any) int64 {
	switch value := value.(type) {
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		return -1
	}
}
