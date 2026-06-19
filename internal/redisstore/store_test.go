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
