package mongostore

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

func TestNormalizeTypesDedupesAndDefaults(t *testing.T) {
	got := normalizeTypes([]string{"incident", "component", "incident"})
	if len(got) != 2 || got[0] != SubscriptionTypeIncident || got[1] != SubscriptionTypeComponent {
		t.Fatalf("normalizeTypes = %v", got)
	}
	if def := normalizeTypes(nil); len(def) != 2 {
		t.Fatalf("normalizeTypes(nil) = %v, want defaults", def)
	}
	if def := normalizeTypes([]string{"bogus"}); len(def) != 2 {
		t.Fatalf("normalizeTypes(bogus) = %v, want defaults", def)
	}
}

func TestNormalizeComponentsTrimsAndDedupes(t *testing.T) {
	got := normalizeComponents([]string{" api ", "API", ""})
	if len(got) != 1 || got[0] != "api" {
		t.Fatalf("normalizeComponents = %v, want [api]", got)
	}
}
