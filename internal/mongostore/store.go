package mongostore

import "go.mongodb.org/mongo-driver/v2/mongo"

// Collection names. Each former Redis key becomes a MongoDB collection; the
// document model collapses the subscribers set + settings hash into one
// collection and replaces the per-event delivery sets with a single TTL-indexed
// collection.
const (
	subscribersCollection            = "subscribers"
	componentStatusesCollection      = "component_statuses"
	pendingComponentEventsCollection = "pending_component_events"
	incidentUpdateVersionsCollection = "incident_update_versions"
	deliveryCollection               = "delivery"
	metaCollection                   = "meta"
)

// meta document IDs.
const (
	metaInitializedID    = "initialized"
	metaTelegramOffsetID = "telegramOffset"
)

const (
	SubscriptionTypeIncident  = "incident"
	SubscriptionTypeComponent = "component"
)

type Store struct {
	db                     *mongo.Database
	subscribers            *mongo.Collection
	componentStatuses      *mongo.Collection
	pendingComponentEvents *mongo.Collection
	incidentUpdateVersions *mongo.Collection
	delivery               *mongo.Collection
	meta                   *mongo.Collection
}

func New(client *mongo.Client, dbName string) *Store {
	db := client.Database(dbName)
	return &Store{
		db:                     db,
		subscribers:            db.Collection(subscribersCollection),
		componentStatuses:      db.Collection(componentStatusesCollection),
		pendingComponentEvents: db.Collection(pendingComponentEventsCollection),
		incidentUpdateVersions: db.Collection(incidentUpdateVersionsCollection),
		delivery:               db.Collection(deliveryCollection),
		meta:                   db.Collection(metaCollection),
	}
}
