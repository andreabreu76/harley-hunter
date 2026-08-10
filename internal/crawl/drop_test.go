package crawl

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/fipe"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func pricedAt(source, id, price string) model.RawListing {
	raw := harley(source, id)
	raw.PriceText = price
	return raw
}

func runWith(t *testing.T, s *store.Store, items ...model.RawListing) Report {
	t.Helper()
	report, err := Run(context.Background(), []Source{fakeSource{name: "olx", items: items}}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return report
}

func drainAlerts(t *testing.T, s *store.Store) {
	t.Helper()
	if _, err := Notify(context.Background(), s, &recordingNotifier{}, 5, nil); err != nil {
		t.Fatalf("draining the pending alerts: %v", err)
	}
}

func TestNotifyAlertsThePriceDropLeadingWithTheSignal(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼ R$ 4.000: ") {
		t.Errorf("alert = %q, want it to lead with the drop", n.messages[0])
	}
	if !strings.Contains(n.messages[0], "R$ 68.000") {
		t.Errorf("alert = %q, want the price it dropped to", n.messages[0])
	}
	if n.alerts[0].URL != "https://example.com/1" {
		t.Errorf("alert url = %q, want the listing url", n.alerts[0].URL)
	}
}

func TestNotifyKeepsADropPendingWhenTheCapIsSpent(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"), pricedAt("olx", "9", "R$ 71.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 1, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: the cap allows one", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼") {
		t.Errorf("alert = %q, want the measured drop ahead of a match with no reference", n.messages[0])
	}

	rest := &recordingNotifier{}
	sent, err = Notify(context.Background(), s, rest, 5, nil)
	if err != nil {
		t.Fatalf("second Notify: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1: what did not fit must survive the round", sent)
	}
}

func TestNotifyKeepsADropPendingWhenDeliveryFails(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	if _, err := Notify(context.Background(), s, &recordingNotifier{failAt: 1}, 5, nil); err == nil {
		t.Fatal("Notify should surface the delivery error")
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify after the failure: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1: a failed delivery leaves the drop pending", sent)
	}
}

func TestNotifyCollapsesTwoDropsIntoTheAccumulatedOne(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 70.000"))
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: two drops before delivery are one piece of news", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼ R$ 4.000: ") {
		t.Errorf("alert = %q, want the accumulated drop from 72.000 to 68.000", n.messages[0])
	}
}

func TestNotifyStaysQuietWhenThePriceGoesBackUp(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0: a price rise is not news", sent)
	}

	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))
	back := &recordingNotifier{}
	sent, err = Notify(context.Background(), s, back, 5, nil)
	if err != nil {
		t.Fatalf("Notify after coming back down: %v", err)
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0: coming back to a price already announced is not news", sent)
	}
}

func TestNotifyRanksTheBiggerDiscountFirst(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"), pricedAt("olx", "2", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 70.000"), pricedAt("olx", "2", "R$ 64.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 1, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼ R$ 8.000: ") {
		t.Errorf("alert = %q, want the 8.000 drop ahead of the 2.000 one", n.messages[0])
	}
}

func TestNotifyRanksTheDeeperFipeDiscountFirst(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"), pricedAt("olx", "2", "R$ 60.000"))

	refs := fipe.NewTable([]fipe.Reference{{
		Bike:       model.BikeStreetGlide,
		Variant:    model.VariantSpecial,
		Year:       2015,
		PriceCents: 7500000,
	}})

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 1, refs)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if !strings.Contains(n.messages[0], "R$ 60.000") {
		t.Errorf("alert = %q, want the listing furthest below the fipe first", n.messages[0])
	}
}

func TestNotifyAlertsOnceWhenADropAlsoFlipsMaybeToMatch(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 80.000"))

	rows, err := s.ListByVerdict(model.VerdictMaybe)
	if err != nil || len(rows) != 1 {
		t.Fatalf("seed should be a maybe: %d rows, %v", len(rows), err)
	}

	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: the flip alerts once, not twice", sent)
	}
	if strings.HasPrefix(n.messages[0], "▼") {
		t.Errorf("alert = %q, want the plain new-match alert", n.messages[0])
	}
}

func TestNotifyAlertsACrossPostDropOnlyOnce(t *testing.T) {
	s := openStore(t)

	posted := func(source, id string, cents int64, km int) model.Listing {
		l := matchListing(id)
		l.Source = source
		l.PriceCents = &cents
		l.Km = &km
		l.Fingerprint = "aa11bb22cc33dd44"
		return l
	}

	for _, l := range []model.Listing{posted("olx", "1", 7200000, 53000), posted("mercadolivre", "2", 7200000, 53000)} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	drainAlerts(t, s)

	for _, l := range []model.Listing{posted("olx", "1", 6800000, 53000), posted("mercadolivre", "2", 6800000, 53000)} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert after the drop: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1: both sides of a cross-post dropping is one piece of news", sent)
	}
}

func TestNotifyReachesADropOnANewerListingBehindAQueueOfOlderMatches(t *testing.T) {
	s := openStore(t)

	old := time.Now().Add(-24 * time.Hour)
	for i := 0; i < 20; i++ {
		older := withPrice(matchListing(fmt.Sprintf("old-%d", i)), 7500000)
		if _, err := s.Upsert(older, old.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("Upsert of the older match: %v", err)
		}
	}

	newer := withPrice(matchListing("newer"), 7200000)
	res, err := s.Upsert(newer, time.Now())
	if err != nil {
		t.Fatalf("Upsert of the newer match: %v", err)
	}
	anchored := int64(7200000)
	if err := s.MarkNotified(res.ID, &anchored); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	if _, err := s.Upsert(withPrice(newer, 6000000), time.Now()); err != nil {
		t.Fatalf("Upsert after the drop: %v", err)
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 1, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: the cap allows one", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼") {
		t.Errorf("alert = %q, want the drop: no number of older matches may hide it from the ordering",
			n.messages[0])
	}
}

func TestNotifyKeepsTheSilencedSideOfACrossPostDropForTheNextRound(t *testing.T) {
	s := openStore(t)

	posted := func(source, id string, cents int64) model.Listing {
		l := withPrice(withKm(matchListing(id), 53000), cents)
		l.Source = source
		l.Fingerprint = "aa11bb22cc33dd44"
		return l
	}

	for _, l := range []model.Listing{posted("olx", "1", 7200000), posted("mercadolivre", "2", 7200000)} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	drainAlerts(t, s)

	for _, l := range []model.Listing{posted("olx", "1", 6800000), posted("mercadolivre", "2", 6800000)} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert after the drop: %v", err)
		}
	}

	announced := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, announced, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: both sides dropping is one piece of news in the round", sent)
	}

	next := &recordingNotifier{}
	sent, err = Notify(context.Background(), s, next, 5, nil)
	if err != nil {
		t.Fatalf("Notify on the next round: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: silencing a cross-post must not swallow its pending drop", sent)
	}
	if !strings.HasPrefix(next.messages[0], "▼ R$ 4.000: ") {
		t.Errorf("alert = %q, want the drop the silenced side was still holding", next.messages[0])
	}
}

func TestNotifyKeepsASecondSameSourceAdWhenTheSortHoistsTheCrossPost(t *testing.T) {
	s := openStore(t)

	shared := func(source, id string, cents int64, km int) model.Listing {
		l := withPrice(withKm(matchListing(id), km), cents)
		l.Source = source
		l.Fingerprint = "aa11bb22cc33dd44"
		return l
	}

	newest := time.Now()
	oldest := newest.Add(-time.Hour)

	if _, err := s.Upsert(shared("olx", "olx-237", 7500000, 53000), newest); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	drainAlerts(t, s)

	for _, l := range []model.Listing{
		shared("webmotors", "wm-237", 7200000, 53000),
		shared("webmotors", "wm-251", 7200000, 53118),
	} {
		if _, err := s.Upsert(l, oldest); err != nil {
			t.Fatalf("Upsert of the webmotors ads: %v", err)
		}
	}
	if _, err := s.Upsert(shared("olx", "olx-237", 6000000, 53000), newest.Add(time.Minute)); err != nil {
		t.Fatalf("Upsert after the drop: %v", err)
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if !strings.HasPrefix(n.messages[0], "▼") {
		t.Fatalf("first alert = %q, want the drop the urgency sort hoisted ahead of the older ads", n.messages[0])
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2: hoisting the olx cross-post must not silence both webmotors bikes", sent)
	}
}
