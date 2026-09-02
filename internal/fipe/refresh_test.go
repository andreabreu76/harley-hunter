package fipe

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeFetcher struct {
	asked []string
	err   error
}

func (f *fakeFetcher) FetchPage(ctx context.Context, url string) (string, error) {
	f.asked = append(f.asked, url)
	if f.err != nil {
		return "", f.err
	}
	code, year := "", ""
	parts := strings.Split(strings.TrimSuffix(url, "-1"), "/")
	if len(parts) >= 3 {
		code, year = parts[len(parts)-3], parts[len(parts)-1]
	}
	body, err := os.ReadFile("testdata/value-" + code + "-" + year + ".json")
	if err != nil {
		return string(mustRead("testdata/value-notfound.json")), nil
	}
	return string(body), nil
}

func mustRead(path string) []byte {
	body, _ := os.ReadFile(path)
	return body
}

type memoryStore struct {
	saved   []Reference
	fetched map[string]time.Time
	loadErr error
	saveErr error
}

func (m *memoryStore) FipeFetchedAt() (map[string]time.Time, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.fetched == nil {
		return map[string]time.Time{}, nil
	}
	return m.fetched, nil
}

func (m *memoryStore) SaveFipeReference(r Reference, at time.Time) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, r)
	return nil
}

func august() time.Time { return time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC) }

func TestRefreshStoresEveryQuoteFipePublishes(t *testing.T) {
	fetcher := &fakeFetcher{}
	db := &memoryStore{}

	stored, err := Refresh(context.Background(), fetcher, db, august(), []int{2016, 2017, 2018, 2019, 2020})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if stored != len(db.saved) {
		t.Errorf("Refresh reported %d stored, saved %d", stored, len(db.saved))
	}
	if stored != 10 {
		t.Errorf("stored = %d, want 10: every quote fipe publishes across the asked years", stored)
	}

	byCombo := make(map[string]Reference, len(db.saved))
	for _, r := range db.saved {
		byCombo[r.Bike+"|"+r.Variant+"|"+strconv.Itoa(r.Year)] = r
	}
	anchor, ok := byCombo["sportster_1200|forty_eight|2016"]
	if !ok {
		t.Fatalf("the owner anchor was not stored, got %v", db.saved)
	}
	if anchor.Code != "810066-7" || anchor.Label != "XL 1200X" {
		t.Errorf("anchor = %+v, want the XL 1200X 810066-7 row", anchor)
	}
	if anchor.PriceCents != 4698800 || anchor.Month == "" {
		t.Errorf("anchor = %+v, want the published price and a reference month", anchor)
	}
	for combo, code := range map[string]string{
		"sportster_1200|iron|2019":     "810099-3",
		"sportster_1200|roadster|2017": "810075-6",
		"sportster_1200|custom|2016":   "810067-5",
	} {
		row, ok := byCombo[combo]
		if !ok {
			t.Errorf("%s was not stored: every trim needs its own row to be lookupable", combo)
			continue
		}
		if row.Code != code {
			t.Errorf("%s code = %q, want %q", combo, row.Code, code)
		}
	}
	if _, ok := byCombo["sportster_1200|iron|2016"]; ok {
		t.Error("an Iron 1200 row was stored for 2016, but the trim only reached the table in 2019")
	}
}

func TestRefreshSkipsCombosAlreadyFetchedThisMonth(t *testing.T) {
	fetcher := &fakeFetcher{}
	db := &memoryStore{fetched: map[string]time.Time{
		"sportster_1200|forty_eight|2016": time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC),
	}}

	if _, err := Refresh(context.Background(), fetcher, db, august(), []int{2016, 2017, 2018, 2019, 2020}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	for _, url := range fetcher.asked {
		if strings.Contains(url, "/6719/years/2016-1") {
			t.Errorf("Refresh asked again for a combo already fetched this month: %s", url)
		}
	}
}

func TestRefreshAsksAgainOnceTheMonthTurns(t *testing.T) {
	fetcher := &fakeFetcher{}
	db := &memoryStore{fetched: map[string]time.Time{
		"sportster_1200|forty_eight|2016": time.Date(2026, 7, 31, 23, 0, 0, 0, time.UTC),
	}}

	if _, err := Refresh(context.Background(), fetcher, db, august(), []int{2016, 2017, 2018, 2019, 2020}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	asked := false
	for _, url := range fetcher.asked {
		if strings.Contains(url, "/6719/years/2016-1") {
			asked = true
		}
	}
	if !asked {
		t.Error("Refresh did not ask again after the reference month turned")
	}
}

func TestRefreshSurvivesAFipeOutage(t *testing.T) {
	fetcher := &fakeFetcher{err: errors.New("fipe is down")}
	db := &memoryStore{}

	stored, err := Refresh(context.Background(), fetcher, db, august(), []int{2016, 2017, 2018, 2019, 2020})
	if err != nil {
		t.Fatalf("Refresh must never fail the round: %v", err)
	}
	if stored != 0 {
		t.Errorf("stored = %d, want 0", stored)
	}
	if len(db.saved) != 0 {
		t.Errorf("saved %d references during an outage, want none", len(db.saved))
	}
}

func TestRefreshSurvivesAnUnreadableStore(t *testing.T) {
	if _, err := Refresh(context.Background(), &fakeFetcher{}, &memoryStore{loadErr: errors.New("db locked")}, august(), []int{2016, 2017, 2018, 2019, 2020}); err != nil {
		t.Fatalf("Refresh must never fail the round: %v", err)
	}
}

func TestRefreshKeepsGoingWhenOneSaveFails(t *testing.T) {
	stored, err := Refresh(context.Background(), &fakeFetcher{}, &memoryStore{saveErr: errors.New("db locked")}, august(), []int{2016, 2017, 2018, 2019, 2020})
	if err != nil {
		t.Fatalf("Refresh must never fail the round: %v", err)
	}
	if stored != 0 {
		t.Errorf("stored = %d, want 0 when every save fails", stored)
	}
}

func TestRefreshStopsWhenTheRoundIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fetcher := &fakeFetcher{}
	if _, err := Refresh(ctx, fetcher, &memoryStore{}, august(), []int{2016, 2017, 2018, 2019, 2020}); err != nil {
		t.Fatalf("Refresh must never fail the round: %v", err)
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked %d urls after cancellation, want none", len(fetcher.asked))
	}
}

func TestRefreshAsksOnlyForTheYearsTheConfigWatches(t *testing.T) {
	fetcher := &fakeFetcher{}
	years := []int{2016}
	if _, err := Refresh(context.Background(), fetcher, &memoryStore{}, august(), years); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got, want := len(fetcher.asked), len(models)*len(years); got != want {
		t.Errorf("asked %d urls, want %d (every mapped combo across %v)", got, want, years)
	}
	for _, url := range fetcher.asked {
		if !strings.Contains(url, "/2016-1") {
			t.Errorf("asked %s, want only the target year: a quote for another year is never read", url)
		}
	}
}

func TestRefreshAsksNothingWithoutAYear(t *testing.T) {
	fetcher := &fakeFetcher{}
	if _, err := Refresh(context.Background(), fetcher, &memoryStore{}, august(), nil); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked %d urls without a target year, want none", len(fetcher.asked))
	}
}
