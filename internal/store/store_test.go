package store

import (
	"fmt"
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
		t.Errorf("PreviousCents = %s, want 7500000", describe(res.PreviousCents))
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

	if err := s.MarkNotified(res.ID, nil); err != nil {
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
		t.Fatalf("Phone = %s, want %q", describe(row.Phone), phone)
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

func TestUpsertKeepsThePublishedDateInUTC(t *testing.T) {
	s := openTemp(t)
	brt := time.FixedZone("BRT", -3*3600)
	published := time.Date(2026, 8, 4, 9, 49, 53, 0, brt)

	l := sample(7200000)
	l.PublishedAt = &published
	res, err := s.Upsert(l, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	row, _, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.PublishedAt == nil {
		t.Fatal("PublishedAt is nil, want the date that was written")
	}
	if !row.PublishedAt.Equal(published) {
		t.Errorf("PublishedAt = %s, want %s", row.PublishedAt, published)
	}
	if row.PublishedAt.Location() != time.UTC {
		t.Errorf("PublishedAt zone = %s, want UTC", row.PublishedAt.Location())
	}
}

func TestUpsertDoesNotForgetAPublishedDateTheSourceStopsSending(t *testing.T) {
	s := openTemp(t)
	published := time.Date(2026, 8, 4, 12, 49, 53, 0, time.UTC)

	l := sample(7200000)
	l.PublishedAt = &published
	res, err := s.Upsert(l, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	l.PublishedAt = nil
	if _, err := s.Upsert(l, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Upsert without a date: %v", err)
	}

	row, _, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.PublishedAt == nil || !row.PublishedAt.Equal(published) {
		t.Fatalf("PublishedAt = %v, want %s to survive a round without it", row.PublishedAt, published)
	}
}

func TestUpsertLeavesThePublishedDateNilWhenNoSourceEverSentOne(t *testing.T) {
	s := openTemp(t)

	res, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	row, _, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.PublishedAt != nil {
		t.Fatalf("PublishedAt = %s, want nil", row.PublishedAt)
	}
}

func TestMarkNotifiedAnchorsAtTheLowestPriceAnnounced(t *testing.T) {
	s := openTemp(t)

	res, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	anchor := func(cents int64) *int64 { return &cents }

	if err := s.MarkNotified(res.ID, anchor(7200000)); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 7200000 {
		t.Fatalf("anchor = %s, want 7200000", describe(got))
	}

	if err := s.MarkNotified(res.ID, anchor(7500000)); err != nil {
		t.Fatalf("MarkNotified on a higher price: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 7200000 {
		t.Errorf("anchor = %s, want it to stay at 7200000: a price rise is not news", describe(got))
	}

	if err := s.MarkNotified(res.ID, anchor(6800000)); err != nil {
		t.Fatalf("MarkNotified on a lower price: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 6800000 {
		t.Errorf("anchor = %s, want 6800000: a drop moves the anchor down", describe(got))
	}

	if err := s.MarkNotified(res.ID, nil); err != nil {
		t.Fatalf("MarkNotified without a price: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 6800000 {
		t.Errorf("anchor = %s, want 6800000 kept: a priceless round must not erase it", describe(got))
	}
}

func TestMarkSilencedLeavesAPendingDropStillPending(t *testing.T) {
	s := openTemp(t)

	l := sample(7200000)
	res, err := s.Upsert(l, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	anchored := int64(7200000)
	if err := s.MarkNotified(res.ID, &anchored); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	lower := int64(6800000)
	l.PriceCents = &lower
	if _, err := s.Upsert(l, time.Now()); err != nil {
		t.Fatalf("Upsert after the drop: %v", err)
	}

	if err := s.MarkSilenced(res.ID, &lower); err != nil {
		t.Fatalf("MarkSilenced: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 7200000 {
		t.Fatalf("anchor = %s, want 7200000 kept: silencing must not announce a price for the owner",
			describe(got))
	}

	pending, err := s.PendingAlerts()
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("pending = %d rows, want the drop still waiting for its round", len(pending))
	}
}

func TestMarkSilencedAnchorsAListingThatNeverHadOne(t *testing.T) {
	s := openTemp(t)

	res, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	price := int64(7200000)
	if err := s.MarkSilenced(res.ID, &price); err != nil {
		t.Fatalf("MarkSilenced: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 7200000 {
		t.Fatalf("anchor = %s, want 7200000: a silenced twin still needs its anchor",
			describe(got))
	}

	pending, err := s.PendingAlerts()
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %d rows, want 0: a silenced listing leaves the queue", len(pending))
	}
}

func describe[T any](value *T) string {
	if value == nil {
		return "nil"
	}
	return fmt.Sprintf("%v", *value)
}

func notifiedPriceOf(t *testing.T, s *Store, id int64) *int64 {
	t.Helper()
	row, _, err := s.GetRow(id)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	return row.NotifiedPriceCents
}

func TestPendingAlertsCarriesNewMatchesAndPriceDrops(t *testing.T) {
	s := openTemp(t)

	fresh, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	dropping := sample(7200000)
	dropping.ExternalID = "dropping"
	res, err := s.Upsert(dropping, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	anchored := int64(7200000)
	if err := s.MarkNotified(res.ID, &anchored); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	steady := sample(7000000)
	steady.ExternalID = "steady"
	quiet, err := s.Upsert(steady, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	held := int64(7000000)
	if err := s.MarkNotified(quiet.ID, &held); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	pending, err := s.PendingAlerts()
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != fresh.ID {
		t.Fatalf("pending = %d rows, want just the new match", len(pending))
	}

	dropped := dropping
	lower := int64(6800000)
	dropped.PriceCents = &lower
	if _, err := s.Upsert(dropped, time.Now()); err != nil {
		t.Fatalf("Upsert after the drop: %v", err)
	}

	pending, err = s.PendingAlerts()
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d rows, want the new match and the drop", len(pending))
	}
	var sawDrop bool
	for _, row := range pending {
		if row.ID == res.ID {
			sawDrop = true
		}
		if row.ID == quiet.ID {
			t.Error("a listing whose price did not move must stay out of the queue")
		}
	}
	if !sawDrop {
		t.Error("the drop is missing from the queue")
	}
}

func TestPendingAlertsIgnoresADropOnAListingAlreadyGone(t *testing.T) {
	s := openTemp(t)

	l := sample(7200000)
	res, err := s.Upsert(l, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	anchored := int64(7200000)
	if err := s.MarkNotified(res.ID, &anchored); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	lower := int64(6800000)
	l.PriceCents = &lower
	if _, err := s.Upsert(l, time.Now()); err != nil {
		t.Fatalf("Upsert after the drop: %v", err)
	}
	if _, err := s.db.Exec("UPDATE listings SET status = ? WHERE id = ?", StatusGone, res.ID); err != nil {
		t.Fatalf("closing the listing: %v", err)
	}

	pending, err := s.PendingAlerts()
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %d rows, want 0: a closed ad is not an opportunity", len(pending))
	}
}

func TestPendingAlertsKeepsAMaybeOutOfTheQueueWhenItsPriceDrops(t *testing.T) {
	s := openTemp(t)

	l := sample(8000000)
	l.Verdict = model.VerdictMaybe
	res, err := s.Upsert(l, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	pending, err := s.PendingAlerts()
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d rows, want 0: a maybe is not a new match", len(pending))
	}

	anchored := int64(8000000)
	if err := s.MarkNotified(res.ID, &anchored); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	lower := int64(7600000)
	l.PriceCents = &lower
	if _, err := s.Upsert(l, time.Now()); err != nil {
		t.Fatalf("Upsert after the drop: %v", err)
	}

	pending, err = s.PendingAlerts()
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %d rows, want 0: only matches ring the phone", len(pending))
	}
}

func TestCountByVerdictCountsTheWholeTable(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	for _, spec := range []struct {
		id      string
		verdict model.Verdict
	}{
		{"c1", model.VerdictMatch},
		{"c2", model.VerdictMaybe},
		{"c3", model.VerdictMaybe},
		{"c4", model.VerdictReject},
	} {
		listing := sample(7200000)
		listing.ExternalID = spec.id
		listing.Verdict = spec.verdict
		if _, err := s.Upsert(listing, now); err != nil {
			t.Fatalf("Upsert %s: %v", spec.id, err)
		}
	}

	counts, err := s.CountByVerdict()
	if err != nil {
		t.Fatalf("CountByVerdict: %v", err)
	}
	for verdict, want := range map[string]int{"match": 1, "maybe": 2, "reject": 1} {
		if counts[verdict] != want {
			t.Errorf("%s: expected %d, got %d", verdict, want, counts[verdict])
		}
	}
	if len(counts) != 3 {
		t.Errorf("expected 3 verdicts, got %d: %v", len(counts), counts)
	}
}

func TestPriceHistoryForGroupsByListingInObservationOrder(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	tracked := sample(7500000)
	tracked.ExternalID = "h1"
	first, err := s.Upsert(tracked, now)
	if err != nil {
		t.Fatalf("Upsert h1: %v", err)
	}
	dropped := int64(7100000)
	tracked.PriceCents = &dropped
	if _, err := s.Upsert(tracked, now.Add(48*time.Hour)); err != nil {
		t.Fatalf("Upsert h1 again: %v", err)
	}

	other := sample(6900000)
	other.ExternalID = "h2"
	second, err := s.Upsert(other, now)
	if err != nil {
		t.Fatalf("Upsert h2: %v", err)
	}

	history, err := s.PriceHistoryFor([]int64{first.ID, second.ID})
	if err != nil {
		t.Fatalf("PriceHistoryFor: %v", err)
	}

	points := history[first.ID]
	if len(points) != 2 {
		t.Fatalf("expected 2 points for the tracked listing, got %d", len(points))
	}
	if points[0].PriceCents != 7500000 || points[1].PriceCents != 7100000 {
		t.Errorf("expected 7500000 then 7100000, got %d then %d",
			points[0].PriceCents, points[1].PriceCents)
	}
	if len(history[second.ID]) != 1 {
		t.Errorf("expected 1 point for the other listing, got %d", len(history[second.ID]))
	}
}

func TestPriceHistoryForIgnoresListingsNotAsked(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	wanted := sample(7500000)
	wanted.ExternalID = "h1"
	asked, err := s.Upsert(wanted, now)
	if err != nil {
		t.Fatalf("Upsert h1: %v", err)
	}
	skipped := sample(6900000)
	skipped.ExternalID = "h2"
	if _, err := s.Upsert(skipped, now); err != nil {
		t.Fatalf("Upsert h2: %v", err)
	}

	history, err := s.PriceHistoryFor([]int64{asked.ID})
	if err != nil {
		t.Fatalf("PriceHistoryFor: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected history for 1 listing, got %d: %v", len(history), history)
	}
}

func TestPriceHistoryForWithoutIDsReturnsEmptyMap(t *testing.T) {
	s := openTemp(t)

	history, err := s.PriceHistoryFor(nil)
	if err != nil {
		t.Fatalf("PriceHistoryFor: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected an empty map, got %v", history)
	}
}

func TestLastRunStartedAtIsEmptyBeforeTheFirstRound(t *testing.T) {
	s := openTemp(t)
	at, ever, err := s.LastRunStartedAt()
	if err != nil {
		t.Fatalf("LastRunStartedAt: %v", err)
	}
	if ever {
		t.Errorf("a fresh database reported a previous round at %s", at)
	}
}

func TestLastRunStartedAtTakesTheLatestAcrossSourcesAndStatuses(t *testing.T) {
	s := openTemp(t)
	old := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 8, 31, 7, 30, 0, 0, time.UTC)

	if err := s.RecordRun("olx", old, old.Add(time.Minute), 12, "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRun("instagram", recent, recent.Add(time.Minute), 0, "error", "blocked"); err != nil {
		t.Fatal(err)
	}

	at, ever, err := s.LastRunStartedAt()
	if err != nil {
		t.Fatalf("LastRunStartedAt: %v", err)
	}
	if !ever {
		t.Fatal("LastRunStartedAt found no round after two were recorded")
	}
	if !at.Equal(recent) {
		t.Errorf("LastRunStartedAt = %s, want the failed round at %s", at, recent)
	}
}
