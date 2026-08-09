package source

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
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

	want := model.RawListing{
		Source:       "olx",
		ExternalID:   "1499153946",
		URL:          "https://pr.olx.com.br/regiao-de-curitiba-e-paranagua/autos-e-pecas/motos/harley-davidson-street-glide-ultra-2025-1499153946",
		Title:        "HARLEY-DAVIDSON STREET GLIDE ULTRA 2025",
		PriceText:    "R$ 185.000",
		YearText:     "2025",
		KmText:       "4780",
		LocationText: "Curitiba - PR",
		ImageURL:     "https://img.olx.com.br/images/76/769644421707054.jpg",
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

	var agents []string
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents = append(agents, r.Header.Get("User-Agent"))
		paths = append(paths, r.URL.Path)
		w.Write(fixture)
	}))
	defer server.Close()

	o := NewOLX(server.Client(), []string{server.URL + "/street", server.URL + "/road"})
	o.delay = 0

	listings, err := o.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := len(listings), 84; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	if got, want := len(paths), 2; got != want {
		t.Fatalf("requested %d urls, want %d", got, want)
	}
	for i, a := range agents {
		if a != defaultUserAgent {
			t.Errorf("request %d: User-Agent = %q, want %q", i, a, defaultUserAgent)
		}
	}
}

func TestOLXFetchFailsOnUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	o := NewOLX(server.Client(), []string{server.URL})
	o.delay = 0

	if _, err := o.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch should fail when the server answers 403")
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
