package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sample(cents int64) model.Listing {
	year := 2015
	km := 31000
	return model.Listing{
		Source:      "olx",
		ExternalID:  "abc123",
		URL:         "https://olx.com.br/abc123",
		Title:       "Harley Street Glide 2015",
		Bike:        model.BikeStreetGlide,
		Variant:     model.VariantBase,
		Year:        &year,
		PriceCents:  &cents,
		Km:          &km,
		City:        "curitiba",
		State:       "PR",
		Verdict:     model.VerdictMatch,
		Fingerprint: "deadbeef",
	}
}

func TestUpsertInsertsThenDeduplicates(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	first, err := s.Upsert(sample(7200000), now)
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	if !first.IsNew {
		t.Error("first insert should report IsNew")
	}

	second, err := s.Upsert(sample(7200000), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if second.IsNew {
		t.Error("same external id should not be reported as new")
	}
	if second.ID != first.ID {
		t.Errorf("ID changed on re-upsert: %d then %d", first.ID, second.ID)
	}
	if second.PriceChanged {
		t.Error("unchanged price should not report a change")
	}

	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestUpsertRecordsPriceDrop(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	if _, err := s.Upsert(sample(7500000), now); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	res, err := s.Upsert(sample(7100000), now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !res.PriceChanged {
		t.Fatal("price drop should be reported")
	}
	if res.PreviousCents == nil || *res.PreviousCents != 7500000 {
		t.Errorf("PreviousCents = %v, want 7500000", res.PreviousCents)
	}

	_, points, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 price points, got %d", len(points))
	}
}

func TestPendingNotificationsOnlyReturnsUnnotifiedMatches(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	maybeListing := sample(7200000)
	maybeListing.ExternalID = "maybe1"
	maybeListing.Verdict = model.VerdictMaybe
	if _, err := s.Upsert(maybeListing, now); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	res, err := s.Upsert(sample(7200000), now)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != res.ID {
		t.Fatalf("expected only the match row, got %d rows", len(pending))
	}

	if err := s.MarkNotified(res.ID); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	pending, err = s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected no pending rows after marking, got %d", len(pending))
	}
}

func TestSetUserState(t *testing.T) {
	s := openTemp(t)
	res, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := s.SetUserState(res.ID, "contacted"); err != nil {
		t.Fatalf("SetUserState: %v", err)
	}
	row, _, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.UserState != "contacted" {
		t.Errorf("UserState = %q, want contacted", row.UserState)
	}
}

func TestRecentRunCounts(t *testing.T) {
	s := openTemp(t)
	now := time.Now()
	for i, count := range []int{7, 9, 0} {
		start := now.Add(time.Duration(i) * time.Hour)
		if err := s.RecordRun("olx", start, start.Add(time.Minute), count, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	counts, err := s.RecentRunCounts("olx", 5)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	if len(counts) != 3 || counts[0] != 0 {
		t.Errorf("counts = %v, want most recent first starting with 0", counts)
	}
}

func TestCountByStateCountsEveryVerdict(t *testing.T) {
	s := openTemp(t)
	seed := []struct {
		id      string
		state   string
		verdict model.Verdict
	}{
		{"a", "RJ", model.VerdictMatch},
		{"b", "RJ", model.VerdictReject},
		{"c", "SP", model.VerdictMaybe},
		{"d", "", model.VerdictReject},
	}
	for _, l := range seed {
		listing := sample(7200000)
		listing.ExternalID = l.id
		listing.State = l.state
		listing.Verdict = l.verdict
		if _, err := s.Upsert(listing, time.Now()); err != nil {
			t.Fatalf("Upsert %s: %v", l.id, err)
		}
	}

	counts, err := s.CountByState()
	if err != nil {
		t.Fatalf("CountByState: %v", err)
	}
	if counts["RJ"] != 2 {
		t.Errorf("RJ = %d, want 2", counts["RJ"])
	}
	if counts["SP"] != 1 {
		t.Errorf("SP = %d, want 1", counts["SP"])
	}
	if counts[""] != 1 {
		t.Errorf("stateless = %d, want 1", counts[""])
	}
}

func TestLastRunAtReturnsTheMostRecentFinish(t *testing.T) {
	s := openTemp(t)
	if _, ok, err := s.LastRunAt("olx"); err != nil || ok {
		t.Fatalf("LastRunAt on an empty store = (%v, %v), want (false, nil)", ok, err)
	}

	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	for i := range 3 {
		start := base.Add(time.Duration(i) * time.Hour)
		if err := s.RecordRun("olx", start, start.Add(time.Minute), 10, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}

	at, ok, err := s.LastRunAt("olx")
	if err != nil {
		t.Fatalf("LastRunAt: %v", err)
	}
	if !ok {
		t.Fatal("LastRunAt should find the recorded runs")
	}
	if want := base.Add(2*time.Hour + time.Minute); !at.Equal(want) {
		t.Errorf("LastRunAt = %s, want %s", at, want)
	}
}

func TestUpsertKeepsTheSellerPhone(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	l := sample(7200000)
	phone := "11982413574"
	l.Phone = &phone
	res, err := s.Upsert(l, now)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	row, _, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.Phone == nil || *row.Phone != phone {
		t.Fatalf("Phone = %v, want %q", row.Phone, phone)
	}

	l.Phone = nil
	if _, err := s.Upsert(l, now.Add(time.Hour)); err != nil {
		t.Fatalf("Upsert without phone: %v", err)
	}
	row, _, err = s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.Phone != nil {
		t.Fatalf("Phone = %q, want nil once the ad stopped publishing it", *row.Phone)
	}
}

func TestUpsertLeavesThePhoneNilWhenTheAdHasNone(t *testing.T) {
	s := openTemp(t)

	res, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	row, _, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.Phone != nil {
		t.Fatalf("Phone = %q, want nil", *row.Phone)
	}
}
