package crawl

import (
	"context"
	"strings"
	"testing"

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

func TestRunReportsAPriceDropOnAMatch(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	report := runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	if len(report.Drops) != 1 {
		t.Fatalf("got %d drops, want 1", len(report.Drops))
	}
	drop := report.Drops[0]
	if drop.PreviousCents != 7200000 || drop.CurrentCents != 6800000 {
		t.Errorf("drop = %+v, want 7200000 to 6800000", drop)
	}
	if drop.ListingID == 0 {
		t.Error("drop carries no listing id")
	}
}

func TestRunReportsNoDropOnTheFirstSighting(t *testing.T) {
	s := openStore(t)
	report := runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	if len(report.Drops) != 0 {
		t.Errorf("got %d drops on a first sighting, want none", len(report.Drops))
	}
}

func TestRunIgnoresAPriceIncrease(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))
	report := runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))

	if len(report.Drops) != 0 {
		t.Errorf("got %d drops on a price increase, want none", len(report.Drops))
	}
}

func TestRunIgnoresADropOnAListingThatIsNotAMatch(t *testing.T) {
	s := openStore(t)
	tooDear := func(price string) model.RawListing {
		raw := pricedAt("olx", "1", price)
		raw.YearText = "2016"
		return raw
	}
	runWith(t, s, tooDear("R$ 84.000"))
	report := runWith(t, s, tooDear("R$ 80.000"))

	if len(report.Drops) != 0 {
		t.Errorf("got %d drops on a maybe listing, want none: only matches ring the phone", len(report.Drops))
	}
}

func TestNotifyAlertsThePriceDropLeadingWithTheSignal(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	report := runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	if _, err := Notify(context.Background(), s, &recordingNotifier{}, 5, nil); err != nil {
		t.Fatalf("draining the pending alert: %v", err)
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, report.Drops)
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

func TestNotifyDropsShareThePerRunCapWithNewMatches(t *testing.T) {
	s := openStore(t)
	first := []model.RawListing{pricedAt("olx", "1", "R$ 72.000")}
	for i := 2; i <= 5; i++ {
		first = append(first, pricedAt("olx", string(rune('0'+i)), "R$ 70.000"))
	}
	runWith(t, s, first...)

	if _, err := Notify(context.Background(), s, &recordingNotifier{}, 5, nil); err != nil {
		t.Fatalf("draining the pending alerts: %v", err)
	}

	second := []model.RawListing{pricedAt("olx", "1", "R$ 68.000"), pricedAt("olx", "9", "R$ 71.000")}
	report := runWith(t, s, second...)
	if len(report.Drops) != 1 {
		t.Fatalf("got %d drops, want 1", len(report.Drops))
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 1, report.Drops)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1: the drop must not spend a slot beyond the cap", sent)
	}
	if strings.HasPrefix(n.messages[0], "▼") {
		t.Errorf("alert = %q, want the new match first and the drop only if the cap allows", n.messages[0])
	}
}

func TestNotifyAlertsOnceWhenADropAlsoFlipsMaybeToMatch(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 80.000"))

	rows, err := s.ListByVerdict(model.VerdictMaybe)
	if err != nil || len(rows) != 1 {
		t.Fatalf("seed should be a maybe: %d rows, %v", len(rows), err)
	}

	report := runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))
	if len(report.Drops) != 1 {
		t.Fatalf("got %d drops, want 1", len(report.Drops))
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, report.Drops)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: the flip already alerts, the drop must not double it", sent)
	}
	if strings.HasPrefix(n.messages[0], "▼") {
		t.Errorf("alert = %q, want the plain new-match alert the pending pass sends", n.messages[0])
	}
}

func TestNotifySurfacesAFailedDropDelivery(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	report := runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	if _, err := Notify(context.Background(), s, &recordingNotifier{}, 5, nil); err != nil {
		t.Fatalf("draining the pending alert: %v", err)
	}

	n := &recordingNotifier{failAt: 1}
	sent, err := Notify(context.Background(), s, n, 5, report.Drops)
	if err == nil {
		t.Fatal("Notify should surface the delivery error")
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0", sent)
	}
}

func TestNotifyIgnoresADropWhoseListingIsGone(t *testing.T) {
	s := openStore(t)
	n := &recordingNotifier{}

	sent, err := Notify(context.Background(), s, n, 5, []PriceDrop{{ListingID: 4242, PreviousCents: 7200000, CurrentCents: 6800000}})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0 for a listing that no longer exists", sent)
	}
}
