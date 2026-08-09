package crawl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

type recordingNotifier struct {
	messages []string
	failAt   int
}

func (r *recordingNotifier) Send(ctx context.Context, message string) error {
	if r.failAt > 0 && len(r.messages) == r.failAt-1 {
		return errors.New("twilio unavailable")
	}
	r.messages = append(r.messages, message)
	return nil
}

func matchListing(id string) model.Listing {
	year := 2015
	cents := int64(7200000)
	return model.Listing{
		Source:     "olx",
		ExternalID: id,
		URL:        "https://example.com",
		Title:      "Harley Street Glide",
		Bike:       model.BikeStreetGlide,
		Year:       &year,
		PriceCents: &cents,
		City:       "curitiba",
		State:      "PR",
		Verdict:    model.VerdictMatch,
	}
}

func storeWithMatches(t *testing.T, count int) *store.Store {
	t.Helper()
	s := openStore(t)
	for i := 0; i < count; i++ {
		if _, err := s.Upsert(matchListing(string(rune('a'+i))), time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	return s
}

func TestNotifyRespectsPerRunCap(t *testing.T) {
	s := storeWithMatches(t, 12)
	n := &recordingNotifier{}

	sent, err := Notify(context.Background(), s, n, 5)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 5 {
		t.Errorf("sent = %d, want 5", sent)
	}
	if len(n.messages) != 5 {
		t.Errorf("delivered %d messages, want 5", len(n.messages))
	}

	pending, err := s.PendingNotifications(20)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 7 {
		t.Errorf("%d rows left pending, want 7", len(pending))
	}
}

func TestNotifyDoesNotMarkOnFailure(t *testing.T) {
	s := storeWithMatches(t, 2)
	n := &recordingNotifier{failAt: 1}

	if _, err := Notify(context.Background(), s, n, 5); err == nil {
		t.Fatal("Notify should surface the delivery error")
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("failed delivery must leave rows pending, got %d", len(pending))
	}
}

func TestNotifyKeepsDeliveredRowsMarkedAfterAPartialFailure(t *testing.T) {
	s := storeWithMatches(t, 4)
	n := &recordingNotifier{failAt: 3}

	sent, err := Notify(context.Background(), s, n, 5)
	if err == nil {
		t.Fatal("Notify should surface the delivery error")
	}
	if sent != 2 {
		t.Errorf("sent = %d, want the 2 messages that went through", sent)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("%d rows left pending, want the 2 undelivered ones", len(pending))
	}
}

func TestNotifyDeduplicatesByFingerprint(t *testing.T) {
	s := openStore(t)

	olx := matchListing("olx-1")
	olx.Fingerprint = "street_glide|special|2015|72"

	ml := matchListing("ml-1")
	ml.Source = "mercadolivre"
	ml.Fingerprint = olx.Fingerprint

	other := matchListing("olx-2")
	other.Fingerprint = "road_glide|base|2014|65"

	for _, l := range []model.Listing{olx, ml, other} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2: the duplicate must not become a second sms", sent)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("%d rows left pending, want 0: the duplicate is marked without sending", len(pending))
	}
}

func TestNotifyTreatsEmptyFingerprintsAsDistinct(t *testing.T) {
	s := storeWithMatches(t, 3)
	n := &recordingNotifier{}

	sent, err := Notify(context.Background(), s, n, 5)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 3 {
		t.Errorf("sent = %d, want 3: an empty fingerprint is not evidence of duplication", sent)
	}
}

func TestNotifyDefaultsTheCapWhenUnset(t *testing.T) {
	s := storeWithMatches(t, 9)
	n := &recordingNotifier{}

	sent, err := Notify(context.Background(), s, n, 0)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != DefaultSMSPerRun {
		t.Errorf("sent = %d, want the default cap of %d", sent, DefaultSMSPerRun)
	}
}

func TestNotifyIsQuietWithNothingPending(t *testing.T) {
	s := openStore(t)
	n := &recordingNotifier{}

	sent, err := Notify(context.Background(), s, n, 5)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 0 || len(n.messages) != 0 {
		t.Errorf("sent = %d with %d messages, want a silent round", sent, len(n.messages))
	}
}
