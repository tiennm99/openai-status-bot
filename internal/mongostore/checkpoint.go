package mongostore

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// deliveryTTL is the retention window for per-event delivery markers; enforced
// by the TTL index in EnsureIndexes.
const deliveryTTL = 7 * 24 * time.Hour

type PendingComponentEvent struct {
	ComponentID    string `bson:"_id"`
	ComponentName  string `bson:"componentName"`
	Status         string `bson:"status"`
	UpdatedAt      string `bson:"updatedAt"`
	Position       int    `bson:"position"`
	PreviousStatus string `bson:"previousStatus"`
	DeliveryKey    string `bson:"deliveryKey"`
}

func (s *Store) IsInitialized(ctx context.Context) (bool, error) {
	err := s.meta.FindOne(ctx, bson.M{"_id": metaInitializedID}).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) SetInitialized(ctx context.Context) error {
	_, err := s.meta.UpdateOne(ctx, bson.M{"_id": metaInitializedID},
		bson.M{"$set": bson.M{"value": true}}, options.UpdateOne().SetUpsert(true))
	return err
}

func (s *Store) ComponentStatuses(ctx context.Context) (map[string]string, error) {
	cursor, err := s.componentStatuses.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	statuses := map[string]string{}
	for cursor.Next(ctx) {
		var doc struct {
			ID     string `bson:"_id"`
			Status string `bson:"status"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		statuses[doc.ID] = doc.Status
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return statuses, nil
}

func (s *Store) SaveComponentStatus(ctx context.Context, componentID, status string) error {
	_, err := s.componentStatuses.UpdateOne(ctx, bson.M{"_id": componentID},
		bson.M{"$set": bson.M{"status": status}}, options.UpdateOne().SetUpsert(true))
	return err
}

func (s *Store) PendingComponentEvents(ctx context.Context) (map[string]PendingComponentEvent, error) {
	cursor, err := s.pendingComponentEvents.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	events := map[string]PendingComponentEvent{}
	for cursor.Next(ctx) {
		var event PendingComponentEvent
		if err := cursor.Decode(&event); err != nil {
			return nil, err
		}
		events[event.ComponentID] = event
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) SavePendingComponentEvent(ctx context.Context, event PendingComponentEvent) error {
	_, err := s.pendingComponentEvents.ReplaceOne(ctx, bson.M{"_id": event.ComponentID}, event,
		options.Replace().SetUpsert(true))
	return err
}

func (s *Store) RemovePendingComponentEvent(ctx context.Context, componentID string) error {
	_, err := s.pendingComponentEvents.DeleteOne(ctx, bson.M{"_id": componentID})
	return err
}

func (s *Store) HasIncidentUpdateVersion(ctx context.Context, updateID, version string) (bool, error) {
	var doc struct {
		Version string `bson:"version"`
	}
	err := s.incidentUpdateVersions.FindOne(ctx, bson.M{"_id": updateID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return doc.Version == version, nil
}

func (s *Store) MarkIncidentUpdateVersion(ctx context.Context, updateID, version string) error {
	_, err := s.incidentUpdateVersions.UpdateOne(ctx, bson.M{"_id": updateID},
		bson.M{"$set": bson.M{"version": version}}, options.UpdateOne().SetUpsert(true))
	return err
}

// DeliveredSubscribers returns the subscriber keys already notified for
// eventKey. One Find replaces a per-subscriber membership check.
func (s *Store) DeliveredSubscribers(ctx context.Context, eventKey string) (map[string]bool, error) {
	cursor, err := s.delivery.Find(ctx, bson.M{"eventKey": eventKey})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	delivered := map[string]bool{}
	for cursor.Next(ctx) {
		var doc struct {
			Subscriber string `bson:"subscriber"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		delivered[doc.Subscriber] = true
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return delivered, nil
}

func (s *Store) MarkDelivered(ctx context.Context, eventKey, subscriberKey string) error {
	id := eventKey + "|" + subscriberKey
	update := bson.M{"$set": bson.M{
		"eventKey":   eventKey,
		"subscriber": subscriberKey,
		"expiresAt":  time.Now().Add(deliveryTTL),
	}}
	_, err := s.delivery.UpdateOne(ctx, bson.M{"_id": id}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (s *Store) ClearDelivery(ctx context.Context, eventKey string) error {
	_, err := s.delivery.DeleteMany(ctx, bson.M{"eventKey": eventKey})
	return err
}

func (s *Store) TelegramOffset(ctx context.Context) (int64, error) {
	var doc struct {
		Value int64 `bson:"value"`
	}
	err := s.meta.FindOne(ctx, bson.M{"_id": metaTelegramOffsetID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return doc.Value, nil
}

func (s *Store) SaveTelegramOffset(ctx context.Context, offset int64) error {
	_, err := s.meta.UpdateOne(ctx, bson.M{"_id": metaTelegramOffsetID},
		bson.M{"$set": bson.M{"value": offset}}, options.UpdateOne().SetUpsert(true))
	return err
}
