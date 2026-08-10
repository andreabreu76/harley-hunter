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

	stored, err := Refresh(context.Background(), fetcher, db, august())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if stored != len(db.saved) {
		t.Errorf("Refresh reported %d stored, saved %d", stored, len(db.saved))
	}
	if stored != 14 {
		t.Errorf("stored = %d, want 14: fipe publishes 6 street glide quotes plus 4 years shared by electra glide and ultra", stored)
	}

	byCombo := make(map[string]Reference, len(db.saved))
	for _, r := range db.saved {
		byCombo[r.Bike+"|"+r.Variant+"|"+strconv.Itoa(r.Year)] = r
	}
	anchor, ok := byCombo["street_glide|base|2014"]
	if !ok {
		t.Fatalf("the owner anchor was not stored, got %v", db.saved)
	}
	if anchor.Code != "810059-4" || anchor.Label != "FLHX" {
		t.Errorf("anchor = %+v, want the FLHX 810059-4 row", anchor)
	}
	if anchor.PriceCents == 0 || anchor.Month == "" {
		t.Errorf("anchor = %+v, want a price and a reference month", anchor)
	}
	for _, combo := range []string{"electra_glide|unknown|2014", "ultra|unknown|2014"} {
		row, ok := byCombo[combo]
		if !ok {
			t.Errorf("%s was not stored: both families need their own row to be lookupable", combo)
			continue
		}
		if row.Code != "810060-8" {
			t.Errorf("%s code = %q, want the shared 810060-8", combo, row.Code)
		}
	}
	if _, ok := byCombo["road_glide|base|2015"]; ok {
		t.Error("a road glide row was stored, but fipe publishes no road glide for 2013-2016")
	}
}

func TestRefreshSkipsCombosAlreadyFetchedThisMonth(t *testing.T) {
	fetcher := &fakeFetcher{}
	db := &memoryStore{fetched: map[string]time.Time{
		"street_glide|base|2014": time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC),
	}}

	if _, err := Refresh(context.Background(), fetcher, db, august()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	for _, url := range fetcher.asked {
		if strings.Contains(url, "/5760/years/2014-1") {
			t.Errorf("Refresh asked again for a combo already fetched this month: %s", url)
		}
	}
}

func TestRefreshAsksAgainOnceTheMonthTurns(t *testing.T) {
	fetcher := &fakeFetcher{}
	db := &memoryStore{fetched: map[string]time.Time{
		"street_glide|base|2014": time.Date(2026, 7, 31, 23, 0, 0, 0, time.UTC),
	}}

	if _, err := Refresh(context.Background(), fetcher, db, august()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	asked := false
	for _, url := range fetcher.asked {
		if strings.Contains(url, "/5760/years/2014-1") {
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

	stored, err := Refresh(context.Background(), fetcher, db, august())
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
	if _, err := Refresh(context.Background(), &fakeFetcher{}, &memoryStore{loadErr: errors.New("db locked")}, august()); err != nil {
		t.Fatalf("Refresh must never fail the round: %v", err)
	}
}

func TestRefreshKeepsGoingWhenOneSaveFails(t *testing.T) {
	stored, err := Refresh(context.Background(), &fakeFetcher{}, &memoryStore{saveErr: errors.New("db locked")}, august())
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
	if _, err := Refresh(ctx, fetcher, &memoryStore{}, august()); err != nil {
		t.Fatalf("Refresh must never fail the round: %v", err)
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked %d urls after cancellation, want none", len(fetcher.asked))
	}
}

func TestRefreshCoversTheYearsTheOwnerWatches(t *testing.T) {
	fetcher := &fakeFetcher{}
	if _, err := Refresh(context.Background(), fetcher, &memoryStore{}, august()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got, want := len(fetcher.asked), len(models)*len(watchedYears); got != want {
		t.Errorf("asked %d urls, want %d (every mapped combo across %v)", got, want, watchedYears)
	}
	for _, year := range watchedYears {
		if year < 2013 || year > 2016 {
			t.Errorf("watched year %d is outside the window the owner asked for", year)
		}
	}
}
