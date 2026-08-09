package source

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
)

func TestParseMercadoLivreExtractsListings(t *testing.T) {
	listings := parseMercadoLivreFixture(t, "testdata/mercadolivre-search.html")
	if len(listings) == 0 {
		t.Fatal("expected at least one listing from the fixture")
	}

	for i, l := range listings {
		if l.Source != "mercadolivre" {
			t.Errorf("listing %d: Source = %q, want mercadolivre", i, l.Source)
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

func TestParseMercadoLivreReadsEveryCard(t *testing.T) {
	listings := parseMercadoLivreFixture(t, "testdata/mercadolivre-search.html")
	if got, want := len(listings), 5; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
}

func TestParseMercadoLivreMapsCardFields(t *testing.T) {
	listings := parseMercadoLivreFixture(t, "testdata/mercadolivre-search.html")

	want := model.RawListing{
		Source:       "mercadolivre",
		ExternalID:   "MLB4820336511",
		URL:          "https://moto.mercadolivre.com.br/MLB-4820336511-harley-davidson-street-glide-custom-_JM",
		Title:        "Harley-davidson Street Glide Custom",
		RawText:      "2013 71.438 Km",
		PriceText:    "R$ 59.900",
		YearText:     "2013 71.438 Km",
		KmText:       "2013 71.438 Km",
		LocationText: "São José Dos Campos - São Paulo",
		ImageURL:     "https://http2.mlstatic.com/D_Q_NP_2X_905140-MLB112701412446_062026-E-harley-davidson-street-glide-custom.webp",
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

func TestParseMercadoLivreDropsTheTrackingFragment(t *testing.T) {
	listings := parseMercadoLivreFixture(t, "testdata/mercadolivre-search.html")

	for _, l := range listings {
		if strings.Contains(l.URL, "#") {
			t.Fatalf("URL %q still carries the per-request tracking fragment", l.URL)
		}
	}
}

func TestParseMercadoLivreFeedsYearAndKmToTheNormalizer(t *testing.T) {
	listings := parseMercadoLivreFixture(t, "testdata/mercadolivre-search.html")

	for _, raw := range listings {
		if raw.ExternalID != "MLB4820336511" {
			continue
		}
		l := normalize.Normalize(raw)
		if l.Year == nil || *l.Year != 2013 {
			t.Errorf("Year = %v, want 2013", l.Year)
		}
		if l.Km == nil || *l.Km != 71438 {
			t.Errorf("Km = %v, want 71438", l.Km)
		}
		if l.PriceCents == nil || *l.PriceCents != 5990000 {
			t.Errorf("PriceCents = %v, want 5990000", l.PriceCents)
		}
		if l.Bike != model.BikeStreetGlide {
			t.Errorf("Bike = %q, want %q", l.Bike, model.BikeStreetGlide)
		}
		if l.State != "SP" {
			t.Errorf("State = %q, want SP", l.State)
		}
		return
	}
	t.Fatal("listing MLB4820336511 not found in the fixture")
}

func TestParseMercadoLivreTakesTheCurrentPriceNotThePreviousOneSynthetic(t *testing.T) {
	listings, err := ParseMercadoLivre(stringReader(mercadoLivreCard(`
		<div class="poly-component__price">
			<div class="poly-price__labels">
				<s class="andes-money-amount andes-money-amount--previous">
					<span class="andes-money-amount__fraction">120.000</span>
				</s>
			</div>
			<div class="poly-price__current">
				<span class="andes-money-amount">
					<span class="andes-money-amount__fraction">99.900</span>
					<span class="andes-money-amount__cents">50</span>
				</span>
			</div>
		</div>`)))
	if err != nil {
		t.Fatalf("ParseMercadoLivre: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1", len(listings))
	}
	if got, want := listings[0].PriceText, "R$ 99.900,50"; got != want {
		t.Fatalf("PriceText = %q, want %q: the struck-through price is not what the seller asks", got, want)
	}
}

func TestParseMercadoLivreKeepsCardsWithoutOptionalFields(t *testing.T) {
	listings, err := ParseMercadoLivre(stringReader(mercadoLivreCard("")))
	if err != nil {
		t.Fatalf("ParseMercadoLivre: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1", len(listings))
	}
	got := listings[0]
	if got.PriceText != "" || got.YearText != "" || got.KmText != "" || got.LocationText != "" || got.ImageURL != "" {
		t.Fatalf("optional fields should stay empty, got %+v", got)
	}
	if got.ExternalID != "MLB4820336511" || got.Title != "Street Glide" {
		t.Fatalf("listing = %+v", got)
	}
}

func TestParseMercadoLivreReadsCatalogItemIDs(t *testing.T) {
	page := `<html><body><ol class="ui-search-layout">` +
		`<li class="ui-search-layout__item"><div class="poly-card"><h3>` +
		`<a class="poly-component__title" href="https://www.mercadolivre.com.br/capa-para-cobrir-moto/up/MLBU3375127101#tracking">Capa</a>` +
		`</h3></div></li></ol></body></html>`

	listings, err := ParseMercadoLivre(stringReader(page))
	if err != nil {
		t.Fatalf("ParseMercadoLivre: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1: catalog cards carry a two-letter site id", len(listings))
	}
	if got, want := listings[0].ExternalID, "MLBU3375127101"; got != want {
		t.Errorf("ExternalID = %q, want %q", got, want)
	}
}

func TestParseMercadoLivreSkipsASingleCardWithoutALink(t *testing.T) {
	page := `<html><body><ol class="ui-search-layout">` +
		`<li class="ui-search-layout__item"><div class="poly-card"><h3>Street Glide</h3></div></li>` +
		`<li class="ui-search-layout__item"><div class="poly-card"><h3>` +
		`<a class="poly-component__title" href="https://moto.mercadolivre.com.br/MLB-4820336511-street-glide-_JM">Street Glide</a>` +
		`</h3></div></li></ol></body></html>`

	listings, err := ParseMercadoLivre(stringReader(page))
	if err != nil {
		t.Fatalf("one broken card among good ones is not a page failure: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("len(listings) = %d, want 1: a card without a link is not a listing", len(listings))
	}
}

func TestParseMercadoLivreReturnsErrorWhenNoCardYieldsAListing(t *testing.T) {
	page := `<html><body><ol class="ui-search-layout">` +
		`<li class="ui-search-layout__item"><div class="poly-card"><h3>Street Glide</h3></div></li>` +
		`<li class="ui-search-layout__item"><div class="poly-card"><h3>Road Glide</h3></div></li>` +
		`</ol></body></html>`

	if _, err := ParseMercadoLivre(stringReader(page)); err == nil {
		t.Fatal("ParseMercadoLivre should return an error when cards are present but none can be read")
	}
}

func TestParseMercadoLivreReturnsErrorWhenTheIDPatternStopsMatching(t *testing.T) {
	page := `<html><body><ol class="ui-search-layout">` +
		`<li class="ui-search-layout__item"><div class="poly-card"><h3>` +
		`<a class="poly-component__title" href="https://moto.mercadolivre.com.br/street-glide-2015">Street Glide</a>` +
		`</h3></div></li></ol></body></html>`

	if _, err := ParseMercadoLivre(stringReader(page)); err == nil {
		t.Fatal("ParseMercadoLivre should return an error when no href carries a readable item id")
	}
}

func TestParseMercadoLivreReturnsAnEmptySliceWhenSearchIsEmpty(t *testing.T) {
	listings := parseMercadoLivreFixture(t, "testdata/mercadolivre-search-empty.html")
	if listings == nil {
		t.Fatal("an empty search should answer with an empty slice, not nil")
	}
}

func TestParseMercadoLivreReturnsErrorWhenBlocked(t *testing.T) {
	if _, err := ParseMercadoLivre(stringReader("<html><body></body></html>")); err == nil {
		t.Fatal("ParseMercadoLivre should return an error when no result container exists")
	}
}

func TestParseMercadoLivreReturnsNoListingsWhenSearchIsEmpty(t *testing.T) {
	f, err := os.Open("testdata/mercadolivre-search-empty.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseMercadoLivre(f)
	if err != nil {
		t.Fatalf("a search with no results is not an error: %v", err)
	}
	if len(listings) != 0 {
		t.Fatalf("len(listings) = %d, want 0", len(listings))
	}
}

func TestMercadoLivreIsASource(t *testing.T) {
	var s Source = NewMercadoLivre(nil, nil)
	if s.Name() != "mercadolivre" {
		t.Errorf("Name() = %q, want mercadolivre", s.Name())
	}
}

func TestMercadoLivreFetchCollectsEveryURL(t *testing.T) {
	fixture, err := os.ReadFile("testdata/mercadolivre-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture)}
	m := NewMercadoLivre(fetcher, []string{"https://ml.test/street", "https://ml.test/road"})
	m.delay = 0

	listings, err := m.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := len(listings), 10; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	want := []string{"https://ml.test/street", "https://ml.test/road"}
	if !slices.Equal(fetcher.asked, want) {
		t.Errorf("asked for %q, want %q", fetcher.asked, want)
	}
}

func TestMercadoLivreFetchFailsWhenTheBrowserFails(t *testing.T) {
	fetcher := &fakePageFetcher{err: errors.New("chrome is not listening")}
	m := NewMercadoLivre(fetcher, []string{"https://ml.test/street"})
	m.delay = 0

	if _, err := m.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch should fail when the page cannot be fetched")
	}
}

func TestMercadoLivreFetchFailsWhenThePageIsBlocked(t *testing.T) {
	fetcher := &fakePageFetcher{page: "<html><body>Verificação de segurança</body></html>"}
	m := NewMercadoLivre(fetcher, []string{"https://ml.test/street"})
	m.delay = 0

	if _, err := m.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch should fail when the browser lands on a challenge page")
	}
}

func TestMercadoLivreFetchStopsOnACancelledContext(t *testing.T) {
	fetcher := &fakePageFetcher{page: "<html></html>"}
	m := NewMercadoLivre(fetcher, []string{"https://ml.test/street", "https://ml.test/road"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := m.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch error = %v, want context.Canceled", err)
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked for %q, want no request at all on a cancelled context", fetcher.asked)
	}
}

func TestMercadoLivreFetchKeepsWhatItGatheredWhenAURLFails(t *testing.T) {
	fixture, err := os.ReadFile("testdata/mercadolivre-search.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	fetcher := &fakePageFetcher{page: string(fixture), failOn: "https://ml.test/road"}
	m := NewMercadoLivre(fetcher, []string{"https://ml.test/street", "https://ml.test/road", "https://ml.test/electra"})
	m.delay = 0

	listings, err := m.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should report the failure")
	}
	if got, want := len(listings), 5; got != want {
		t.Fatalf("len(listings) = %d, want %d: the first url's listings are worth keeping", got, want)
	}
}

func mercadoLivreCard(extra string) string {
	return `<html><body><ol class="ui-search-layout"><li class="ui-search-layout__item"><div class="poly-card">` +
		`<h3 class="poly-component__title-wrapper">` +
		`<a class="poly-component__title" href="https://moto.mercadolivre.com.br/MLB-4820336511-street-glide-_JM#polycard_client=search-desktop&amp;tracking_id=abc">Street Glide</a>` +
		`</h3>` + extra +
		`</div></li></ol></body></html>`
}

func parseMercadoLivreFixture(t *testing.T, path string) []model.RawListing {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseMercadoLivre(f)
	if err != nil {
		t.Fatalf("ParseMercadoLivre: %v", err)
	}
	return listings
}
