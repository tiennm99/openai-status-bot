//go:build integration

// Package mongostore integration tests run against a real mongod via
// testcontainers and are excluded from the default `go test ./...`. Run them
// with Docker available:
//
//	go test -tags=integration ./internal/mongostore/...
package mongostore

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	tcmongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
)

var (
	sharedClientOnce sync.Once
	sharedClient     *mongo.Client
	sharedClientErr  error
)

// testMongo starts the shared mongod container once, then hands each test a
// Store bound to a unique database that is dropped on cleanup.
func testMongo(t *testing.T) *Store {
	t.Helper()
	store := testMongoWithoutIndexes(t)
	if err := store.EnsureIndexes(context.Background()); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	return store
}

func testMongoWithoutIndexes(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()

	sharedClientOnce.Do(func() {
		container, err := tcmongo.Run(ctx, "mongo:8")
		if err != nil {
			sharedClientErr = err
			return
		}
		uri, err := container.ConnectionString(ctx)
		if err != nil {
			sharedClientErr = err
			return
		}
		// The container is intentionally left running for the whole package
		// run; testcontainers' reaper removes it after the process exits.
		_ = container
		sharedClient, sharedClientErr = mongo.Connect(options.Client().ApplyURI(uri))
	})
	if sharedClientErr != nil {
		t.Fatalf("start mongo container: %v", sharedClientErr)
	}

	dbName := "test_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db := sharedClient.Database(dbName)
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
	})

	return New(sharedClient, dbName)
}

func TestAddAndGetSubscriber(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	sub := NewSubscriber(123, nil)
	if err := store.AddSubscriber(ctx, sub); err != nil {
		t.Fatalf("AddSubscriber: %v", err)
	}

	got, exists, err := store.GetSubscriber(ctx, sub)
	if err != nil || !exists {
		t.Fatalf("GetSubscriber exists=%v err=%v", exists, err)
	}
	if len(got.Types) != 2 {
		t.Fatalf("default Types = %v, want both defaults", got.Types)
	}
	if len(got.Components) != 0 {
		t.Fatalf("default Components = %v, want empty", got.Components)
	}
}

func TestAddSubscriberPreservesSettingsOnReStart(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	sub := NewSubscriber(123, nil)
	if err := store.AddSubscriber(ctx, sub); err != nil {
		t.Fatalf("AddSubscriber: %v", err)
	}
	if _, err := store.UpdateSubscriberSettings(ctx, sub, []string{SubscriptionTypeComponent}, []string{"c-api"}); err != nil {
		t.Fatalf("UpdateSubscriberSettings: %v", err)
	}
	if err := store.AddSubscriber(ctx, sub); err != nil {
		t.Fatalf("re-AddSubscriber: %v", err)
	}

	got, _, err := store.GetSubscriber(ctx, sub)
	if err != nil {
		t.Fatalf("GetSubscriber: %v", err)
	}
	if len(got.Types) != 1 || got.Types[0] != SubscriptionTypeComponent {
		t.Fatalf("Types = %v, want preserved [component]", got.Types)
	}
	if len(got.Components) != 1 || got.Components[0] != "c-api" {
		t.Fatalf("Components = %v, want preserved [c-api]", got.Components)
	}
}

func TestRemoveSubscriberDeletesDocument(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	sub := NewSubscriber(123, nil)
	if err := store.AddSubscriber(ctx, sub); err != nil {
		t.Fatalf("AddSubscriber: %v", err)
	}
	if err := store.RemoveSubscriber(ctx, sub); err != nil {
		t.Fatalf("RemoveSubscriber: %v", err)
	}
	_, exists, err := store.GetSubscriber(ctx, sub)
	if err != nil {
		t.Fatalf("GetSubscriber after remove: %v", err)
	}
	if exists {
		t.Fatal("subscriber still exists after remove")
	}
}

func TestGetSubscriberMissingReturnsFalse(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	_, exists, err := store.GetSubscriber(ctx, NewSubscriber(999, nil))
	if err != nil {
		t.Fatalf("GetSubscriber: %v", err)
	}
	if exists {
		t.Fatal("exists = true, want false for missing subscriber")
	}
}

func TestListSubscribersSelfHealsMalformedKey(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	good := NewSubscriber(1, nil)
	if err := store.AddSubscriber(ctx, good); err != nil {
		t.Fatalf("AddSubscriber: %v", err)
	}
	if _, err := store.subscribers.InsertOne(ctx, bson.M{"_id": "abc:def:ghi", "types": []string{}, "components": []string{}}); err != nil {
		t.Fatalf("insert malformed subscriber: %v", err)
	}

	if _, err := store.ListSubscribers(ctx); err == nil {
		t.Fatal("expected malformed key error on first list")
	}
	// Malformed doc was deleted; the next list succeeds with only the good one.
	subs, err := store.ListSubscribers(ctx)
	if err != nil {
		t.Fatalf("second ListSubscribers: %v", err)
	}
	if len(subs) != 1 || subs[0].Key() != good.Key() {
		t.Fatalf("subscribers = %v, want only %q", subs, good.Key())
	}
}

func TestListSubscribersSelfHealsMalformedSettings(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	if _, err := store.subscribers.InsertOne(ctx, bson.M{
		"_id":        "2",
		"chatID":     int64(2),
		"types":      "not-an-array",
		"components": bson.A{123},
	}); err != nil {
		t.Fatalf("insert malformed subscriber settings: %v", err)
	}

	subs, err := store.ListSubscribers(ctx)
	if err != nil {
		t.Fatalf("ListSubscribers: %v", err)
	}
	if len(subs) != 1 || subs[0].Key() != "2" {
		t.Fatalf("subscribers = %v, want key 2", subs)
	}
	if len(subs[0].Types) != 2 || len(subs[0].Components) != 0 {
		t.Fatalf("settings = types %v components %v, want defaults", subs[0].Types, subs[0].Components)
	}

	var raw bson.M
	if err := store.subscribers.FindOne(ctx, bson.M{"_id": "2"}).Decode(&raw); err != nil {
		t.Fatalf("load healed subscriber: %v", err)
	}
	types, ok := storedStringSlice(raw["types"])
	if !ok || len(normalizeTypes(types)) != 2 {
		t.Fatalf("healed types = %v ok=%v, want defaults", raw["types"], ok)
	}
	components, ok := storedStringSlice(raw["components"])
	if !ok || len(components) != 0 {
		t.Fatalf("healed components = %v ok=%v, want empty", raw["components"], ok)
	}
}

func TestGetSubscriberSelfHealsMalformedSettings(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	if _, err := store.subscribers.InsertOne(ctx, bson.M{
		"_id":        "3",
		"chatID":     int64(3),
		"types":      bson.A{"incident"},
		"components": int32(42),
	}); err != nil {
		t.Fatalf("insert malformed subscriber settings: %v", err)
	}

	got, exists, err := store.GetSubscriber(ctx, NewSubscriber(3, nil))
	if err != nil || !exists {
		t.Fatalf("GetSubscriber exists=%v err=%v", exists, err)
	}
	if len(got.Types) != 1 || got.Types[0] != SubscriptionTypeIncident || len(got.Components) != 0 {
		t.Fatalf("settings = types %v components %v, want incident and empty components", got.Types, got.Components)
	}
}

func TestUpdateSubscriberSettingsMatchedVsMissing(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	sub := NewSubscriber(1, nil)

	updated, err := store.UpdateSubscriberSettings(ctx, sub, []string{SubscriptionTypeComponent}, []string{"c-api"})
	if err != nil {
		t.Fatalf("UpdateSubscriberSettings: %v", err)
	}
	if updated {
		t.Fatal("updated = true for missing subscriber, want false")
	}

	if err := store.AddSubscriber(ctx, sub); err != nil {
		t.Fatalf("AddSubscriber: %v", err)
	}
	updated, err = store.UpdateSubscriberSettings(ctx, sub, []string{SubscriptionTypeComponent}, []string{"c-api"})
	if err != nil {
		t.Fatalf("UpdateSubscriberSettings: %v", err)
	}
	if !updated {
		t.Fatal("updated = false for existing subscriber, want true")
	}
}

func TestUpdateSubscriberTypesMatchedVsMissing(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	sub := NewSubscriber(1, nil)

	updated, err := store.UpdateSubscriberTypes(ctx, sub, []string{SubscriptionTypeComponent})
	if err != nil {
		t.Fatalf("UpdateSubscriberTypes missing: %v", err)
	}
	if updated {
		t.Fatal("updated = true for missing subscriber, want false")
	}

	if err := store.AddSubscriber(ctx, sub); err != nil {
		t.Fatalf("AddSubscriber: %v", err)
	}
	updated, err = store.UpdateSubscriberTypes(ctx, sub, []string{" COMPONENT ", "invalid"})
	if err != nil {
		t.Fatalf("UpdateSubscriberTypes existing: %v", err)
	}
	if !updated {
		t.Fatal("updated = false for existing subscriber, want true")
	}
	got, _, err := store.GetSubscriber(ctx, sub)
	if err != nil {
		t.Fatalf("GetSubscriber: %v", err)
	}
	if len(got.Types) != 1 || got.Types[0] != SubscriptionTypeComponent {
		t.Fatalf("Types = %v, want normalized [component]", got.Types)
	}
}

func TestComponentStatusRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	if err := store.SaveComponentStatus(ctx, "c1", "degraded_performance"); err != nil {
		t.Fatalf("SaveComponentStatus: %v", err)
	}
	if err := store.SaveComponentStatus(ctx, "c1", "operational"); err != nil {
		t.Fatalf("SaveComponentStatus update: %v", err)
	}
	statuses, err := store.ComponentStatuses(ctx)
	if err != nil {
		t.Fatalf("ComponentStatuses: %v", err)
	}
	if statuses["c1"] != "operational" {
		t.Fatalf("status = %q, want operational", statuses["c1"])
	}
}

func TestPendingComponentEventRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	event := PendingComponentEvent{
		ComponentID:    "c1",
		ComponentName:  "API",
		Status:         "degraded_performance",
		UpdatedAt:      "2026-01-01T00:00:00Z",
		Position:       2,
		PreviousStatus: "operational",
		DeliveryKey:    "component:c1:degraded_performance:2026-01-01T00:00:00Z",
	}
	if err := store.SavePendingComponentEvent(ctx, event); err != nil {
		t.Fatalf("SavePendingComponentEvent: %v", err)
	}
	events, err := store.PendingComponentEvents(ctx)
	if err != nil {
		t.Fatalf("PendingComponentEvents: %v", err)
	}
	if got := events["c1"]; got != event {
		t.Fatalf("event = %+v, want %+v", got, event)
	}
	if err := store.RemovePendingComponentEvent(ctx, "c1"); err != nil {
		t.Fatalf("RemovePendingComponentEvent: %v", err)
	}
	events, err = store.PendingComponentEvents(ctx)
	if err != nil {
		t.Fatalf("PendingComponentEvents after remove: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %v, want empty after remove", events)
	}
}

func TestIncidentUpdateVersionDedup(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	seen, err := store.HasIncidentUpdateVersion(ctx, "u1", "v1")
	if err != nil {
		t.Fatalf("HasIncidentUpdateVersion: %v", err)
	}
	if seen {
		t.Fatal("unseen update reported as seen")
	}
	if err := store.MarkIncidentUpdateVersion(ctx, "u1", "v1"); err != nil {
		t.Fatalf("MarkIncidentUpdateVersion: %v", err)
	}
	seen, err = store.HasIncidentUpdateVersion(ctx, "u1", "v1")
	if err != nil || !seen {
		t.Fatalf("same version seen=%v err=%v, want seen", seen, err)
	}
	// An edited update (new version) must notify again.
	seen, err = store.HasIncidentUpdateVersion(ctx, "u1", "v2")
	if err != nil {
		t.Fatalf("HasIncidentUpdateVersion v2: %v", err)
	}
	if seen {
		t.Fatal("edited update version reported as seen")
	}
}

func TestDeliveryDedupAndClear(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	if err := store.MarkDelivered(ctx, "evt1", "sub-a"); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if err := store.MarkDelivered(ctx, "evt1", "sub-b"); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	var marker struct {
		ExpiresAt time.Time `bson:"expiresAt"`
	}
	if err := store.delivery.FindOne(ctx, bson.M{"_id": "evt1|sub-a"}).Decode(&marker); err != nil {
		t.Fatalf("load delivery marker: %v", err)
	}
	if marker.ExpiresAt.Before(time.Now().Add(deliveryTTL-time.Minute)) || marker.ExpiresAt.After(time.Now().Add(deliveryTTL+time.Minute)) {
		t.Fatalf("expiresAt = %s, want about %s from now", marker.ExpiresAt, deliveryTTL)
	}
	delivered, err := store.DeliveredSubscribers(ctx, "evt1")
	if err != nil {
		t.Fatalf("DeliveredSubscribers: %v", err)
	}
	if !delivered["sub-a"] || !delivered["sub-b"] || len(delivered) != 2 {
		t.Fatalf("delivered = %v, want sub-a and sub-b", delivered)
	}
	if err := store.ClearDelivery(ctx, "evt1"); err != nil {
		t.Fatalf("ClearDelivery: %v", err)
	}
	delivered, err = store.DeliveredSubscribers(ctx, "evt1")
	if err != nil {
		t.Fatalf("DeliveredSubscribers after clear: %v", err)
	}
	if len(delivered) != 0 {
		t.Fatalf("delivered = %v, want empty after clear", delivered)
	}
}

func TestDeliveryTTLIndexExists(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	assertDeliveryTTLIndex(t, ctx, store, int64(deliveryTTLIndexExpireAfterSeconds))
}

func TestEnsureIndexesReplacesLegacyDeliveryTTLIndex(t *testing.T) {
	ctx := context.Background()
	store := testMongoWithoutIndexes(t)
	if _, err := store.delivery.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(604800),
	}); err != nil {
		t.Fatalf("create legacy TTL index: %v", err)
	}
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	assertDeliveryTTLIndex(t, ctx, store, int64(deliveryTTLIndexExpireAfterSeconds))
}

func assertDeliveryTTLIndex(t *testing.T, ctx context.Context, store *Store, want int64) {
	t.Helper()
	cursor, err := store.delivery.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	var indexes []bson.M
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("decode indexes: %v", err)
	}
	found := false
	for _, idx := range indexes {
		if !isExpiresAtIndex(idx["key"]) {
			continue
		}
		if asInt(idx["expireAfterSeconds"]) == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("no expiresAt TTL index with expireAfterSeconds=%d in %v", want, indexes)
	}
}

func asInt(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return -1
	}
}

func TestTelegramOffsetRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	offset, err := store.TelegramOffset(ctx)
	if err != nil {
		t.Fatalf("TelegramOffset: %v", err)
	}
	if offset != 0 {
		t.Fatalf("default offset = %d, want 0", offset)
	}
	if err := store.SaveTelegramOffset(ctx, 42); err != nil {
		t.Fatalf("SaveTelegramOffset: %v", err)
	}
	offset, err = store.TelegramOffset(ctx)
	if err != nil {
		t.Fatalf("TelegramOffset: %v", err)
	}
	if offset != 42 {
		t.Fatalf("offset = %d, want 42", offset)
	}
}

func TestTelegramOffsetAcceptsStringValue(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	if _, err := store.meta.InsertOne(ctx, bson.M{"_id": metaTelegramOffsetID, "value": "42"}); err != nil {
		t.Fatalf("insert string offset: %v", err)
	}
	offset, err := store.TelegramOffset(ctx)
	if err != nil {
		t.Fatalf("TelegramOffset: %v", err)
	}
	if offset != 42 {
		t.Fatalf("offset = %d, want 42", offset)
	}
}

func TestTelegramOffsetSelfHealsInvalidValue(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	if _, err := store.meta.InsertOne(ctx, bson.M{"_id": metaTelegramOffsetID, "value": "bad"}); err != nil {
		t.Fatalf("insert invalid offset: %v", err)
	}
	offset, err := store.TelegramOffset(ctx)
	if err != nil {
		t.Fatalf("TelegramOffset: %v", err)
	}
	if offset != 0 {
		t.Fatalf("offset = %d, want 0", offset)
	}
	count, err := store.meta.CountDocuments(ctx, bson.M{"_id": metaTelegramOffsetID})
	if err != nil {
		t.Fatalf("CountDocuments: %v", err)
	}
	if count != 0 {
		t.Fatalf("invalid offset doc count = %d, want 0", count)
	}
}

func TestInitializedFlag(t *testing.T) {
	ctx := context.Background()
	store := testMongo(t)
	initialized, err := store.IsInitialized(ctx)
	if err != nil {
		t.Fatalf("IsInitialized: %v", err)
	}
	if initialized {
		t.Fatal("fresh store reported initialized")
	}
	if err := store.SetInitialized(ctx); err != nil {
		t.Fatalf("SetInitialized: %v", err)
	}
	initialized, err = store.IsInitialized(ctx)
	if err != nil || !initialized {
		t.Fatalf("IsInitialized after set = %v err=%v, want true", initialized, err)
	}
}
