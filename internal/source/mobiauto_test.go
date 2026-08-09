package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestParseMobiautoExtractsListings(t *testing.T) {
	listings := parseMobiautoFixture(t, "testdata/mobiauto-search.html")

	if got, want := len(listings), 5; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	for i, l := range listings {
		if l.Source != model.SourceMobiauto {
			t.Errorf("listing %d: Source = %q, want %q", i, l.Source, model.SourceMobiauto)
		}
		if l.ExternalID == "" {
			t.Errorf("listing %d: ExternalID is empty", i)
		}
		if !strings.HasPrefix(l.URL, "https://www.mobiauto.com.br/") {
			t.Errorf("listing %d: URL = %q, want an absolute mobiauto url", i, l.URL)
		}
		if l.Title == "" {
			t.Errorf("listing %d: Title is empty", i)
		}
	}
}

func TestParseMobiautoGivesEveryListingItsOwnID(t *testing.T) {
	listings := parseMobiautoFixture(t, "testdata/mobiauto-search.html")

	seen := make(map[string]bool, len(listings))
	for _, l := range listings {
		if seen[l.ExternalID] {
			t.Fatalf("ExternalID %q appears twice", l.ExternalID)
		}
		seen[l.ExternalID] = true
	}
}

func TestParseMobiautoMapsResultFields(t *testing.T) {
	listings := parseMobiautoFixture(t, "testdata/mobiauto-search.html")

	want := model.RawListing{
		Source:       model.SourceMobiauto,
		ExternalID:   "28753136",
		URL:          "https://www.mobiauto.com.br/comprar/motos/sp-taboao-da-serra/harley-davidson/street-glide/2012/flhx/detalhes/28753136",
		Title:        "Harley-Davidson Street Glide FLHX",
		RawText:      "FLHX Touring",
		PriceText:    "R$ 65888",
		YearText:     "2012/2012",
		KmText:       "72197 km",
		LocationText: "Taboão da Serra - SP",
		ImageURL:     "https://image1.mobiauto.com.br/images/api/images/v1.0/644738305/transform/fl_progressive,f_webp,q_70,w_640",
	}

	for _, got := range listings {
		if got.ExternalID != want.ExternalID {
			continue
		}
		if got != want {
			t.Fatalf("listing =\n%+v\nwant\n%+v", got, want)
		}
		return
	}
	t.Fatalf("listing %s not found in the fixture", want.ExternalID)
}

func TestParseMobiautoTakesTheURLThePagePublishes(t *testing.T) {
	listings := parseMobiautoFixture(t, "testdata/mobiauto-search.html")

	for _, l := range listings {
		if strings.Contains(l.URL, "?") {
			t.Errorf("listing %s: URL = %q, want the tracking query dropped", l.ExternalID, l.URL)
		}
		if !strings.Contains(l.URL, "/detalhes/"+l.ExternalID) {
			t.Errorf("listing %s: URL = %q, want the anchor the page publishes for that id", l.ExternalID, l.URL)
		}
	}
}

func TestParseMobiautoReturnsNoListingsWhenSearchIsEmpty(t *testing.T) {
	f, err := os.Open("testdata/mobiauto-search-empty.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseMobiauto(f)
	if err != nil {
		t.Fatalf("a search with no results is not an error: %v", err)
	}
	if len(listings) != 0 {
		t.Fatalf("len(listings) = %d, want 0", len(listings))
	}
}

func TestParseMobiautoReturnsErrorWhenThePayloadIsMissing(t *testing.T) {
	blocked := stringReader("<html><body>Just a moment, checking your browser</body></html>")
	if _, err := ParseMobiauto(blocked); err == nil {
		t.Fatal("ParseMobiauto should return an error when the page carries no payload")
	}
}

func TestParseMobiautoReturnsErrorWhenTheResultsKeyIsMissing(t *testing.T) {
	page := mobiautoPage(`{"props":{"pageProps":{"deals":{"numResults":5}}}}`)
	if _, err := ParseMobiauto(page); err == nil {
		t.Fatal("ParseMobiauto should return an error when the results key is gone")
	}
}

func TestParseMobiautoSkipsAMalformedResult(t *testing.T) {
	page := mobiautoPage(`{"props":{"pageProps":{"deals":{"numResults":2,"results":[
		{"id":1,"price":72000,"km":50000,"trim":{"name":"FLHX","make":{"name":"Harley-Davidson"},"model":{"name":"Street Glide","year":2014},"productionYear":2014},"dealer":{"location":{"city":"Curitiba","state":"PR"}}},
		{"price":65000,"trim":{"name":"FLHX"}}
	]}}}}`, 1)

	listings, err := ParseMobiauto(page)
	if err != nil {
		t.Fatalf("ParseMobiauto: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1: the result without an id is skipped in silence", len(listings))
	}
	if got, want := listings[0].ExternalID, "1"; got != want {
		t.Errorf("ExternalID = %q, want %q", got, want)
	}
}

func TestParseMobiautoReturnsErrorWhenNoResultYieldsAListing(t *testing.T) {
	page := mobiautoPage(`{"props":{"pageProps":{"deals":{"numResults":2,"results":[
		{"price":72000,"trim":{"name":"FLHX"}},
		{"price":65000,"trim":{"name":"FLTRX"}}
	]}}}}`)

	if _, err := ParseMobiauto(page); err == nil {
		t.Fatal("ParseMobiauto should return an error when the results array is populated but no result can be read")
	}
}

func TestParseMobiautoReportsASearchThatSpansMorePages(t *testing.T) {
	listings, err := ParseMobiauto(mobiautoFixtureWithNumResults(t, 305))
	if err == nil {
		t.Fatal("ParseMobiauto should report a search that does not fit in one page")
	}
	if got, want := len(listings), 5; got != want {
		t.Fatalf("len(listings) = %d, want %d: the page that was read is worth keeping", got, want)
	}
	for _, want := range []string{"305", "5"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestParseMobiautoAcceptsAFullSinglePage(t *testing.T) {
	f, err := os.Open("testdata/mobiauto-search.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	if _, err := ParseMobiauto(f); err != nil {
		t.Fatalf("a search that fits in one page is not an overflow: %v", err)
	}
}

func TestParseMobiautoKeepsBothYearsWhenTheyDiffer(t *testing.T) {
	page := mobiautoPage(`{"props":{"pageProps":{"deals":{"numResults":1,"results":[
		{"id":9713586,"price":100000,"km":1,"trim":{"name":"ULTRA FLTRU","make":{"name":"Harley-Davidson"},"model":{"name":"Road Glide","year":2019},"productionYear":2018},"dealer":{"location":{"city":"Porto Ferreira","state":"SP"}}}
	]}}}}`, 9713586)

	listings, err := ParseMobiauto(page)
	if err != nil {
		t.Fatalf("ParseMobiauto: %v", err)
	}
	if got, want := listings[0].YearText, "2018/2019"; got != want {
		t.Fatalf("YearText = %q, want %q: the model year is the one the matcher needs", got, want)
	}
}

func TestParseMobiautoLeavesKmEmptyWhenTheResultHasNone(t *testing.T) {
	page := mobiautoPage(`{"props":{"pageProps":{"deals":{"numResults":1,"results":[
		{"id":2,"price":150000,"deal0km":true,"trim":{"name":"CVO","make":{"name":"Harley-Davidson"},"model":{"name":"Road Glide","year":2025},"productionYear":2025},"dealer":{"location":{"city":"Curitiba","state":"PR"}}}
	]}}}}`, 2)

	listings, err := ParseMobiauto(page)
	if err != nil {
		t.Fatalf("ParseMobiauto: %v", err)
	}
	if listings[0].KmText != "" {
		t.Errorf("KmText = %q, want empty when the result carries no odometer", listings[0].KmText)
	}
}

func TestMobiautoIsASource(t *testing.T) {
	var s Source = NewMobiauto(nil, nil)
	if s.Name() != model.SourceMobiauto {
		t.Errorf("Name() = %q, want %q", s.Name(), model.SourceMobiauto)
	}
}

func TestMobiautoDefaultsToPlainHTTPInsteadOfTheBrowser(t *testing.T) {
	m := NewMobiauto(nil, nil)
	if _, ok := m.fetcher.(*HTTPFetcher); !ok {
		t.Fatalf("fetcher = %T, want *HTTPFetcher: mobiauto must keep working when Chrome is down", m.fetcher)
	}
}

func TestMobiautoFetchCollectsEveryURL(t *testing.T) {
	fixture, err := os.ReadFile("testdata/mobiauto-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture)}
	m := NewMobiauto(fetcher, []string{"https://mobiauto.test/street", "https://mobiauto.test/road"})
	m.delay = 0

	listings, err := m.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := len(listings), 10; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	want := []string{"https://mobiauto.test/street", "https://mobiauto.test/road"}
	if !slices.Equal(fetcher.asked, want) {
		t.Errorf("asked for %q, want %q", fetcher.asked, want)
	}
}

func TestMobiautoFetchFailsWhenTheRequestFails(t *testing.T) {
	fetcher := &fakePageFetcher{err: errors.New("dns is down")}
	m := NewMobiauto(fetcher, []string{"https://mobiauto.test/street"})
	m.delay = 0

	if _, err := m.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch should fail when the page cannot be fetched")
	}
}

func TestMobiautoFetchStopsOnACancelledContext(t *testing.T) {
	fixture, err := os.ReadFile("testdata/mobiauto-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture)}
	m := NewMobiauto(fetcher, []string{"https://mobiauto.test/street", "https://mobiauto.test/road"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := m.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch error = %v, want context.Canceled", err)
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked for %q, want no request at all on a cancelled context", fetcher.asked)
	}
}

func TestMobiautoFetchKeepsWhatItGatheredWhenAURLFails(t *testing.T) {
	fixture, err := os.ReadFile("testdata/mobiauto-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture), failOn: "https://mobiauto.test/road"}
	m := NewMobiauto(fetcher, []string{"https://mobiauto.test/street", "https://mobiauto.test/road", "https://mobiauto.test/electra"})
	m.delay = 0

	listings, err := m.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should report the failure")
	}
	if got, want := len(listings), 5; got != want {
		t.Fatalf("len(listings) = %d, want %d: the first url's listings are worth keeping", got, want)
	}
}

func TestMobiautoFetchKeepsThePageItReadWhenTheSearchOverflows(t *testing.T) {
	page, err := io.ReadAll(mobiautoFixtureWithNumResults(t, 305))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(page)}
	m := NewMobiauto(fetcher, []string{"https://mobiauto.test/street"})
	m.delay = 0

	listings, err := m.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should report the overflow")
	}
	if got, want := len(listings), 5; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	if !strings.Contains(err.Error(), "https://mobiauto.test/street") {
		t.Errorf("error %q does not name the url that overflowed", err)
	}
}

func TestHTTPFetcherSendsTheBrowserUserAgent(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		fmt.Fprint(w, "hello")
	}))
	defer server.Close()

	page, err := NewHTTPFetcher().FetchPage(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if page != "hello" {
		t.Errorf("page = %q, want the body", page)
	}
	if got != defaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", got, defaultUserAgent)
	}
}

func TestHTTPFetcherFailsOnANonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "denied")
	}))
	defer server.Close()

	_, err := NewHTTPFetcher().FetchPage(context.Background(), server.URL)
	if err == nil {
		t.Fatal("FetchPage should fail on a non-200 answer")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not name the status code", err)
	}
}

func TestHTTPFetcherStopsOnACancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := NewHTTPFetcher().FetchPage(ctx, server.URL); !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchPage error = %v, want context.Canceled", err)
	}
}

func TestHTTPFetcherIsAPageFetcher(t *testing.T) {
	var _ PageFetcher = NewHTTPFetcher()
}

func mobiautoPage(payload string, ids ...int64) io.Reader {
	var anchors strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&anchors,
			`<a href="https://www.mobiauto.com.br/comprar/motos/sp-sao-paulo/harley-davidson/street-glide/2014/flhx/detalhes/%d?page=detail">card</a>`, id)
	}
	return stringReader(`<html><body>` + anchors.String() +
		`<script id="__NEXT_DATA__" type="application/json">` + payload + `</script></body></html>`)
}

func mobiautoFixtureWithNumResults(t *testing.T, total int) io.Reader {
	t.Helper()
	page, err := os.ReadFile("testdata/mobiauto-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	const single = `"numResults":5`
	if !bytes.Contains(page, []byte(single)) {
		t.Fatalf("fixture no longer carries %s", single)
	}
	doctored := bytes.Replace(page, []byte(single), fmt.Appendf(nil, `"numResults":%d`, total), 1)
	return bytes.NewReader(doctored)
}

func parseMobiautoFixture(t *testing.T, path string) []model.RawListing {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseMobiauto(f)
	if err != nil {
		t.Fatalf("ParseMobiauto: %v", err)
	}
	return listings
}
