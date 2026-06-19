package redisstore

import "testing"

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
