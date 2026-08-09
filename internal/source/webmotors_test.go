package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestParseWebmotorsExtractsListings(t *testing.T) {
	listings := parseWebmotorsFixture(t, "testdata/webmotors-search.html")

	if got, want := len(listings), 42; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	for i, l := range listings {
		if l.Source != model.SourceWebmotors {
			t.Errorf("listing %d: Source = %q, want %q", i, l.Source, model.SourceWebmotors)
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

func TestParseWebmotorsGivesEveryListingItsOwnID(t *testing.T) {
	listings := parseWebmotorsFixture(t, "testdata/webmotors-search.html")

	seen := make(map[string]bool, len(listings))
	for _, l := range listings {
		if seen[l.ExternalID] {
			t.Fatalf("ExternalID %q appears twice", l.ExternalID)
		}
		seen[l.ExternalID] = true
	}
}

func TestParseWebmotorsMapsResultFields(t *testing.T) {
	listings := parseWebmotorsFixture(t, "testdata/webmotors-search.html")

	want := model.RawListing{
		Source:       model.SourceWebmotors,
		ExternalID:   "2789183",
		URL:          "https://www.webmotors.com.br/comprar/harley-davidson/street-glide/1700cc/2014/2789183",
		Title:        "HARLEY-DAVIDSON STREET GLIDE",
		RawText:      "Quadro Rushmore, Pneus bons, Sissy bar destacável, Farol de led, Filtro de ar lavável novo, óleo motor (Motul 100% sintético), primária e filtro de óleo recém trocados.  Aceito entrada + saldo em até 24x no cartão.  Sem trocas, já comprei outra moto.  **NÃO NEGOCIO COM LOJISTAS**",
		PriceText:    "R$ 66900",
		YearText:     "2014/2014",
		KmText:       "43500 km",
		LocationText: "Sorocaba - SP",
		ImageURL:     "https://image.webmotors.com.br/_fotos/anunciousados/gigante/2025/202512/20251219/harleydavidsonstreetglide-WMIMAGEM11305825214.jpg",
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

func TestParseWebmotorsKeepsBothYearsWhenTheyDiffer(t *testing.T) {
	listings := parseWebmotorsFixture(t, "testdata/webmotors-search.html")

	for _, l := range listings {
		if l.ExternalID != "2944774" {
			continue
		}
		if got, want := l.YearText, "2011/2012"; got != want {
			t.Fatalf("YearText = %q, want %q: the model year is the one the matcher needs", got, want)
		}
		if got, want := l.URL, "https://www.webmotors.com.br/comprar/harley-davidson/street-glide/1800cc/2011-2012/2944774"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return
	}
	t.Fatal("listing 2944774 not found in the fixture")
}

func TestParseWebmotorsKeepsListingsWithoutAPhoto(t *testing.T) {
	listings := parseWebmotorsFixture(t, "testdata/webmotors-search.html")

	for _, l := range listings {
		if l.ExternalID != "2707181" {
			continue
		}
		if l.ImageURL != "" {
			t.Fatalf("ImageURL = %q, want empty: the fixture result carries no photo", l.ImageURL)
		}
		return
	}
	t.Fatal("listing 2707181 not found in the fixture")
}

func TestParseWebmotorsReturnsNoListingsWhenSearchIsEmpty(t *testing.T) {
	f, err := os.Open("testdata/webmotors-search-empty.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseWebmotors(f)
	if err != nil {
		t.Fatalf("a search with no results is not an error: %v", err)
	}
	if len(listings) != 0 {
		t.Fatalf("len(listings) = %d, want 0", len(listings))
	}
}

func TestParseWebmotorsReturnsErrorWhenBlocked(t *testing.T) {
	blocked := stringReader(`<html><head><title>Access to this page has been denied</title></head><body>px-captcha</body></html>`)
	if _, err := ParseWebmotors(blocked); err == nil {
		t.Fatal("ParseWebmotors should return an error when the response is not the search json")
	}
}

func TestParseWebmotorsReturnsErrorWhenResultsKeyIsMissing(t *testing.T) {
	page := webmotorsPage(`{"Count":63,"Filters":[]}`)
	if _, err := ParseWebmotors(page); err == nil {
		t.Fatal("ParseWebmotors should return an error when the results key is gone")
	}
}

func TestParseWebmotorsSkipsAMalformedResult(t *testing.T) {
	page := webmotorsPage(`{"SearchResults":[
		{"UniqueId":1,"Specification":{"Title":"HARLEY-DAVIDSON STREET GLIDE ","Make":{"Value":"HARLEY-DAVIDSON"},"Model":{"Value":"STREET GLIDE"},"YearFabrication":"2014","YearModel":2014.0,"Odometer":50000.0,"CubicCentimeter":1700.0},"Seller":{"City":"Curitiba","State":"Paraná (PR)"},"Prices":{"Price":72000.0}},
		{"Specification":{"Title":"HARLEY-DAVIDSON ROAD GLIDE "}}
	]}`)

	listings, err := ParseWebmotors(page)
	if err != nil {
		t.Fatalf("ParseWebmotors: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1: the result without an id is skipped in silence", len(listings))
	}
	if got, want := listings[0].ExternalID, "1"; got != want {
		t.Errorf("ExternalID = %q, want %q", got, want)
	}
}

func TestParseWebmotorsReturnsErrorWhenNoResultYieldsAListing(t *testing.T) {
	page := webmotorsPage(`{"SearchResults":[
		{"Specification":{"Title":"HARLEY-DAVIDSON STREET GLIDE "}},
		{"Specification":{"Title":"HARLEY-DAVIDSON ROAD GLIDE "}}
	]}`)

	if _, err := ParseWebmotors(page); err == nil {
		t.Fatal("ParseWebmotors should return an error when the results array is populated but no result can be read")
	}
}

func TestParseWebmotorsAcceptsTheOnlyPageOfTheFixture(t *testing.T) {
	f, err := os.Open("testdata/webmotors-search.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	if _, err := ParseWebmotors(f); err != nil {
		t.Fatalf("a single page search is not an overflow: %v", err)
	}
}

func TestParseWebmotorsReportsASearchThatSpansMorePages(t *testing.T) {
	listings, err := ParseWebmotors(webmotorsFixtureWithPageTotal(t, 3))
	if err == nil {
		t.Fatal("ParseWebmotors should report a search that does not fit in one page")
	}
	if got, want := len(listings), 42; got != want {
		t.Fatalf("len(listings) = %d, want %d: the page that was read is worth keeping", got, want)
	}
	for _, want := range []string{"3 pages", "42"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestParseWebmotorsDoesNotReportOverflowOnAnEmptySearch(t *testing.T) {
	f, err := os.Open("testdata/webmotors-search-empty.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	if _, err := ParseWebmotors(f); err != nil {
		t.Fatalf("an empty search reports zero pages, not an overflow: %v", err)
	}
}

func TestWebmotorsFetchKeepsThePageItReadWhenTheSearchOverflows(t *testing.T) {
	page, err := io.ReadAll(webmotorsFixtureWithPageTotal(t, 2))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(page)}
	w := NewWebmotors(fetcher, []string{"https://webmotors.test/street"})
	w.delay = 0

	listings, err := w.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should report the overflow")
	}
	if got, want := len(listings), 42; got != want {
		t.Fatalf("len(listings) = %d, want %d: the overflowing page is still worth keeping", got, want)
	}
	if !strings.Contains(err.Error(), "https://webmotors.test/street") {
		t.Errorf("error %q does not name the url that overflowed", err)
	}
}

func webmotorsFixtureWithPageTotal(t *testing.T, total int) io.Reader {
	t.Helper()
	page, err := os.ReadFile("testdata/webmotors-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	const single = `"PageTotal":1`
	if !bytes.Contains(page, []byte(single)) {
		t.Fatalf("fixture no longer carries %s", single)
	}
	doctored := bytes.Replace(page, []byte(single), fmt.Appendf(nil, `"PageTotal":%d`, total), 1)
	return bytes.NewReader(doctored)
}

func TestParseWebmotorsAcceptsRawJSON(t *testing.T) {
	body := stringReader(`{"SearchResults":[{"UniqueId":7,"Specification":{"Title":"HARLEY-DAVIDSON ROAD GLIDE ","Make":{"Value":"HARLEY-DAVIDSON"},"Model":{"Value":"ROAD GLIDE"},"YearFabrication":"2015","YearModel":2015.0,"Odometer":10000.0,"CubicCentimeter":1700.0},"Seller":{"City":"Rio de Janeiro","State":"Rio de Janeiro (RJ)"},"Prices":{"Price":70000.0}}]}`)

	listings, err := ParseWebmotors(body)
	if err != nil {
		t.Fatalf("ParseWebmotors: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1", len(listings))
	}
	if got, want := listings[0].LocationText, "Rio de Janeiro - RJ"; got != want {
		t.Errorf("LocationText = %q, want %q", got, want)
	}
}

func TestParseWebmotorsFallsBackToTheStateNameWithoutAnAbbreviation(t *testing.T) {
	page := webmotorsPage(`{"SearchResults":[{"UniqueId":8,"Specification":{"Title":"HARLEY-DAVIDSON STREET GLIDE ","Make":{"Value":"HARLEY-DAVIDSON"},"Model":{"Value":"STREET GLIDE"},"YearFabrication":"2014","YearModel":2014.0,"Odometer":10000.0,"CubicCentimeter":1700.0},"Seller":{"City":"Curitiba","State":"Paraná"},"Prices":{"Price":70000.0}}]}`)

	listings, err := ParseWebmotors(page)
	if err != nil {
		t.Fatalf("ParseWebmotors: %v", err)
	}
	if got, want := listings[0].LocationText, "Curitiba - Paraná"; got != want {
		t.Errorf("LocationText = %q, want %q", got, want)
	}
}

func TestWebmotorsIsASource(t *testing.T) {
	var s Source = NewWebmotors(nil, nil)
	if s.Name() != model.SourceWebmotors {
		t.Errorf("Name() = %q, want %q", s.Name(), model.SourceWebmotors)
	}
}

func TestWebmotorsFetchCollectsEveryURL(t *testing.T) {
	fixture, err := os.ReadFile("testdata/webmotors-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture)}
	w := NewWebmotors(fetcher, []string{"https://webmotors.test/street", "https://webmotors.test/road"})
	w.delay = 0

	listings, err := w.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := len(listings), 84; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	want := []string{"https://webmotors.test/street", "https://webmotors.test/road"}
	if !slices.Equal(fetcher.asked, want) {
		t.Errorf("asked for %q, want %q", fetcher.asked, want)
	}
}

func TestWebmotorsFetchFailsWhenTheBrowserFails(t *testing.T) {
	fetcher := &fakePageFetcher{err: errors.New("chrome is not listening")}
	w := NewWebmotors(fetcher, []string{"https://webmotors.test/street"})
	w.delay = 0

	if _, err := w.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch should fail when the page cannot be fetched")
	}
}

func TestWebmotorsFetchFailsWhenTheResponseIsABlockPage(t *testing.T) {
	fetcher := &fakePageFetcher{page: "<html><body>Access to this page has been denied</body></html>"}
	w := NewWebmotors(fetcher, []string{"https://webmotors.test/street"})
	w.delay = 0

	if _, err := w.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch should fail when the browser lands on a block page")
	}
}

func TestWebmotorsFetchStopsOnACancelledContext(t *testing.T) {
	fixture, err := os.ReadFile("testdata/webmotors-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture)}
	w := NewWebmotors(fetcher, []string{"https://webmotors.test/street", "https://webmotors.test/road"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := w.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch error = %v, want context.Canceled", err)
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked for %q, want no request at all on a cancelled context", fetcher.asked)
	}
}

func TestWebmotorsFetchKeepsWhatItGatheredWhenAURLFails(t *testing.T) {
	fixture, err := os.ReadFile("testdata/webmotors-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture), failOn: "https://webmotors.test/road"}
	w := NewWebmotors(fetcher, []string{"https://webmotors.test/street", "https://webmotors.test/road", "https://webmotors.test/electra"})
	w.delay = 0

	listings, err := w.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should report the failure")
	}
	if got, want := len(listings), 42; got != want {
		t.Fatalf("len(listings) = %d, want %d: the first url's listings are worth keeping", got, want)
	}
}

func webmotorsPage(payload string) io.Reader {
	return stringReader("<html><head></head><body><pre>" + payload + "</pre></body></html>")
}

func parseWebmotorsFixture(t *testing.T, path string) []model.RawListing {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseWebmotors(f)
	if err != nil {
		t.Fatalf("ParseWebmotors: %v", err)
	}
	return listings
}
