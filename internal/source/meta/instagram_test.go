package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

type fakePageFetcher struct {
	pages  map[string]string
	asked  []string
	failOn string
}

func (f *fakePageFetcher) FetchPage(_ context.Context, url string) (string, error) {
	f.asked = append(f.asked, url)
	if url == f.failOn {
		return "", errors.New("fetch failed")
	}
	return f.pages[url], nil
}

func fixture(t *testing.T, name string) io.Reader {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return strings.NewReader(string(body))
}

func fixtureString(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(body)
}

func TestParseInstagramKeepsOnlyPostsWithASaleSignal(t *testing.T) {
	listings, err := ParseInstagram(fixture(t, "instagram-hashtag.html"))
	if err != nil {
		t.Fatalf("ParseInstagram: %v", err)
	}

	ids := make([]string, len(listings))
	for i, l := range listings {
		ids[i] = l.ExternalID
	}
	want := []string{"DWZCweaDb4X", "DPxNH-zEZ-G"}
	if len(ids) != len(want) {
		t.Fatalf("external ids = %q, want %q: the post without price or sale term must be dropped", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("external ids = %q, want %q", ids, want)
		}
	}
}

func TestParseInstagramFillsOnlyTheFieldsTheGridCarries(t *testing.T) {
	listings, err := ParseInstagram(fixture(t, "instagram-hashtag.html"))
	if err != nil {
		t.Fatalf("ParseInstagram: %v", err)
	}
	if len(listings) == 0 {
		t.Fatal("no listing parsed")
	}

	first := listings[0]
	if got, want := first.Source, model.SourceInstagram; got != want {
		t.Errorf("Source = %q, want %q", got, want)
	}
	if got, want := first.URL, "https://www.instagram.com/p/DWZCweaDb4X/"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
	if !strings.HasPrefix(first.RawText, "Alerta de pão quente.") {
		t.Errorf("RawText = %q, want the caption", first.RawText)
	}
	if !strings.HasPrefix(first.ImageURL, "https://") {
		t.Errorf("ImageURL = %q, want the thumbnail url", first.ImageURL)
	}
	for name, value := range map[string]string{
		"PriceText": first.PriceText, "YearText": first.YearText,
		"KmText": first.KmText, "LocationText": first.LocationText,
	} {
		if value != "" {
			t.Errorf("%s = %q, want empty: the grid carries no structured field", name, value)
		}
	}
}

func TestParseInstagramAcceptsAHashtagWithNoPosts(t *testing.T) {
	listings, err := ParseInstagram(fixture(t, "instagram-hashtag-empty.html"))
	if err != nil {
		t.Fatalf("ParseInstagram: %v: a hashtag with no results is not a failure", err)
	}
	if len(listings) != 0 {
		t.Errorf("len(listings) = %d, want 0", len(listings))
	}
}

func TestParseInstagramFailsOnTheLoginWall(t *testing.T) {
	_, err := ParseInstagram(fixture(t, "instagram-login.html"))
	if err == nil {
		t.Fatal("ParseInstagram should fail when instagram asks for login")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "login") {
		t.Errorf("error = %q, want it to name the login screen", err)
	}
}

func TestParseInstagramFailsWhenTheGridContainerIsGone(t *testing.T) {
	_, err := ParseInstagram(strings.NewReader(`<html><body><div id="other">nothing here</div></body></html>`))
	if err == nil {
		t.Fatal("ParseInstagram should fail when the grid container is missing")
	}
}

func TestParseInstagramSkipsPostsWithoutACaption(t *testing.T) {
	page := gridPage(`<a href="/p/AAAA/"><img src="https://cdn/a.jpg"></a>` + postHTML("BBBB", "Street Glide 2015 R$ 70.000,00"))

	listings, err := ParseInstagram(strings.NewReader(page))
	if err != nil {
		t.Fatalf("ParseInstagram: %v", err)
	}
	if got, want := len(listings), 1; got != want {
		t.Fatalf("len(listings) = %d, want %d: the post with no caption is skipped", got, want)
	}
	if got, want := listings[0].ExternalID, "BBBB"; got != want {
		t.Errorf("ExternalID = %q, want %q", got, want)
	}
}

func TestParseInstagramCapsThePostsItTakesFromOneHashtag(t *testing.T) {
	var posts strings.Builder
	for i := 0; i < instagramMaxPosts+5; i++ {
		posts.WriteString(postHTML(fmt.Sprintf("SHORT%02d", i), "Street Glide 2015 à venda R$ 70.000,00"))
	}

	listings, err := ParseInstagram(strings.NewReader(gridPage(posts.String())))
	if err != nil {
		t.Fatalf("ParseInstagram: %v", err)
	}
	if got, want := len(listings), instagramMaxPosts; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
}

func TestParseInstagramReadsEachPostOnce(t *testing.T) {
	page := gridPage(postHTML("AAAA", "Street Glide R$ 70.000,00") + postHTML("AAAA", "Street Glide R$ 70.000,00"))

	listings, err := ParseInstagram(strings.NewReader(page))
	if err != nil {
		t.Fatalf("ParseInstagram: %v", err)
	}
	if got, want := len(listings), 1; got != want {
		t.Errorf("len(listings) = %d, want %d: the same shortcode twice is one listing", got, want)
	}
}

func TestInstagramSaleSignal(t *testing.T) {
	cases := []struct {
		caption string
		want    bool
	}{
		{"Harley-Davidson STREET GLIDE SPECIAL 114 - 2022 - R$ 124.990,00", true},
		{"💰R$ 107.000,00", true},
		{"✅ DISPONÍVEL ✅️ HARLEY ULTRA LIMITED 2013", true},
		{"À VENDA! Street Glide 2013, motor TC103", true},
		{"Vendo minha Road Glide 2015, aceito propostas", true},
		{"*OBS: Aceitamos seu usado na troca !!*", false},
		{"Aceito troca em moto menor", true},
		{"Road Glide Special #garagem75raridades #harleydavidson", false},
		{"Harley Davidson, motocicletas raras? Trabalho sim, faz 26 anos já", false},
		{"SADAKO The first Turbocharged H-D CVO/ST Roadglide in Indonesia", false},
		{"", false},
	}
	for _, c := range cases {
		if got := hasSaleSignal(c.caption); got != c.want {
			t.Errorf("hasSaleSignal(%q) = %v, want %v", c.caption, got, c.want)
		}
	}
}

func TestParseInstagramGivesEveryListingATitle(t *testing.T) {
	listings, err := ParseInstagram(fixture(t, "instagram-hashtag.html"))
	if err != nil {
		t.Fatalf("ParseInstagram: %v", err)
	}

	want := map[string]string{
		"DWZCweaDb4X": "Alerta de pão quente.",
		"DPxNH-zEZ-G": "Harley-Davidson STREET GLIDE SPECIAL 114 - 2022 - R$ 124.990,00",
	}
	for _, l := range listings {
		if l.Title == "" {
			t.Fatalf("%s has no title: the dashboard renders an invisible link", l.ExternalID)
		}
		if got := want[l.ExternalID]; got != "" && l.Title != got {
			t.Errorf("%s: Title = %q, want %q", l.ExternalID, l.Title, got)
		}
	}
}

func TestInstagramTitle(t *testing.T) {
	long := "Harley Davidson Street Glide Special 2015 impecável com todos os acessórios originais e revisões feitas na concessionária"
	cases := []struct {
		name      string
		caption   string
		shortcode string
		want      string
	}{
		{"first line", "Alerta de pão quente. \n🏍️ Street Glide\n📅ANO: 2020/20", "ABC", "Alerta de pão quente."},
		{"teaser line borrows the next", "Confira:\n🛵 HD Street Glide 📌 2014/2014", "ABC", "Confira: 🛵 HD Street Glide 📌 2014/2014"},
		{"skips leading blank lines", "\n\n💰R$ 107.000,00\nStreet Glide", "ABC", "💰R$ 107.000,00 Street Glide"},
		{"short line kept whole", "Road Glide Special", "ABC", "Road Glide Special"},
		{"skips a divider line", "✅ DISPONÍVEL ✅️\n—————————————————\n🇺🇸 HARLEY ULTRA LIMITED 2013", "ABC", "✅ DISPONÍVEL ✅️ 🇺🇸 HARLEY ULTRA LIMITED 2013"},
		{"long line cut at a word", long, "ABC", "Harley Davidson Street Glide Special 2015 impecável com todos os acessórios…"},
		{"empty caption falls back", "", "DPxNH-zEZ-G", "Instagram DPxNH-zEZ-G"},
		{"blank caption falls back", "\n   \n", "DPxNH-zEZ-G", "Instagram DPxNH-zEZ-G"},
	}
	for _, c := range cases {
		got := instagramTitle(c.caption, c.shortcode)
		if got != c.want {
			t.Errorf("%s: instagramTitle(...) = %q, want %q", c.name, got, c.want)
		}
		if runes := len([]rune(got)); runes > instagramTitleRunes {
			t.Errorf("%s: title has %d runes, want at most %d", c.name, runes, instagramTitleRunes)
		}
	}
}

func TestInstagramSoldCaption(t *testing.T) {
	cases := []struct {
		caption string
		want    bool
	}{
		{"💰 VENDIDA ✅ Aceitamos troca* ✅ Financiamento disponível*", true},
		{"Motocicleta impecável - Revisões em dia R$ vendida", true},
		{"Harley Davidson XL 1200 Sportster Iron - 2020 VENDIDO", true},
		{"Os dois vendidos na mesma semana", true},
		{"✅ 𝐕𝐄𝐍𝐃𝐈𝐃𝐎 Harley Davidson XL 1200 Sportster Iron - 2020", true},
		{"À VENDA! Street Glide 2013, motor TC103", false},
		{"Vendo minha Road Glide 2015", false},
		{"Vende-se Street Glide R$ 70.000", false},
		{"Financiamento disponível em até 48x, R$ 107.000,00", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isSold(c.caption); got != c.want {
			t.Errorf("isSold(%q) = %v, want %v", c.caption, got, c.want)
		}
	}
}

func TestParseInstagramDropsPostsAlreadySold(t *testing.T) {
	page := gridPage(
		postHTML("SOLD", "STREET GLIDE SPECIAL Ano: 2019 KM: 30.700 💰 VENDIDA ✅ Aceitamos troca ✅ Financiamento disponível") +
			postHTML("OPEN", "STREET GLIDE 2015 à venda R$ 70.000,00"))

	listings, err := ParseInstagram(strings.NewReader(page))
	if err != nil {
		t.Fatalf("ParseInstagram: %v", err)
	}
	if got, want := len(listings), 1; got != want {
		t.Fatalf("len(listings) = %d, want %d: a sold post is not a listing", got, want)
	}
	if got, want := listings[0].ExternalID, "OPEN"; got != want {
		t.Errorf("ExternalID = %q, want %q", got, want)
	}
}

func TestInstagramFetchReadsEveryHashtag(t *testing.T) {
	urls := []string{"https://www.instagram.com/explore/tags/streetglide/", "https://www.instagram.com/explore/tags/roadglide/"}
	page := fixtureString(t, "instagram-hashtag.html")
	fetcher := &fakePageFetcher{pages: map[string]string{urls[0]: page, urls[1]: page}}

	instagram := NewInstagram(fetcher, urls)
	instagram.delay = func() time.Duration { return 0 }

	listings, err := instagram.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := len(fetcher.asked), 2; got != want {
		t.Fatalf("asked %d urls, want %d", got, want)
	}
	if got, want := len(listings), 4; got != want {
		t.Errorf("len(listings) = %d, want %d", got, want)
	}
	if got, want := instagram.Name(), model.SourceInstagram; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestInstagramFetchStopsAtTheLoginWall(t *testing.T) {
	urls := []string{"https://www.instagram.com/explore/tags/streetglide/", "https://www.instagram.com/explore/tags/roadglide/"}
	fetcher := &fakePageFetcher{pages: map[string]string{
		urls[0]: fixtureString(t, "instagram-hashtag.html"),
		urls[1]: fixtureString(t, "instagram-login.html"),
	}}

	instagram := NewInstagram(fetcher, urls)
	instagram.delay = func() time.Duration { return 0 }

	listings, err := instagram.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should report the login wall")
	}
	if got, want := len(listings), 2; got != want {
		t.Errorf("len(listings) = %d, want %d: what was read before the wall survives", got, want)
	}
}

func TestInstagramWaitsARandomTimeBetweenHashtags(t *testing.T) {
	instagram := NewInstagram(&fakePageFetcher{}, nil)

	seen := make(map[time.Duration]bool)
	for i := 0; i < 200; i++ {
		d := instagram.delay()
		if d < instagramMinDelay || d > instagramMaxDelay {
			t.Fatalf("delay = %v, want between %v and %v", d, instagramMinDelay, instagramMaxDelay)
		}
		seen[d] = true
	}
	if len(seen) < 10 {
		t.Errorf("%d distinct delays in 200 draws, want a random spread", len(seen))
	}
}

func gridPage(inner string) string {
	return `<html><body><main role="main">` + inner + `</main></body></html>`
}

func postHTML(shortcode, caption string) string {
	return fmt.Sprintf(`<a href="/p/%s/"><img alt="%s" src="https://cdn/%s.jpg"></a>`, shortcode, caption, shortcode)
}
