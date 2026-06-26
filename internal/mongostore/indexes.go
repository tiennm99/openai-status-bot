package mongostore

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// deliveryTTLSeconds is the TTL index expiry for delivery markers, in seconds
// (7 days). Mongo's TTL monitor sweeps expired docs roughly once a minute,
// which is irrelevant against a 7-day window.
const deliveryTTLSeconds = int32(deliveryTTL / 1e9)

// EnsureIndexes creates the indexes the store relies on. It is idempotent:
// re-creating an existing identical index is a no-op.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.delivery.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "expiresAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(deliveryTTLSeconds),
		},
		{
			// Speeds DeliveredSubscribers/ClearDelivery lookups by eventKey.
			Keys: bson.D{{Key: "eventKey", Value: 1}},
		},
	})
	return err
}
