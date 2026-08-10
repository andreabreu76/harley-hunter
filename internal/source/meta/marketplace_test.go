package meta

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestParseMarketplaceReadsEveryCard(t *testing.T) {
	listings, err := ParseMarketplace(fixture(t, "marketplace-search.html"))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if got, want := len(listings), 18; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}

	seen := make(map[string]bool, len(listings))
	for _, l := range listings {
		if l.ExternalID == "" {
			t.Fatalf("listing %+v has no external id", l)
		}
		if seen[l.ExternalID] {
			t.Fatalf("external id %q appears twice", l.ExternalID)
		}
		seen[l.ExternalID] = true
	}
}

func TestParseMarketplaceFillsTheStructuredFields(t *testing.T) {
	listings, err := ParseMarketplace(fixture(t, "marketplace-search.html"))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}

	first := listings[0]
	want := model.RawListing{
		Source:       model.SourceMarketplace,
		ExternalID:   "27640613035566314",
		URL:          "https://www.facebook.com/marketplace/item/27640613035566314/",
		Title:        "2015 Harley-Davidson streetglide",
		PriceText:    "R$85.990",
		LocationText: "Curitiba, PR",
	}
	if first.Source != want.Source || first.ExternalID != want.ExternalID || first.URL != want.URL ||
		first.Title != want.Title || first.PriceText != want.PriceText || first.LocationText != want.LocationText {
		t.Errorf("first listing =\n%+v\nwant\n%+v", first, want)
	}
	if !strings.HasPrefix(first.ImageURL, "https://") {
		t.Errorf("ImageURL = %q, want the card thumbnail", first.ImageURL)
	}
}

func TestParseMarketplaceReadsThePriceAndCityOfEveryCard(t *testing.T) {
	listings, err := ParseMarketplace(fixture(t, "marketplace-search.html"))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	for _, l := range listings {
		if !strings.HasPrefix(l.PriceText, "R$") {
			t.Errorf("%s: PriceText = %q, want a price", l.ExternalID, l.PriceText)
		}
		if !strings.Contains(l.LocationText, ", ") {
			t.Errorf("%s: LocationText = %q, want city and state", l.ExternalID, l.LocationText)
		}
	}
}

func TestParseMarketplaceDropsPartsPricedBelowAMotorcycle(t *testing.T) {
	listings, err := ParseMarketplace(fixture(t, "marketplace-search.html"))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}

	kept := make(map[string]string, len(listings))
	for _, l := range listings {
		kept[l.ExternalID] = l.Title
	}
	for id, title := range map[string]string{
		"1454486673386424": "Pisadeira moto Harley electra glide, R$250",
		"2073348313246483": "Peças para Harley-Davidson, R$1",
		"1577711237022663": "Escapamento original completo, R$1.700",
		"2478266602648337": "Banco original Harley Davidson Street Glide, R$1.000",
		"1719016595759927": "2013 Harley-Davidson electraglide, R$1.300",
	} {
		if _, found := kept[id]; found {
			t.Errorf("%s (%s) was kept: nothing under R$ 5.000 is a motorcycle", id, title)
		}
	}
}

func TestParseMarketplaceAcceptsAPageOfNothingButParts(t *testing.T) {
	page := marketplacePage(
		cardHTML("111", "R$250", "Pisadeira moto Harley electra glide", "Curitiba, PR") +
			cardHTML("222", "R$1.000", "Banco original Harley Davidson Street Glide", "Curitiba, PR"))

	listings, err := ParseMarketplace(strings.NewReader(page))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v: cards that were read and filtered are not a broken layout", err)
	}
	if len(listings) != 0 {
		t.Errorf("len(listings) = %d, want 0", len(listings))
	}
}

func TestMarketplacePriceCents(t *testing.T) {
	cases := []struct {
		text  string
		cents int64
		ok    bool
	}{
		{"R$85.990", 8599000, true},
		{"R$1.000", 100000, true},
		{"R$250", 25000, true},
		{"R$1", 100, true},
		{"R$75.990,50", 7599050, true},
		{"R$ 68.900", 6890000, true},
		{"Grátis", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		cents, ok := marketplacePriceCents(c.text)
		if ok != c.ok || cents != c.cents {
			t.Errorf("marketplacePriceCents(%q) = %d, %v, want %d, %v", c.text, cents, ok, c.cents, c.ok)
		}
	}
}

func TestParseMarketplaceAcceptsASearchWithNoResults(t *testing.T) {
	listings, err := ParseMarketplace(fixture(t, "marketplace-search-empty.html"))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v: a search with no result is not a failure", err)
	}
	if len(listings) != 0 {
		t.Errorf("len(listings) = %d, want 0", len(listings))
	}
}

func TestParseMarketplaceFailsOnTheLoginWall(t *testing.T) {
	_, err := ParseMarketplace(fixture(t, "marketplace-login.html"))
	if err == nil {
		t.Fatal("ParseMarketplace should fail when facebook serves the login page")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "login") {
		t.Errorf("error = %q, want it to name the login screen", err)
	}
}

func TestParseMarketplaceFailsWhenTheContainerIsGone(t *testing.T) {
	_, err := ParseMarketplace(strings.NewReader(`<html><body><div>nothing here</div></body></html>`))
	if err == nil {
		t.Fatal("ParseMarketplace should fail when the marketplace container is missing")
	}
}

func TestParseMarketplaceFailsWhenCardsYieldNothing(t *testing.T) {
	page := marketplacePage(`<a href="/marketplace/item/12345/?ref=search"><img src="https://cdn/a.jpg"></a>`)

	_, err := ParseMarketplace(strings.NewReader(page))
	if err == nil {
		t.Fatal("ParseMarketplace should fail when the page has cards and none can be read")
	}
}

func TestParseMarketplaceSkipsACardWithoutATitle(t *testing.T) {
	page := marketplacePage(
		cardHTML("111", "R$70.000", "2015 Harley-Davidson streetglide", "Curitiba, PR") +
			`<a href="/marketplace/item/222/"><img src="https://cdn/b.jpg"><span dir="auto">R$80.000</span></a>`)

	listings, err := ParseMarketplace(strings.NewReader(page))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if got, want := len(listings), 1; got != want {
		t.Fatalf("len(listings) = %d, want %d: the card with no title is skipped", got, want)
	}
	if got, want := listings[0].ExternalID, "111"; got != want {
		t.Errorf("ExternalID = %q, want %q", got, want)
	}
}

func TestParseMarketplaceReadsEachItemOnce(t *testing.T) {
	page := marketplacePage(
		cardHTML("111", "R$70.000", "Street Glide", "Curitiba, PR") +
			cardHTML("111", "R$70.000", "Street Glide", "Curitiba, PR"))

	listings, err := ParseMarketplace(strings.NewReader(page))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if got, want := len(listings), 1; got != want {
		t.Errorf("len(listings) = %d, want %d", got, want)
	}
}

func TestParseMarketplaceKeepsACardWithoutAPrice(t *testing.T) {
	page := marketplacePage(`<a href="/marketplace/item/333/"><img src="https://cdn/c.jpg"><span dir="auto">2015 Harley-Davidson streetglide</span><span dir="auto">Curitiba, PR</span></a>`)

	listings, err := ParseMarketplace(strings.NewReader(page))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if got, want := len(listings), 1; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	if listings[0].PriceText != "" {
		t.Errorf("PriceText = %q, want empty", listings[0].PriceText)
	}
	if got, want := listings[0].Title, "2015 Harley-Davidson streetglide"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
	if got, want := listings[0].LocationText, "Curitiba, PR"; got != want {
		t.Errorf("LocationText = %q, want %q", got, want)
	}
}

func TestMarketplaceFetchReadsEverySearch(t *testing.T) {
	urls := []string{
		"https://www.facebook.com/marketplace/curitiba/search?query=harley%20street%20glide&radius=100",
		"https://www.facebook.com/marketplace/saopaulo/search?query=harley%20street%20glide&radius=100",
	}
	page := fixtureString(t, "marketplace-search.html")
	fetcher := &fakePageFetcher{pages: map[string]string{urls[0]: page, urls[1]: page}}

	marketplace := NewMarketplace(fetcher, urls)
	marketplace.delay = func() time.Duration { return 0 }

	listings, err := marketplace.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := len(fetcher.asked), 2; got != want {
		t.Fatalf("asked %d urls, want %d", got, want)
	}
	if got, want := len(listings), 36; got != want {
		t.Errorf("len(listings) = %d, want %d", got, want)
	}
	if got, want := marketplace.Name(), model.SourceMarketplace; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestMarketplaceFetchStopsAtTheLoginWall(t *testing.T) {
	urls := []string{
		"https://www.facebook.com/marketplace/curitiba/search?query=harley%20street%20glide",
		"https://www.facebook.com/marketplace/saopaulo/search?query=harley%20street%20glide",
	}
	fetcher := &fakePageFetcher{pages: map[string]string{
		urls[0]: fixtureString(t, "marketplace-search.html"),
		urls[1]: fixtureString(t, "marketplace-login.html"),
	}}

	marketplace := NewMarketplace(fetcher, urls)
	marketplace.delay = func() time.Duration { return 0 }

	listings, err := marketplace.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should report the login wall")
	}
	if got, want := len(listings), 18; got != want {
		t.Errorf("len(listings) = %d, want %d: what was read before the wall survives", got, want)
	}
}

func TestMarketplaceWaitsARandomTimeBetweenSearches(t *testing.T) {
	marketplace := NewMarketplace(&fakePageFetcher{}, nil)

	seen := make(map[time.Duration]bool)
	for i := 0; i < 200; i++ {
		d := marketplace.delay()
		if d < instagramMinDelay || d > instagramMaxDelay {
			t.Fatalf("delay = %v, want between %v and %v", d, instagramMinDelay, instagramMaxDelay)
		}
		seen[d] = true
	}
	if len(seen) < 10 {
		t.Errorf("%d distinct delays in 200 draws, want a random spread", len(seen))
	}
}

func marketplacePage(inner string) string {
	return `<html><body><a href="/marketplace/create/">Criar novo classificado</a>` + inner + `</body></html>`
}

func cardHTML(id, price, title, location string) string {
	return fmt.Sprintf(`<a href="/marketplace/item/%s/?ref=search"><img src="https://cdn/%s.jpg"><span dir="auto">%s</span><span dir="auto">%s</span><span dir="auto">%s</span></a>`,
		id, id, price, title, location)
}

func TestParseMarketplaceDropsAListingAlreadyGone(t *testing.T) {
	for _, title := range []string{
		"VENDIDO Harley-Davidson Street Glide 2015",
		"Harley Road Glide 2015 - vendida",
		"ENTREGUE!! Harley-Davidson Street Glide Special",
		"harley street glide 2015 entregue",
	} {
		if !isSold(title) {
			t.Errorf("isSold(%q) = false, want true", title)
		}
	}
	for _, title := range []string{
		"Harley-Davidson Street Glide 2015",
		"Harley Road Glide entrega para todo o Brasil",
	} {
		if isSold(title) {
			t.Errorf("isSold(%q) = true, want false", title)
		}
	}
}

func TestParseMarketplaceDropsACardTheSellerMarkedGone(t *testing.T) {
	page := marketplacePage(
		cardHTML("111", "R$ 72.000", "Harley-Davidson Street Glide 2015", "Curitiba, PR") +
			cardHTML("222", "R$ 74.000", "VENDIDO Harley-Davidson Street Glide 2015", "Curitiba, PR") +
			cardHTML("333", "R$ 76.000", "ENTREGUE!! Harley-Davidson Road Glide 2015", "Curitiba, PR"))

	listings, err := ParseMarketplace(strings.NewReader(page))
	if err != nil {
		t.Fatalf("ParseMarketplace: %v", err)
	}
	if got, want := len(listings), 1; got != want {
		t.Fatalf("len(listings) = %d, want %d: a sold or delivered card is not on offer", got, want)
	}
	if listings[0].ExternalID != "111" {
		t.Errorf("kept %q, want the one still for sale", listings[0].ExternalID)
	}
}

func TestParseMarketplaceAcceptsAPageWhereEverythingSold(t *testing.T) {
	page := marketplacePage(
		cardHTML("111", "R$ 72.000", "VENDIDO Harley-Davidson Street Glide", "Curitiba, PR") +
			cardHTML("222", "R$ 74.000", "Harley Road Glide - entregue", "Curitiba, PR"))

	listings, err := ParseMarketplace(strings.NewReader(page))
	if err != nil {
		t.Fatalf("a page of sold cards is readable, not broken: %v", err)
	}
	if len(listings) != 0 {
		t.Fatalf("len(listings) = %d, want 0", len(listings))
	}
}
