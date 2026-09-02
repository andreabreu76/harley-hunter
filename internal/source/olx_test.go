package source

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
)

func TestParseOLXExtractsListings(t *testing.T) {
	f, err := os.Open("testdata/olx-search.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseOLX(f)
	if err != nil {
		t.Fatalf("ParseOLX: %v", err)
	}
	if len(listings) == 0 {
		t.Fatal("expected at least one listing from the fixture")
	}

	for i, l := range listings {
		if l.Source != "olx" {
			t.Errorf("listing %d: Source = %q, want olx", i, l.Source)
		}
		if l.ExternalID == "" {
			t.Errorf("listing %d: ExternalID is empty", i)
		}
		if l.URL == "" {
			t.Errorf("listing %d: URL is empty", i)
		}
		if l.Title == "" {
			t.Errorf("listing %d: Title is empty", i)
		}
	}
}

func TestParseOLXSkipsAdvertisingSlots(t *testing.T) {
	listings := parseFixture(t, "testdata/olx-search.html")
	if got, want := len(listings), 42; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
}

func TestParseOLXMapsAdFields(t *testing.T) {
	listings := parseFixture(t, "testdata/olx-search.html")

	publishedAt := time.Date(2026, 8, 4, 12, 49, 53, 0, time.UTC)
	want := model.RawListing{
		Source:       "olx",
		ExternalID:   "1499153946",
		URL:          "https://pr.olx.com.br/regiao-de-curitiba-e-paranagua/autos-e-pecas/motos/harley-davidson-street-glide-ultra-2025-1499153946",
		Title:        "HARLEY-DAVIDSON STREET GLIDE ULTRA 2025",
		RawText:      "Harley-Davidson Glide Ultra Flhxu",
		PriceText:    "R$ 185.000",
		YearText:     "2025",
		KmText:       "4780",
		LocationText: "Curitiba - PR",
		ImageURL:     "https://img.olx.com.br/images/76/769644421707054.jpg",
		PublishedAt:  &publishedAt,
	}

	for _, got := range listings {
		if got.ExternalID != want.ExternalID {
			continue
		}
		if got.PublishedAt == nil || !got.PublishedAt.Equal(*want.PublishedAt) {
			t.Fatalf("PublishedAt = %v, want %v", got.PublishedAt, want.PublishedAt)
		}
		got.PublishedAt = want.PublishedAt
		if got != want {
			t.Fatalf("listing =\n%+v\nwant\n%+v", got, want)
		}
		return
	}
	t.Fatalf("listing %s not found in the fixture", want.ExternalID)
}

func TestParseOLXKeepsTheStructuredModelReachable(t *testing.T) {
	listings := parseFixture(t, "testdata/olx-search.html")

	for _, l := range listings {
		if l.ExternalID != "1524873809" {
			continue
		}
		if l.Title != "Stret glide excelente estado" {
			t.Fatalf("fixture changed: Title = %q", l.Title)
		}
		if l.RawText != "Harley-Davidson Glide Flhx" {
			t.Fatalf("RawText = %q, want the vehicle_model property", l.RawText)
		}
		bike, _ := normalize.DetectBike(l.Title + " " + l.RawText)
		if bike != model.BikeOther {
			t.Fatalf("DetectBike = %q, want %q: the structured model reaches the classifier, which rejects a Touring", bike, model.BikeOther)
		}
		return
	}
	t.Fatal("listing 1524873809 not found in the fixture")
}

func TestParseOLXReturnsErrorWhenBlocked(t *testing.T) {
	blocked := stringReader("<html><body>Acesso negado</body></html>")
	if _, err := ParseOLX(blocked); err == nil {
		t.Fatal("ParseOLX should return an error when the payload is missing")
	}
}

func TestParseOLXReturnsErrorWhenPayloadHasNoAds(t *testing.T) {
	page := olxPage(t, `2:["$","html",null,{"lang":"pt-BR"}]`)
	if _, err := ParseOLX(page); err == nil {
		t.Fatal("ParseOLX should return an error when the payload carries no ads array")
	}
}

func TestParseOLXReturnsErrorWhenNoAdYieldsAListing(t *testing.T) {
	page := olxPage(t, `1b:{"ads":[{"subject":"Street Glide"},{"subject":"Road Glide"}]}`)
	if _, err := ParseOLX(page); err == nil {
		t.Fatal("ParseOLX should return an error when the ads array is populated but no ad can be read")
	}
}

func TestParseOLXFallsBackToTheLocationLabel(t *testing.T) {
	page := olxPage(t, `1b:{"ads":[{"listId":1,"url":"https://pr.olx.com.br/d/1","subject":"Street Glide","locationDetails":null,"location":"Curitiba, Batel"}]}`)

	listings, err := ParseOLX(page)
	if err != nil {
		t.Fatalf("ParseOLX: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1", len(listings))
	}
	if got, want := listings[0].LocationText, "Curitiba, Batel"; got != want {
		t.Errorf("LocationText = %q, want %q", got, want)
	}
}

func TestParseOLXIgnoresTheVipSelectionWhenTheSearchIsEmptySynthetic(t *testing.T) {
	page := olxPage(t, vipFirstFlight(`"ads":[],"searchBoxProps":{"keyword":"harley street glide"}`))

	listings, err := ParseOLX(page)
	if err != nil {
		t.Fatalf("ParseOLX: %v", err)
	}
	if len(listings) != 0 {
		t.Fatalf("len(listings) = %d, want 0: the vip selection is not the result set", len(listings))
	}
}

func TestParseOLXIgnoresTheVipSelectionWhenTheSearchHasResultsSynthetic(t *testing.T) {
	results := `"ads":[{"listId":7,"url":"https://olx.test/d/7","subject":"Street Glide"}],"searchBoxProps":{"keyword":"harley street glide"}`
	page := olxPage(t, vipFirstFlight(results))

	listings, err := ParseOLX(page)
	if err != nil {
		t.Fatalf("ParseOLX: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1", len(listings))
	}
	if got, want := listings[0].ExternalID, "7"; got != want {
		t.Errorf("ExternalID = %q, want %q: took the vip ad instead of the result", got, want)
	}
}

func vipFirstFlight(results string) string {
	seller := `{"name":"Loja","description":"` + strings.Repeat("x", 4000) + `"}`
	vip := `"topoVipSelection":{"seller":` + seller + `,"ads":[{"listId":99,"url":"https://olx.test/d/99","subject":"VIP"}]}`
	return `1b:{` + vip + `,` + results + `}`
}

func TestParseOLXReturnsNoListingsWhenSearchIsEmpty(t *testing.T) {
	f, err := os.Open("testdata/olx-search-empty.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseOLX(f)
	if err != nil {
		t.Fatalf("a search with no results is not an error: %v", err)
	}
	if len(listings) != 0 {
		t.Fatalf("len(listings) = %d, want 0", len(listings))
	}
}

func TestOLXIsASource(t *testing.T) {
	var s Source = NewOLX(nil, nil)
	if s.Name() != "olx" {
		t.Errorf("Name() = %q, want olx", s.Name())
	}
}

func TestOLXFetchCollectsEveryURL(t *testing.T) {
	fixture, err := os.ReadFile("testdata/olx-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture)}
	o := NewOLX(fetcher, []string{"https://olx.test/street", "https://olx.test/road"})
	o.delay = 0

	listings, err := o.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := len(listings), 84; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	want := []string{"https://olx.test/street", "https://olx.test/road"}
	if !slices.Equal(fetcher.asked, want) {
		t.Errorf("asked for %q, want %q", fetcher.asked, want)
	}
}

func TestOLXFetchFailsWhenTheBrowserFails(t *testing.T) {
	fetcher := &fakePageFetcher{err: errors.New("chrome is not listening")}
	o := NewOLX(fetcher, []string{"https://olx.test/street"})
	o.delay = 0

	if _, err := o.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch should fail when the page cannot be fetched")
	}
}

func TestOLXFetchFailsWhenThePageCarriesNoPayload(t *testing.T) {
	fetcher := &fakePageFetcher{page: "<html><body>Acesso negado</body></html>"}
	o := NewOLX(fetcher, []string{"https://olx.test/street"})
	o.delay = 0

	if _, err := o.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch should fail when the browser lands on a block page")
	}
}

func TestOLXFetchStopsOnACancelledContext(t *testing.T) {
	fixture, err := os.ReadFile("testdata/olx-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture)}
	o := NewOLX(fetcher, []string{"https://olx.test/street", "https://olx.test/road"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := o.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch error = %v, want context.Canceled", err)
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked for %q, want no request at all on a cancelled context", fetcher.asked)
	}
}

func TestOLXFetchKeepsWhatItGatheredWhenAURLFails(t *testing.T) {
	fixture, err := os.ReadFile("testdata/olx-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture), failOn: "https://olx.test/road"}
	o := NewOLX(fetcher, []string{"https://olx.test/street", "https://olx.test/road", "https://olx.test/electra"})
	o.delay = 0

	listings, err := o.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should report the failure")
	}
	if got, want := len(listings), 42; got != want {
		t.Fatalf("len(listings) = %d, want %d: the first url's listings are worth keeping", got, want)
	}
}

func TestNewBrowserFetcherDefaultsToTheLocalDevtoolsPort(t *testing.T) {
	var _ PageFetcher = NewBrowserFetcher("")

	if got := NewBrowserFetcher("").devtoolsURL; got != defaultDevtoolsURL {
		t.Errorf("devtoolsURL = %q, want %q", got, defaultDevtoolsURL)
	}
	if got := NewBrowserFetcher("http://127.0.0.1:9333").devtoolsURL; got != "http://127.0.0.1:9333" {
		t.Errorf("devtoolsURL = %q, want the configured one", got)
	}
}

func olxPage(t *testing.T, flight string) io.Reader {
	t.Helper()
	chunk, err := json.Marshal([]any{1, flight})
	if err != nil {
		t.Fatalf("encoding flight chunk: %v", err)
	}
	return stringReader("<html><body><script>self.__next_f.push(" + string(chunk) + ")</script></body></html>")
}

func parseFixture(t *testing.T, path string) []model.RawListing {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseOLX(f)
	if err != nil {
		t.Fatalf("ParseOLX: %v", err)
	}
	return listings
}

func TestParseOLXReadsTheDateTheAdWasPublished(t *testing.T) {
	listings := parseFixture(t, "testdata/olx-search.html")

	want := time.Date(2026, 8, 4, 12, 49, 53, 0, time.UTC)
	for _, l := range listings {
		if l.ExternalID != "1499153946" {
			continue
		}
		if l.PublishedAt == nil {
			t.Fatal("PublishedAt is nil, want the date the flight payload carries")
		}
		if !l.PublishedAt.Equal(want) {
			t.Fatalf("PublishedAt = %s, want %s", l.PublishedAt.Format(time.RFC3339), want.Format(time.RFC3339))
		}
		if l.PublishedAt.Location() != time.UTC {
			t.Errorf("PublishedAt zone = %s, want UTC", l.PublishedAt.Location())
		}
		return
	}
	t.Fatal("listing 1499153946 not found in the fixture")
}

func TestParseOLXLeavesThePublishedDateNilWhenTheAdOmitsIt(t *testing.T) {
	page := olxPage(t, `{"ads":[{"listId":1,"subject":"Street Glide","url":"https://olx.com.br/1"}]}`)

	listings, err := ParseOLX(page)
	if err != nil {
		t.Fatalf("ParseOLX: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1", len(listings))
	}
	if listings[0].PublishedAt != nil {
		t.Errorf("PublishedAt = %v, want nil for an ad with no date", listings[0].PublishedAt)
	}
}
