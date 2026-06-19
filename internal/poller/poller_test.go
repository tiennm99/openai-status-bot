package poller

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/redisstore"
	"github.com/tiennm99/openai-status-bot/internal/telegram"
)

type fakeStatusClient struct {
	summary   openai.Summary
	incidents openai.IncidentsResponse
}

func (f fakeStatusClient) FetchSummary(context.Context) (openai.Summary, error) {
	return f.summary, nil
}

func (f fakeStatusClient) FetchIncidents(context.Context) (openai.IncidentsResponse, error) {
	return f.incidents, nil
}

type fakePollerStore struct {
	initialized       bool
	componentStatuses map[string]string
	incidentVersions  map[string]string
	subscribers       []redisstore.Subscriber
	delivered         map[string]map[string]bool
	savedComponents   map[string]string
	markedVersions    map[string]string
	pendingComponents map[string]redisstore.PendingComponentEvent
	removed           []string
}

func newFakePollerStore() *fakePollerStore {
	return &fakePollerStore{
		componentStatuses: map[string]string{},
		incidentVersions:  map[string]string{},
		delivered:         map[string]map[string]bool{},
		savedComponents:   map[string]string{},
		markedVersions:    map[string]string{},
		pendingComponents: map[string]redisstore.PendingComponentEvent{},
	}
}

func (f *fakePollerStore) ComponentStatuses(context.Context) (map[string]string, error) {
	return f.componentStatuses, nil
}

func (f *fakePollerStore) ClearDelivery(_ context.Context, eventKey string) error {
	delete(f.delivered, eventKey)
	return nil
}

func (f *fakePollerStore) HasDelivered(_ context.Context, eventKey, subscriberKey string) (bool, error) {
	return f.delivered[eventKey][subscriberKey], nil
}

func (f *fakePollerStore) HasIncidentUpdateVersion(_ context.Context, updateID, version string) (bool, error) {
	stored, ok := f.incidentVersions[updateID]
	return ok && stored == version, nil
}

func (f *fakePollerStore) IsInitialized(context.Context) (bool, error) {
	return f.initialized, nil
}

func (f *fakePollerStore) ListSubscribers(context.Context) ([]redisstore.Subscriber, error) {
	return f.subscribers, nil
}

func (f *fakePollerStore) MarkDelivered(_ context.Context, eventKey, subscriberKey string) error {
	if f.delivered[eventKey] == nil {
		f.delivered[eventKey] = map[string]bool{}
	}
	f.delivered[eventKey][subscriberKey] = true
	return nil
}

func (f *fakePollerStore) MarkIncidentUpdateVersion(_ context.Context, updateID, version string) error {
	f.incidentVersions[updateID] = version
	f.markedVersions[updateID] = version
	return nil
}

func (f *fakePollerStore) PendingComponentEvents(context.Context) (map[string]redisstore.PendingComponentEvent, error) {
	result := make(map[string]redisstore.PendingComponentEvent, len(f.pendingComponents))
	for key, value := range f.pendingComponents {
		result[key] = value
	}
	return result, nil
}

func (f *fakePollerStore) SavePendingComponentEvent(_ context.Context, event redisstore.PendingComponentEvent) error {
	f.pendingComponents[event.ComponentID] = event
	return nil
}

func (f *fakePollerStore) RemovePendingComponentEvent(_ context.Context, componentID string) error {
	delete(f.pendingComponents, componentID)
	return nil
}

func (f *fakePollerStore) RemoveSubscriber(_ context.Context, sub redisstore.Subscriber) error {
	f.removed = append(f.removed, sub.Key())
	return nil
}

func (f *fakePollerStore) SaveComponentStatus(_ context.Context, componentID, status string) error {
	f.componentStatuses[componentID] = status
	f.savedComponents[componentID] = status
	return nil
}

func (f *fakePollerStore) SetInitialized(context.Context) error {
	f.initialized = true
	return nil
}

type fakeNotifier struct {
	errors map[string]error
	sends  []string
}

func (f *fakeNotifier) SendMessage(_ context.Context, sub redisstore.Subscriber, _ string) error {
	f.sends = append(f.sends, sub.Key())
	if f.errors != nil {
		return f.errors[sub.Key()]
	}
	return nil
}

func TestCheckOnceDoesNotCheckpointWhenSendFails(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.subscribers = []redisstore.Subscriber{redisstore.NewSubscriber(1, nil)}
	notifier := &fakeNotifier{errors: map[string]error{"1": errors.New("network")}}
	runner := NewRunner(fakeStatusClient{summary: summaryWithComponent("c1", "API", "degraded_performance")}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err == nil {
		t.Fatal("expected send error")
	}
	if got := store.savedComponents["c1"]; got != "" {
		t.Fatalf("component checkpoint = %q, want empty", got)
	}
}

func TestCheckOnceSkipsAlreadyDeliveredSubscriberOnRetry(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.subscribers = []redisstore.Subscriber{redisstore.NewSubscriber(1, nil), redisstore.NewSubscriber(2, nil)}
	notifier := &fakeNotifier{errors: map[string]error{"2": errors.New("rate limit")}}
	runner := NewRunner(fakeStatusClient{summary: summaryWithComponent("c1", "API", "degraded_performance")}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err == nil {
		t.Fatal("expected first send error")
	}
	if len(notifier.sends) != 2 || notifier.sends[0] != "1" || notifier.sends[1] != "2" {
		t.Fatalf("first sends = %v, want [1 2]", notifier.sends)
	}
	notifier.errors = nil
	notifier.sends = nil
	if err := runner.CheckOnce(context.Background()); err != nil {
		t.Fatalf("second CheckOnce returned error: %v", err)
	}
	if len(notifier.sends) != 1 || notifier.sends[0] != "2" {
		t.Fatalf("second sends = %v, want [2]", notifier.sends)
	}
	if got := store.savedComponents["c1"]; got != "degraded_performance" {
		t.Fatalf("component checkpoint = %q", got)
	}
}

func TestCheckOnceRemovesTerminalSubscriberAndCheckpoints(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.subscribers = []redisstore.Subscriber{redisstore.NewSubscriber(1, nil), redisstore.NewSubscriber(2, nil)}
	notifier := &fakeNotifier{errors: map[string]error{"2": &telegram.APIError{StatusCode: 403, Description: "Forbidden: bot was blocked by the user"}}}
	runner := NewRunner(fakeStatusClient{summary: summaryWithComponent("c1", "API", "degraded_performance")}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err != nil {
		t.Fatalf("CheckOnce returned error: %v", err)
	}
	if len(store.removed) != 1 || store.removed[0] != "2" {
		t.Fatalf("removed = %v, want [2]", store.removed)
	}
	if got := store.savedComponents["c1"]; got != "degraded_performance" {
		t.Fatalf("component checkpoint = %q", got)
	}
}

func TestIncidentUpdateVersionEditSendsAgain(t *testing.T) {
	update := openai.IncidentUpdate{ID: "u1", Body: "old", UpdatedAt: "2026-01-01T00:00:00Z"}
	store := newFakePollerStore()
	store.initialized = true
	store.incidentVersions["u1"] = IncidentUpdateVersion(update)
	store.subscribers = []redisstore.Subscriber{redisstore.NewSubscriber(1, nil)}
	updated := openai.IncidentUpdate{ID: "u1", Body: "new", UpdatedAt: "2026-01-01T00:01:00Z"}
	notifier := &fakeNotifier{}
	runner := NewRunner(fakeStatusClient{incidents: openai.IncidentsResponse{Incidents: []openai.Incident{{ID: "i1", Name: "incident", Impact: "minor", IncidentUpdates: []openai.IncidentUpdate{updated}}}}}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err != nil {
		t.Fatalf("CheckOnce returned error: %v", err)
	}
	if len(notifier.sends) != 1 {
		t.Fatalf("sends = %v, want one send", notifier.sends)
	}
	if got := store.markedVersions["u1"]; got != IncidentUpdateVersion(updated) {
		t.Fatalf("marked version = %q", got)
	}
}

func TestPendingComponentEventSurvivesReturnToPreviousStatus(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.subscribers = []redisstore.Subscriber{redisstore.NewSubscriber(1, nil), redisstore.NewSubscriber(2, nil)}
	notifier := &fakeNotifier{errors: map[string]error{"2": errors.New("rate limit")}}
	runner := NewRunner(fakeStatusClient{summary: summaryWithComponent("c1", "API", "degraded_performance")}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err == nil {
		t.Fatal("expected first send error")
	}
	if len(store.pendingComponents) != 1 {
		t.Fatalf("pending components = %v, want one", store.pendingComponents)
	}

	notifier.errors = nil
	notifier.sends = nil
	runner.statusClient = fakeStatusClient{summary: summaryWithComponent("c1", "API", "operational")}
	if err := runner.CheckOnce(context.Background()); err != nil {
		t.Fatalf("second CheckOnce returned error: %v", err)
	}
	if len(notifier.sends) != 1 || notifier.sends[0] != "2" {
		t.Fatalf("second sends = %v, want retry only for subscriber 2", notifier.sends)
	}
	if got := store.savedComponents["c1"]; got != "degraded_performance" {
		t.Fatalf("component checkpoint = %q, want pending degraded status", got)
	}
	if len(store.pendingComponents) != 0 {
		t.Fatalf("pending components = %v, want cleared", store.pendingComponents)
	}

	notifier.sends = nil
	if err := runner.CheckOnce(context.Background()); err != nil {
		t.Fatalf("third CheckOnce returned error: %v", err)
	}
	if len(notifier.sends) != 2 {
		t.Fatalf("third sends = %v, want recovery sent to both subscribers", notifier.sends)
	}
	if got := store.savedComponents["c1"]; got != "operational" {
		t.Fatalf("component checkpoint = %q, want recovery status", got)
	}
}

func summaryWithComponent(id, name, status string) openai.Summary {
	return openai.Summary{Components: []openai.Component{{ID: id, Name: name, Status: status, UpdatedAt: "2026-01-01T00:00:00Z"}}}
}
