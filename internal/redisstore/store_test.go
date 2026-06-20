package redisstore

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSubscriberKeyRoundTripWithoutThread(t *testing.T) {
	sub := NewSubscriber(12345, nil)
	parsed, err := ParseSubscriberKey(sub.Key())
	if err != nil {
		t.Fatalf("ParseSubscriberKey returned error: %v", err)
	}
	if parsed.ChatID != sub.ChatID || parsed.ThreadID != nil {
		t.Fatalf("parsed subscriber = %+v, want %+v", parsed, sub)
	}
}

func TestSubscriberKeyRoundTripWithThreadZero(t *testing.T) {
	threadID := 0
	sub := NewSubscriber(-10012345, &threadID)
	parsed, err := ParseSubscriberKey(sub.Key())
	if err != nil {
		t.Fatalf("ParseSubscriberKey returned error: %v", err)
	}
	if parsed.ChatID != sub.ChatID || parsed.ThreadID == nil || *parsed.ThreadID != threadID {
		t.Fatalf("parsed subscriber = %+v, want thread ID %d", parsed, threadID)
	}
}

func TestParseSubscriberKeyRejectsInvalidValue(t *testing.T) {
	if _, err := ParseSubscriberKey("abc:def:ghi"); err == nil {
		t.Fatal("expected invalid key error")
	}
}

func TestSubscriberAcceptsComponentIDAndLegacyNameFilters(t *testing.T) {
	sub := NewSubscriber(12345, nil)
	sub.Components = []string{"component-id", "Legacy API Name"}
	if !sub.Accepts(SubscriptionTypeComponent, "component-id", "API") {
		t.Fatal("expected component ID filter to match")
	}
	if !sub.Accepts(SubscriptionTypeComponent, "other-id", "legacy api name") {
		t.Fatal("expected legacy component name filter to match")
	}
	if sub.Accepts(SubscriptionTypeComponent, "other-id", "Other") {
		t.Fatal("unexpected component filter match")
	}
}

func TestDecodePendingComponentEventBackfillsComponentID(t *testing.T) {
	event, err := decodePendingComponentEvent("c1", `{"component_name":"API","status":"operational"}`)
	if err != nil {
		t.Fatalf("decodePendingComponentEvent returned error: %v", err)
	}
	if event.ComponentID != "c1" {
		t.Fatalf("ComponentID = %q, want c1", event.ComponentID)
	}
}

func TestDecodePendingComponentEventRejectsCorruptedPayload(t *testing.T) {
	if _, err := decodePendingComponentEvent("c1", `{bad json`); err == nil {
		t.Fatal("expected corrupted pending event error")
	}
}

func TestDecodeSubscriberSettingsNormalizesValues(t *testing.T) {
	settings, err := decodeSubscriberSettings("123", `{"types":["incident","component","incident"],"components":[" api ","API",""]}`)
	if err != nil {
		t.Fatalf("decodeSubscriberSettings returned error: %v", err)
	}
	if len(settings.Types) != 2 || settings.Types[0] != SubscriptionTypeIncident || settings.Types[1] != SubscriptionTypeComponent {
		t.Fatalf("Types = %v", settings.Types)
	}
	if len(settings.Components) != 1 || settings.Components[0] != "api" {
		t.Fatalf("Components = %v", settings.Components)
	}
}

func TestDecodeSubscriberSettingsRejectsCorruptedPayload(t *testing.T) {
	if _, err := decodeSubscriberSettings("123", `{bad json`); err == nil {
		t.Fatal("expected corrupted subscriber settings error")
	}
}

func TestListSubscribersRecoversCorruptSettingsWithoutBlockingOthers(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	good := NewSubscriber(1, nil)
	corrupt := NewSubscriber(2, nil)
	if err := client.SAdd(ctx, subscribersKey, good.Key(), corrupt.Key()).Err(); err != nil {
		t.Fatalf("SAdd subscribers: %v", err)
	}
	if err := client.HSet(ctx, subscriberSettingsKey, good.Key(), `{"types":["incident"],"components":[]}`).Err(); err != nil {
		t.Fatalf("HSet good settings: %v", err)
	}
	if err := client.HSet(ctx, subscriberSettingsKey, corrupt.Key(), `{bad json`).Err(); err != nil {
		t.Fatalf("HSet corrupt settings: %v", err)
	}

	subscribers, err := store.ListSubscribers(ctx)
	if err != nil {
		t.Fatalf("ListSubscribers returned error: %v", err)
	}
	byKey := map[string]Subscriber{}
	for _, subscriber := range subscribers {
		byKey[subscriber.Key()] = subscriber
	}
	if len(byKey) != 2 {
		t.Fatalf("subscribers = %v, want two subscribers", subscribers)
	}
	if got := byKey[good.Key()].Types; len(got) != 1 || got[0] != SubscriptionTypeIncident {
		t.Fatalf("good Types = %v, want [incident]", got)
	}
	if got := byKey[corrupt.Key()].Types; len(got) != 2 {
		t.Fatalf("corrupt Types = %v, want default types", got)
	}
	exists, err := client.HExists(ctx, subscriberSettingsKey, corrupt.Key()).Result()
	if err != nil {
		t.Fatalf("HExists corrupt settings: %v", err)
	}
	if exists {
		t.Fatal("corrupt settings field still exists")
	}
}

func TestUpdateSubscriberSettingsSavesTypesAndComponentsTogether(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)
	sub := NewSubscriber(1, nil)
	if err := store.AddSubscriber(ctx, sub); err != nil {
		t.Fatalf("AddSubscriber returned error: %v", err)
	}

	updated, err := store.UpdateSubscriberSettings(ctx, sub, []string{SubscriptionTypeComponent}, []string{"c-api"})
	if err != nil {
		t.Fatalf("UpdateSubscriberSettings returned error: %v", err)
	}
	if !updated {
		t.Fatal("UpdateSubscriberSettings updated = false, want true")
	}
	got, exists, err := store.GetSubscriber(ctx, sub)
	if err != nil {
		t.Fatalf("GetSubscriber returned error: %v", err)
	}
	if !exists {
		t.Fatal("subscriber missing after update")
	}
	if len(got.Types) != 1 || got.Types[0] != SubscriptionTypeComponent {
		t.Fatalf("Types = %v, want [component]", got.Types)
	}
	if len(got.Components) != 1 || got.Components[0] != "c-api" {
		t.Fatalf("Components = %v, want [c-api]", got.Components)
	}
}

func TestUpdateSubscriberSettingsReturnsFalseForMissingSubscriber(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	sub := NewSubscriber(1, nil)

	updated, err := store.UpdateSubscriberSettings(ctx, sub, []string{SubscriptionTypeComponent}, []string{"c-api"})
	if err != nil {
		t.Fatalf("UpdateSubscriberSettings returned error: %v", err)
	}
	if updated {
		t.Fatal("UpdateSubscriberSettings updated = true, want false")
	}
	exists, err := client.HExists(ctx, subscriberSettingsKey, sub.Key()).Result()
	if err != nil {
		t.Fatalf("HExists settings: %v", err)
	}
	if exists {
		t.Fatal("settings were written for missing subscriber")
	}
}

func TestHasIncidentUpdateVersionMigratesLegacySeenUpdate(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	if err := client.SAdd(ctx, incidentUpdatesKey, "u1").Err(); err != nil {
		t.Fatalf("SAdd legacy incident update: %v", err)
	}

	seen, err := store.HasIncidentUpdateVersion(ctx, "u1", "v1")
	if err != nil {
		t.Fatalf("HasIncidentUpdateVersion returned error: %v", err)
	}
	if !seen {
		t.Fatal("legacy incident update was not treated as seen")
	}
	stored, err := client.HGet(ctx, incidentVersionsKey, "u1").Result()
	if err != nil {
		t.Fatalf("HGet migrated incident version: %v", err)
	}
	if stored != "v1" {
		t.Fatalf("stored version = %q, want v1", stored)
	}

	seen, err = store.HasIncidentUpdateVersion(ctx, "u1", "v2")
	if err != nil {
		t.Fatalf("HasIncidentUpdateVersion changed version returned error: %v", err)
	}
	if seen {
		t.Fatal("changed incident update version should not be treated as seen")
	}
}

func TestTelegramOffsetClearsInvalidStoredValue(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	if err := client.Set(ctx, telegramOffsetKey, "not-an-offset", 0).Err(); err != nil {
		t.Fatalf("Set invalid telegram offset: %v", err)
	}

	offset, err := store.TelegramOffset(ctx)
	if err != nil {
		t.Fatalf("TelegramOffset returned error: %v", err)
	}
	if offset != 0 {
		t.Fatalf("offset = %d, want 0", offset)
	}
	exists, err := client.Exists(ctx, telegramOffsetKey).Result()
	if err != nil {
		t.Fatalf("Exists telegram offset: %v", err)
	}
	if exists != 0 {
		t.Fatal("invalid telegram offset key was not cleared")
	}
}

func newTestStore(t *testing.T) (*Store, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return New(client), client
}
