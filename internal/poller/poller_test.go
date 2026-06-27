package poller

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/mongostore"
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
	subscribers       []mongostore.Subscriber
	delivered         map[string]map[string]bool
	hasDeliveredErr   error
	markDeliveredErr  error
	removeErr         error
	savedComponents   map[string]string
	markedVersions    map[string]string
	pendingComponents map[string]mongostore.PendingComponentEvent
	removed           []string
}

func newFakePollerStore() *fakePollerStore {
	return &fakePollerStore{
		componentStatuses: map[string]string{},
		incidentVersions:  map[string]string{},
		delivered:         map[string]map[string]bool{},
		savedComponents:   map[string]string{},
		markedVersions:    map[string]string{},
		pendingComponents: map[string]mongostore.PendingComponentEvent{},
	}
}

func (f *fakePollerStore) ComponentStatuses(context.Context) (map[string]string, error) {
	return f.componentStatuses, nil
}

func (f *fakePollerStore) ClearDelivery(_ context.Context, eventKey string) error {
	delete(f.delivered, eventKey)
	return nil
}

func (f *fakePollerStore) DeliveredSubscribers(_ context.Context, eventKey string) (map[string]bool, error) {
	if f.hasDeliveredErr != nil {
		return nil, f.hasDeliveredErr
	}
	return f.delivered[eventKey], nil
}

func (f *fakePollerStore) HasIncidentUpdateVersion(_ context.Context, updateID, version string) (bool, error) {
	stored, ok := f.incidentVersions[updateID]
	return ok && stored == version, nil
}

func (f *fakePollerStore) IsInitialized(context.Context) (bool, error) {
	return f.initialized, nil
}

func (f *fakePollerStore) ListSubscribers(context.Context) ([]mongostore.Subscriber, error) {
	return f.subscribers, nil
}

func (f *fakePollerStore) MarkDelivered(_ context.Context, eventKey, subscriberKey string) error {
	if f.markDeliveredErr != nil {
		return f.markDeliveredErr
	}
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

func (f *fakePollerStore) PendingComponentEvents(context.Context) (map[string]mongostore.PendingComponentEvent, error) {
	result := make(map[string]mongostore.PendingComponentEvent, len(f.pendingComponents))
	for key, value := range f.pendingComponents {
		result[key] = value
	}
	return result, nil
}

func (f *fakePollerStore) SavePendingComponentEvent(_ context.Context, event mongostore.PendingComponentEvent) error {
	f.pendingComponents[event.ComponentID] = event
	return nil
}

func (f *fakePollerStore) RemovePendingComponentEvent(_ context.Context, componentID string) error {
	delete(f.pendingComponents, componentID)
	return nil
}

func (f *fakePollerStore) RemoveSubscriber(_ context.Context, sub mongostore.Subscriber) error {
	if f.removeErr != nil {
		return f.removeErr
	}
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
	errors     map[string]error
	errorQueue map[string][]error
	sends      []string
	messages   []string
}

func (f *fakeNotifier) SendMessage(_ context.Context, sub mongostore.Subscriber, text string) error {
	f.sends = append(f.sends, sub.Key())
	f.messages = append(f.messages, text)
	if queued := f.errorQueue[sub.Key()]; len(queued) > 0 {
		err := queued[0]
		f.errorQueue[sub.Key()] = queued[1:]
		return err
	}
	if f.errors != nil {
		return f.errors[sub.Key()]
	}
	return nil
}

func TestCheckOnceDoesNotCheckpointWhenSendFails(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil)}
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
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil), mongostore.NewSubscriber(2, nil)}
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

func TestCheckOnceContinuesFanOutAfterRetryableSendFailures(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.componentStatuses["c2"] = "operational"
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil), mongostore.NewSubscriber(2, nil)}
	notifier := &fakeNotifier{errors: map[string]error{"1": errors.New("rate limit")}}
	runner := NewRunner(fakeStatusClient{summary: openai.Summary{Components: []openai.Component{
		{ID: "c1", Name: "API", Status: "degraded_performance", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "c2", Name: "ChatGPT", Status: "partial_outage", UpdatedAt: "2026-01-01T00:01:00Z"},
	}}}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err == nil {
		t.Fatal("expected retryable send error")
	}
	if got, want := notifier.sends, []string{"1", "2", "2"}; !equalStrings(got, want) {
		t.Fatalf("sends = %v, want %v", got, want)
	}
	if len(store.savedComponents) != 0 {
		t.Fatalf("savedComponents = %v, want no checkpoints after delivery error", store.savedComponents)
	}
}

func TestCheckOnceStopsOnDeliveryStateReadError(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.componentStatuses["c2"] = "operational"
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil)}
	store.hasDeliveredErr = errors.New("store read failed")
	notifier := &fakeNotifier{}
	runner := NewRunner(fakeStatusClient{summary: openai.Summary{Components: []openai.Component{
		{ID: "c1", Name: "API", Status: "degraded_performance", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "c2", Name: "ChatGPT", Status: "partial_outage", UpdatedAt: "2026-01-01T00:01:00Z"},
	}}}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "store read failed") {
		t.Fatalf("CheckOnce error = %v, want delivery state read error", err)
	}
	if len(notifier.sends) != 0 {
		t.Fatalf("sends = %v, want none after delivery state read error", notifier.sends)
	}
}

func TestCheckOnceStopsWhenTerminalSubscriberRemovalFails(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.componentStatuses["c2"] = "operational"
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil)}
	store.removeErr = errors.New("store remove failed")
	notifier := &fakeNotifier{errors: map[string]error{"1": &telegram.APIError{StatusCode: 200, ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"}}}
	runner := NewRunner(fakeStatusClient{summary: openai.Summary{Components: []openai.Component{
		{ID: "c1", Name: "API", Status: "degraded_performance", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "c2", Name: "ChatGPT", Status: "partial_outage", UpdatedAt: "2026-01-01T00:01:00Z"},
	}}}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "store remove failed") {
		t.Fatalf("CheckOnce error = %v, want subscriber remove error", err)
	}
	if got, want := notifier.sends, []string{"1"}; !equalStrings(got, want) {
		t.Fatalf("sends = %v, want %v", got, want)
	}
}

func TestCheckOnceSkipsLaterEventsForSubscriberAfterRetryableFailure(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.componentStatuses["c2"] = "operational"
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil), mongostore.NewSubscriber(2, nil)}
	notifier := &fakeNotifier{errorQueue: map[string][]error{"1": {errors.New("rate limit")}}}
	runner := NewRunner(fakeStatusClient{summary: openai.Summary{Components: []openai.Component{
		{ID: "c1", Name: "API", Status: "degraded_performance", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "c2", Name: "ChatGPT", Status: "partial_outage", UpdatedAt: "2026-01-01T00:01:00Z"},
	}}}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err == nil {
		t.Fatal("expected retryable send error")
	}
	if got, want := notifier.sends, []string{"1", "2", "2"}; !equalStrings(got, want) {
		t.Fatalf("sends = %v, want %v", got, want)
	}
}

func TestCheckOnceRemovesTerminalSubscriberAndCheckpoints(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil), mongostore.NewSubscriber(2, nil)}
	notifier := &fakeNotifier{errors: map[string]error{"2": &telegram.APIError{StatusCode: 200, ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"}}}
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
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil)}
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
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil), mongostore.NewSubscriber(2, nil)}
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

func TestPendingComponentLabelDoesNotCountCurrentComponentAsDuplicate(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.pendingComponents["c1"] = mongostore.PendingComponentEvent{
		ComponentID:    "c1",
		ComponentName:  "API",
		Status:         "degraded_performance",
		UpdatedAt:      "2026-01-01T00:00:00Z",
		PreviousStatus: "operational",
		DeliveryKey:    "component:c1:degraded_performance:2026-01-01T00:00:00Z",
	}
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil)}
	notifier := &fakeNotifier{}
	runner := NewRunner(fakeStatusClient{summary: summaryWithComponent("c1", "API", "degraded_performance")}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err != nil {
		t.Fatalf("CheckOnce returned error: %v", err)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("messages = %v, want one message", notifier.messages)
	}
	if strings.Contains(notifier.messages[0], "(ID:") {
		t.Fatalf("message counted current component as duplicate: %s", notifier.messages[0])
	}
}

func TestPendingComponentDuplicateLabelsIncludeCurrentRename(t *testing.T) {
	pending := map[string]mongostore.PendingComponentEvent{
		"c1": {
			ComponentID:   "c1",
			ComponentName: "Old API",
			Status:        "degraded_performance",
		},
	}
	components := []openai.Component{
		{ID: "c1", Name: "API", Status: "degraded_performance"},
		{ID: "c2", Name: "API", Status: "operational"},
	}

	duplicates := DuplicateComponentNames(componentsForDuplicateLabels(components, pending))
	if !duplicates["API"] {
		t.Fatalf("duplicates = %v, want current API rename counted as duplicate", duplicates)
	}
	if duplicates["Old API"] {
		t.Fatalf("duplicates = %v, old pending name should not be duplicate", duplicates)
	}
}

func TestCheckOnceCheckpointsDeliveredEventWhenSiblingEventFails(t *testing.T) {
	// Regression: a fully-delivered event must checkpoint even when another
	// event in the same poll fails to deliver. Previously any delivery failure
	// aborted checkpoints for the whole batch, re-emitting delivered events.
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.componentStatuses["c2"] = "operational"
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil)}
	// c1 (sorts first) succeeds; c2 fails for the same subscriber.
	notifier := &fakeNotifier{errorQueue: map[string][]error{"1": {nil, errors.New("rate limit")}}}
	runner := NewRunner(fakeStatusClient{summary: openai.Summary{Components: []openai.Component{
		{ID: "c1", Name: "API", Status: "degraded_performance", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "c2", Name: "ChatGPT", Status: "partial_outage", UpdatedAt: "2026-01-01T00:01:00Z"},
	}}}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err == nil {
		t.Fatal("expected delivery error from c2")
	}
	if got := store.savedComponents["c1"]; got != "degraded_performance" {
		t.Fatalf("c1 checkpoint = %q, want delivered event checkpointed", got)
	}
	if got, ok := store.savedComponents["c2"]; ok {
		t.Fatalf("c2 checkpoint = %q, want failed event NOT checkpointed", got)
	}
	if _, ok := store.pendingComponents["c2"]; !ok {
		t.Fatal("c2 pending marker should persist for retry")
	}
	if _, ok := store.pendingComponents["c1"]; ok {
		t.Fatal("c1 pending marker should be cleared after delivery")
	}
}

func TestCheckOnceContinuesWhenMarkDeliveredFailsAfterSend(t *testing.T) {
	store := newFakePollerStore()
	store.initialized = true
	store.componentStatuses["c1"] = "operational"
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil)}
	store.markDeliveredErr = errors.New("store write failed")
	notifier := &fakeNotifier{}
	runner := NewRunner(fakeStatusClient{summary: summaryWithComponent("c1", "API", "degraded_performance")}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err != nil {
		t.Fatalf("CheckOnce returned error: %v", err)
	}
	if len(notifier.sends) != 1 || notifier.sends[0] != "1" {
		t.Fatalf("sends = %v, want [1]", notifier.sends)
	}
	if got := store.savedComponents["c1"]; got != "degraded_performance" {
		t.Fatalf("component checkpoint = %q", got)
	}
}

func TestIncidentUpdateVersionIgnoresUpdatedAtRefresh(t *testing.T) {
	base := openai.IncidentUpdate{ID: "u1", Status: "monitoring", Body: "same", DisplayAt: "2026-01-01T00:00:00Z", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:01:00Z"}
	refreshed := base
	refreshed.UpdatedAt = "2026-01-01T00:02:00Z"

	if got, want := IncidentUpdateVersion(refreshed), IncidentUpdateVersion(base); got != want {
		t.Fatalf("version changed for UpdatedAt-only refresh: got %s, want %s", got, want)
	}
}

func TestCheckOnceDoesNotRemarkSeenIncidentVersion(t *testing.T) {
	update := openai.IncidentUpdate{ID: "u1", Body: "same", UpdatedAt: "2026-01-01T00:00:00Z"}
	store := newFakePollerStore()
	store.initialized = true
	store.incidentVersions["u1"] = IncidentUpdateVersion(update)
	store.subscribers = []mongostore.Subscriber{mongostore.NewSubscriber(1, nil)}
	notifier := &fakeNotifier{}
	runner := NewRunner(fakeStatusClient{incidents: openai.IncidentsResponse{Incidents: []openai.Incident{{ID: "i1", Name: "incident", Impact: "minor", IncidentUpdates: []openai.IncidentUpdate{update}}}}}, store, notifier, time.Minute, slog.Default())

	if err := runner.CheckOnce(context.Background()); err != nil {
		t.Fatalf("CheckOnce returned error: %v", err)
	}
	if len(notifier.sends) != 0 {
		t.Fatalf("sends = %v, want none", notifier.sends)
	}
	if len(store.markedVersions) != 0 {
		t.Fatalf("markedVersions = %v, want none", store.markedVersions)
	}
}

func summaryWithComponent(id, name, status string) openai.Summary {
	return openai.Summary{Components: []openai.Component{{ID: id, Name: name, Status: status, UpdatedAt: "2026-01-01T00:00:00Z"}}}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
