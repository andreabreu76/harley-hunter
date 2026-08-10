package crawl

import (
	"context"
	"errors"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func statusOf(t *testing.T, s *store.Store, externalID string) string {
	t.Helper()
	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	for _, row := range rows {
		if row.ExternalID == externalID {
			return row.Status
		}
	}
	t.Fatalf("listing %q is not stored", externalID)
	return ""
}

func TestRunExpiresAListingMissedForThreeRounds(t *testing.T) {
	s := openStore(t)
	both := fakeSource{name: "olx", items: []model.RawListing{harley("olx", "leaving"), harley("olx", "staying")}}
	remaining := fakeSource{name: "olx", items: []model.RawListing{harley("olx", "staying")}}

	if _, err := Run(context.Background(), []Source{both}, s, testConfig()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	for i := range 2 {
		report, err := Run(context.Background(), []Source{remaining}, s, testConfig())
		if err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
		if report.Expired != 0 {
			t.Fatalf("round %d expired %d listings, want 0: two misses are not three", i+2, report.Expired)
		}
		if got := statusOf(t, s, "leaving"); got != store.StatusActive {
			t.Fatalf("after round %d the missing listing is %q, want %q", i+2, got, store.StatusActive)
		}
	}

	report, err := Run(context.Background(), []Source{remaining}, s, testConfig())
	if err != nil {
		t.Fatalf("fourth Run: %v", err)
	}
	if report.Expired != 1 {
		t.Errorf("Expired = %d, want 1", report.Expired)
	}
	if got := statusOf(t, s, "leaving"); got != store.StatusGone {
		t.Errorf("missing listing status = %q, want %q", got, store.StatusGone)
	}
	if got := statusOf(t, s, "staying"); got != store.StatusActive {
		t.Errorf("listing seen every round has status %q, want %q", got, store.StatusActive)
	}
}

func TestRunKeepsListingsAliveWhileTheSourceIsBroken(t *testing.T) {
	s := openStore(t)
	working := fakeSource{name: "olx", items: []model.RawListing{harley("olx", "leaving")}}
	broken := fakeSource{name: "olx", err: errors.New("blocked")}

	if _, err := Run(context.Background(), []Source{working}, s, testConfig()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	for i := range 3 {
		report, err := Run(context.Background(), []Source{broken}, s, testConfig())
		if err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
		if report.Expired != 0 {
			t.Fatalf("round %d expired %d listings: a blocked source is not proof the ad is gone", i+2, report.Expired)
		}
	}
	if got := statusOf(t, s, "leaving"); got != store.StatusActive {
		t.Errorf("status = %q, want %q", got, store.StatusActive)
	}
}

func TestNotifySkipsAGoneMatch(t *testing.T) {
	s := openStore(t)
	source := fakeSource{name: "olx", items: []model.RawListing{harley("olx", "leaving")}}
	empty := fakeSource{name: "olx", items: []model.RawListing{harley("olx", "other")}}

	if _, err := Run(context.Background(), []Source{source}, s, testConfig()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	for range 3 {
		if _, err := Run(context.Background(), []Source{empty}, s, testConfig()); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	if got := statusOf(t, s, "leaving"); got != store.StatusGone {
		t.Fatalf("status = %q, want %q before checking the alerts", got, store.StatusGone)
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: only the listing still online deserves an alert", sent)
	}
	if len(n.alerts) != 1 || n.alerts[0].URL != "https://example.com/other" {
		t.Errorf("alert = %+v, want the listing that is still online", n.alerts)
	}
}
