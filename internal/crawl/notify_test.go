package crawl

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/notify"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

type recordingNotifier struct {
	messages []string
	alerts   []notify.Alert
	failAt   int
}

func (r *recordingNotifier) Send(ctx context.Context, alert notify.Alert) error {
	if r.failAt > 0 && len(r.messages) == r.failAt-1 {
		return errors.New("notification refused")
	}
	r.messages = append(r.messages, alert.Message)
	r.alerts = append(r.alerts, alert)
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

func withKm(l model.Listing, km int) model.Listing {
	l.Km = &km
	return l
}

func withPrice(l model.Listing, cents int64) model.Listing {
	l.PriceCents = &cents
	return l
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

	sent, err := Notify(context.Background(), s, n, 5, nil)
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

	if _, err := Notify(context.Background(), s, n, 5, nil); err == nil {
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

	sent, err := Notify(context.Background(), s, n, 5, nil)
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

	olx := withKm(matchListing("olx-1"), 90195)
	olx.Fingerprint = "b5778ec73e0cac33"

	ml := withKm(matchListing("ml-1"), 90195)
	ml.Source = "mercadolivre"
	ml.Fingerprint = olx.Fingerprint

	other := withKm(matchListing("olx-2"), 37234)
	other.Fingerprint = "b469ca445d30ab1c"

	for _, l := range []model.Listing{olx, ml, other} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
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

func TestNotifyKeepsBothWhenMileageIsUnknown(t *testing.T) {
	s := openStore(t)

	cheaper := withPrice(matchListing("olx-3"), 7200000)
	cheaper.Title = "Harley-Davidson Street Glide 2014"
	cheaper.Fingerprint = "416f75cb1a66a44e"

	dearer := withPrice(matchListing("olx-15"), 7500000)
	dearer.Title = "Stret glide excelente estado"
	dearer.Fingerprint = cheaper.Fingerprint

	for _, l := range []model.Listing{cheaper, dearer} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2: without mileage the fingerprint cannot tell two bikes apart", sent)
	}
}

func TestNotifyKeepsBothWhenTheSameSourceRepeatsAFingerprint(t *testing.T) {
	s := openStore(t)

	first := withKm(matchListing("wm-237"), 53000)
	first.Source = "webmotors"
	first.Fingerprint = "aa11bb22cc33dd44"

	second := withKm(matchListing("wm-251"), 53118)
	second.Source = "webmotors"
	second.Fingerprint = first.Fingerprint

	for _, l := range []model.Listing{first, second} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2: two active ads on one source are two bikes", sent)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("%d rows left pending, want 0: both were alerted", len(pending))
	}
}

func TestNotifyKeepsTheSecondSameSourceAdAfterACrossPost(t *testing.T) {
	s := openStore(t)

	onOlx := withKm(matchListing("olx-237"), 53000)
	onOlx.Fingerprint = "aa11bb22cc33dd44"

	mirrored := withKm(matchListing("wm-237"), 53000)
	mirrored.Source = "webmotors"
	mirrored.Fingerprint = onOlx.Fingerprint

	otherBike := withKm(matchListing("wm-251"), 53118)
	otherBike.Source = "webmotors"
	otherBike.Fingerprint = onOlx.Fingerprint

	for _, l := range []model.Listing{onOlx, mirrored, otherBike} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2: the olx cross-post must not silence a second webmotors bike", sent)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("%d rows left pending, want 0", len(pending))
	}
}

func TestNotifyDedupSkipsDoNotConsumeCapSlots(t *testing.T) {
	s := openStore(t)

	crossPosted := []string{"olx", "mercadolivre", "webmotors"}
	var listings []model.Listing
	for i, src := range crossPosted {
		l := withKm(matchListing(fmt.Sprintf("dup-%d", i)), 90195)
		l.Source = src
		l.Fingerprint = "b5778ec73e0cac33"
		listings = append(listings, l)
	}
	for i := 0; i < 5; i++ {
		l := withKm(matchListing(fmt.Sprintf("uniq-%d", i)), 10000+i)
		l.Fingerprint = fmt.Sprintf("unique-%d", i)
		listings = append(listings, l)
	}
	for _, l := range listings {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 5 {
		t.Errorf("sent = %d, want 5: the two cross-post skips must not eat cap slots", sent)
	}

	pending, err := s.PendingNotifications(50)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("%d rows left pending, want 1: 3 cross-posts collapse to 1 send, 5 sends spend the cap", len(pending))
	}
}

func TestNotifyTreatsEmptyFingerprintsAsDistinct(t *testing.T) {
	s := storeWithMatches(t, 3)
	n := &recordingNotifier{}

	sent, err := Notify(context.Background(), s, n, 5, nil)
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

	sent, err := Notify(context.Background(), s, n, 0, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != DefaultAlertsPerRun {
		t.Errorf("sent = %d, want the default cap of %d", sent, DefaultAlertsPerRun)
	}
}

func TestNotifyIsQuietWithNothingPending(t *testing.T) {
	s := openStore(t)
	n := &recordingNotifier{}

	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 0 || len(n.messages) != 0 {
		t.Errorf("sent = %d with %d messages, want a silent round", sent, len(n.messages))
	}
}

func TestNotifyCarriesTheListingURLOnTheAlert(t *testing.T) {
	s := openStore(t)
	l := matchListing("olx-1")
	l.URL = "https://pr.olx.com.br/regiao-de-curitiba/motos/harley-1509210244"
	if _, err := s.Upsert(l, time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	n := &recordingNotifier{}
	if _, err := Notify(context.Background(), s, n, 5, nil); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(n.alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(n.alerts))
	}
	if n.alerts[0].URL != l.URL {
		t.Errorf("alert url = %q, want %q so the click opens the listing", n.alerts[0].URL, l.URL)
	}
}
