package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func seededStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	year := 2015
	cents := int64(7200000)
	matched := model.Listing{
		Source: "olx", ExternalID: "m1", URL: "https://example.com/m1",
		Title: "Harley Street Glide Special", Bike: model.BikeStreetGlide,
		Year: &year, PriceCents: &cents, City: "curitiba", State: "PR",
		Verdict: model.VerdictMatch,
	}
	if _, err := s.Upsert(matched, time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	maybeCents := int64(8100000)
	maybe := matched
	maybe.ExternalID = "m2"
	maybe.Title = "Harley Road Glide"
	maybe.PriceCents = &maybeCents
	maybe.Verdict = model.VerdictMaybe
	if _, err := s.Upsert(maybe, time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	return s
}

func emptyStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func upsert(t *testing.T, s *store.Store, l model.Listing, at time.Time) int64 {
	t.Helper()
	res, err := s.Upsert(l, at)
	if err != nil {
		t.Fatalf("Upsert %s: %v", l.ExternalID, err)
	}
	return res.ID
}

func get(t *testing.T, srv http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func useSaoPauloZone(t *testing.T) {
	t.Helper()
	t.Setenv("TZ", "America/Sao_Paulo")
	zone, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	previous := time.Local
	time.Local = zone
	t.Cleanup(func() { time.Local = previous })
}

var (
	metaPattern      = regexp.MustCompile(`(?s)<div class="meta">(.*?)</div>`)
	spacePattern     = regexp.MustCompile(`\s+`)
	tagPattern       = regexp.MustCompile(`<[^>]+>`)
	healthRowPattern = regexp.MustCompile(`(?s)<td>(\w+)</td>\s*<td[^>]*>(\w+)</td>\s*<td[^>]*>(.*?)</td>\s*<td[^>]*>(.*?)</td>`)
	stripPattern     = regexp.MustCompile(`(?s)<span class="luz luz-(\w+)"[^>]*></span>\s*<span class="luz-fonte">(.*?)</span>\s*<span class="luz-quando">(.*?)</span>`)
	imgPattern       = regexp.MustCompile(`<img[^>]*>`)
)

func flatten(s string) string {
	return strings.TrimSpace(spacePattern.ReplaceAllString(tagPattern.ReplaceAllString(s, ""), " "))
}

func metaLines(body string) []string {
	var lines []string
	for _, m := range metaPattern.FindAllStringSubmatch(body, -1) {
		lines = append(lines, flatten(m[1]))
	}
	return lines
}

type healthCell struct {
	status string
	last   string
	counts string
}

func healthRow(t *testing.T, body, source string) healthCell {
	t.Helper()
	for _, m := range healthRowPattern.FindAllStringSubmatch(body, -1) {
		if m[1] == source {
			return healthCell{status: m[2], last: flatten(m[3]), counts: flatten(m[4])}
		}
	}
	t.Fatalf("health page has no row for source %q, body:\n%s", source, body)
	return healthCell{}
}

type stripLight struct {
	status string
	name   string
	when   string
}

func stripLights(t *testing.T, body string) []stripLight {
	t.Helper()
	var lights []stripLight
	for _, m := range stripPattern.FindAllStringSubmatch(body, -1) {
		lights = append(lights, stripLight{status: m[1], name: flatten(m[2]), when: flatten(m[3])})
	}
	return lights
}

func TestRoutesRenderExpectedListings(t *testing.T) {
	srv := NewServer(seededStore(t), []string{"olx"})

	cases := []struct {
		path        string
		wantPresent string
		wantAbsent  string
	}{
		{"/", "Street Glide Special", "Road Glide"},
		{"/maybe", "Road Glide", "Street Glide Special"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, c.wantPresent) {
				t.Errorf("%s should contain %q", c.path, c.wantPresent)
			}
			if strings.Contains(body, c.wantAbsent) {
				t.Errorf("%s should not contain %q", c.path, c.wantAbsent)
			}
		})
	}
}

func TestHealthRouteRenders(t *testing.T) {
	srv := NewServer(seededStore(t), []string{"olx"})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "olx") {
		t.Error("health page should list the configured source")
	}
}

func TestSetUserStateUpdatesRow(t *testing.T) {
	s := seededStore(t)
	srv := NewServer(s, []string{"olx"})

	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil || len(rows) == 0 {
		t.Fatalf("seed failed: %v", err)
	}
	id := rows[0].ID

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/listing/"+strconv.FormatInt(id, 10)+"/state?value=contacted", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	row, _, err := s.GetRow(id)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.UserState != "contacted" {
		t.Errorf("UserState = %q, want contacted", row.UserState)
	}
}

func TestRejectedRouteRenders(t *testing.T) {
	s := emptyStore(t)
	cents := int64(1500000)
	upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "r1", URL: "https://example.com/r1",
		Title: "Harley Sportster 883", PriceCents: &cents, City: "curitiba", State: "PR",
		Verdict: model.VerdictReject,
	}, time.Now())

	code, body := get(t, NewServer(s, []string{"olx"}), "/rejected")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "Sportster 883") {
		t.Error("/rejected should contain the rejected listing")
	}
}

func TestListGroupsRegionsInPriorityOrder(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	seed := []struct {
		id    string
		title string
		city  string
		state string
		age   time.Duration
	}{
		{"a", "Glide Curitiba", "curitiba", "PR", 4 * time.Hour},
		{"b", "Glide Sao Paulo", "sao paulo", "SP", 3 * time.Hour},
		{"c", "Glide Niteroi", "niteroi", "RJ", 2 * time.Hour},
		{"d", "Glide Brasilia", "brasilia", "DF", 1 * time.Hour},
		{"e", "Glide Rio", "rio de janeiro", "RJ", 0},
	}
	for _, l := range seed {
		upsert(t, s, model.Listing{
			Source: "olx", ExternalID: l.id, URL: "https://example.com/" + l.id,
			Title: l.title, City: l.city, State: l.state, Verdict: model.VerdictMatch,
		}, base.Add(-l.age))
	}

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	want := []string{
		"Rio de Janeiro", "Glide Rio", "Glide Niteroi",
		"São Paulo", "Glide Sao Paulo",
		"Curitiba", "Glide Curitiba",
		"Outras regiões", "Glide Brasilia",
	}
	at := -1
	for _, marker := range want {
		i := strings.Index(body, marker)
		if i < 0 {
			t.Fatalf("list should contain %q", marker)
		}
		if i < at {
			t.Errorf("%q rendered out of order, want %v", marker, want)
		}
		at = i
	}
}

func TestRegionWithoutRowsStillShowsItIsWatched(t *testing.T) {
	s := emptyStore(t)
	cents := int64(21000000)
	for i := range 3 {
		id := "rej" + strconv.Itoa(i)
		upsert(t, s, model.Listing{
			Source: "olx", ExternalID: id, URL: "https://example.com/" + id,
			Title: "Road Glide 2026", PriceCents: &cents, City: "rio de janeiro", State: "RJ",
			Verdict: model.VerdictReject,
		}, time.Now())
	}
	upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "match", URL: "https://example.com/match",
		Title: "Street Glide 2014", PriceCents: &cents, City: "curitiba", State: "PR",
		Verdict: model.VerdictMatch,
	}, time.Now())

	code, body := get(t, NewServer(s, []string{"olx"}), "/")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "Rio de Janeiro") {
		t.Error("a region without matches should still show its header")
	}
	if !strings.Contains(body, "Nenhum anúncio no alvo — monitorando a cada rodada") {
		t.Errorf("an empty region should say it is being watched, body:\n%s", body)
	}
	if !strings.Contains(body, "3 anúncios avaliados") {
		t.Error("an empty region should show how many listings it has scanned")
	}
	if !strings.Contains(body, "1 anúncio avaliado") {
		t.Error("the scanned count should be singular for a single listing")
	}
}

func TestOtherRegionsGroupOnlyAppearsWhenUsed(t *testing.T) {
	s := emptyStore(t)
	upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "sp", URL: "https://example.com/sp",
		Title: "Street Glide", City: "sao paulo", State: "SP", Verdict: model.VerdictMatch,
	}, time.Now())

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	if strings.Contains(body, "Outras regiões") {
		t.Error("the fallback region should stay hidden when every listing is in a watched region")
	}
}

func TestEmptyListShowsEveryWatchedRegion(t *testing.T) {
	code, body := get(t, NewServer(emptyStore(t), []string{"olx"}), "/")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	for _, region := range []string{"Rio de Janeiro", "São Paulo", "Curitiba"} {
		if !strings.Contains(body, region) {
			t.Errorf("an empty board should still show %q", region)
		}
	}
	if !strings.Contains(body, "nenhum anúncio avaliado") {
		t.Errorf("a region with nothing scanned should say so, body:\n%s", body)
	}
}

func TestListRendersRowWithEveryOptionalFieldAbsent(t *testing.T) {
	s := emptyStore(t)
	upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "bare", URL: "https://example.com/bare",
		Title: "Stret glide excelente estado", City: "curitiba", Verdict: model.VerdictMatch,
	}, time.Now())

	code, body := get(t, NewServer(s, []string{"olx"}), "/")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "preço não informado") {
		t.Error("a row without a price should say so")
	}
	if strings.Contains(body, "<img") {
		t.Error("a row without an image should not render an img tag")
	}
	if !strings.Contains(body, `class="foto"`) {
		t.Error("a row without an image should still render the photo well")
	}
	if !strings.Contains(body, "sem foto") {
		t.Error("a row without an image should say the photo is missing")
	}
	if strings.Contains(body, "curitiba/") {
		t.Error("a city without a state should not render a trailing slash")
	}
	lines := metaLines(body)
	if len(lines) == 0 {
		t.Fatal("list should render a meta line")
	}
	if want := "curitiba · olx"; lines[0] != want {
		t.Errorf("meta line = %q, want %q", lines[0], want)
	}
}

func TestListRendersStateWithoutCity(t *testing.T) {
	s := emptyStore(t)
	upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "nocity", URL: "https://example.com/nocity",
		Title: "Road Glide", State: "RJ", Verdict: model.VerdictMatch,
	}, time.Now())

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	lines := metaLines(body)
	if len(lines) == 0 {
		t.Fatal("list should render a meta line")
	}
	if want := "RJ · olx"; lines[0] != want {
		t.Errorf("meta line = %q, want %q", lines[0], want)
	}
}

func TestListFormatsKilometresAsNumerals(t *testing.T) {
	s := emptyStore(t)
	km := 90195
	upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "km", URL: "https://example.com/km",
		Title: "Street Glide", Km: &km, City: "curitiba", State: "PR", Verdict: model.VerdictMatch,
	}, time.Now())

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	lines := metaLines(body)
	if len(lines) == 0 {
		t.Fatal("list should render a meta line")
	}
	if want := "90.195 km · curitiba/PR · olx"; lines[0] != want {
		t.Errorf("meta line = %q, want %q", lines[0], want)
	}
}

func TestTriagedRowShowsItsStateAndLosesTheNewMarker(t *testing.T) {
	s := emptyStore(t)
	id := upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "triaged", URL: "https://example.com/triaged",
		Title: "Street Glide", City: "curitiba", State: "PR", Verdict: model.VerdictMatch,
	}, time.Now())

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	if !strings.Contains(body, `class="novo"`) {
		t.Error("an untriaged listing should carry the new marker")
	}

	if err := s.SetUserState(id, "contacted"); err != nil {
		t.Fatalf("SetUserState: %v", err)
	}
	_, body = get(t, NewServer(s, []string{"olx"}), "/")
	if strings.Contains(body, `class="novo"`) {
		t.Error("a triaged listing should lose the new marker")
	}
	if want := "curitiba/PR · olx · contatado"; metaLines(body)[0] != want {
		t.Errorf("meta line = %q, want %q", metaLines(body)[0], want)
	}
}

func TestListingImagesSurviveHotlinkProtection(t *testing.T) {
	s := emptyStore(t)
	upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "img", URL: "https://example.com/img",
		Title: "Street Glide", ImageURL: "https://img.olx.com.br/images/38/387646197932805.jpg",
		City: "curitiba", State: "PR", Verdict: model.VerdictMatch,
	}, time.Now())
	srv := NewServer(s, []string{"olx"})

	for _, path := range []string{"/", "/listing/1"} {
		_, body := get(t, srv, path)
		tags := imgPattern.FindAllString(body, -1)
		if len(tags) == 0 {
			t.Fatalf("%s should render the listing image", path)
		}
		for _, tag := range tags {
			if !strings.Contains(tag, `referrerpolicy="no-referrer"`) {
				t.Errorf("%s: image without a referrer policy is blocked by the host: %s", path, tag)
			}
			if !strings.Contains(tag, `loading="lazy"`) {
				t.Errorf("%s: image should load lazily: %s", path, tag)
			}
		}
	}
}

func TestPriceDropTickerOnlyAfterARealDrop(t *testing.T) {
	useSaoPauloZone(t)
	s := emptyStore(t)
	first := int64(7500000)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	steady := model.Listing{
		Source: "olx", ExternalID: "steady", URL: "https://example.com/steady",
		Title: "Glide Sem Queda", PriceCents: &first, City: "curitiba", State: "PR",
		Verdict: model.VerdictMatch,
	}
	upsert(t, s, steady, base)

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	if strings.Contains(body, "▼") {
		t.Error("a listing seen at one price should not show a price drop")
	}

	lower := int64(7000000)
	dropped := steady
	dropped.ExternalID = "dropped"
	dropped.Title = "Glide Com Queda"
	upsert(t, s, dropped, base)
	dropped.PriceCents = &lower
	upsert(t, s, dropped, base.Add(24*time.Hour))

	_, body = get(t, NewServer(s, []string{"olx"}), "/")
	if want := "▼ R$ 5.000 desde 01/08"; !strings.Contains(body, want) {
		t.Errorf("a listing whose price fell should show %q, body:\n%s", want, body)
	}
}

func TestDetailShowsPriceHistoryInLocalTime(t *testing.T) {
	useSaoPauloZone(t)
	s := emptyStore(t)
	cents := int64(7200000)
	observed := time.Date(2026, 8, 9, 21, 27, 0, 0, time.UTC)
	id := upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "d1", URL: "https://example.com/d1",
		Title: "Harley Street Glide Special", PriceCents: &cents, City: "curitiba", State: "PR",
		Verdict: model.VerdictMatch,
	}, observed)

	code, body := get(t, NewServer(s, []string{"olx"}), "/listing/"+strconv.FormatInt(id, 10))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "72.000") {
		t.Error("detail should show the price")
	}
	if want := "09/08/2026 18:27"; !strings.Contains(body, want) {
		t.Errorf("price history should show %q, body:\n%s", want, body)
	}
	if strings.Contains(body, "09/08/2026 21:27") {
		t.Error("price history should not show the UTC timestamp")
	}
}

func TestDetailWithoutPriceHistoryShowsEmptyState(t *testing.T) {
	s := emptyStore(t)
	id := upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "noprice", URL: "https://example.com/noprice",
		Title: "Street Glide", City: "curitiba", State: "PR", Verdict: model.VerdictMatch,
	}, time.Now())

	code, body := get(t, NewServer(s, []string{"olx"}), "/listing/"+strconv.FormatInt(id, 10))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "Nenhum preço registrado.") {
		t.Errorf("a listing without price observations should say so, body:\n%s", body)
	}
	if strings.Contains(body, "<table") {
		t.Error("a listing without price observations should not render an empty table")
	}
}

func TestDetailOfMissingListingIsNotFound(t *testing.T) {
	code, body := get(t, NewServer(emptyStore(t), []string{"olx"}), "/listing/999")
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
	if got := strings.TrimSpace(body); got != "Anúncio não encontrado" {
		t.Errorf("body = %q, want %q", got, "Anúncio não encontrado")
	}
}

func TestDetailWithUnparsableIDIsBadRequest(t *testing.T) {
	code, body := get(t, NewServer(emptyStore(t), []string{"olx"}), "/listing/abc")
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if got := strings.TrimSpace(body); got != "Valor inválido" {
		t.Errorf("body = %q, want %q", got, "Valor inválido")
	}
}

func TestSetUserStateRejectsUnknownValue(t *testing.T) {
	s := seededStore(t)
	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil || len(rows) == 0 {
		t.Fatalf("seed failed: %v", err)
	}

	rec := httptest.NewRecorder()
	rec2 := httptest.NewRecorder()
	srv := NewServer(s, []string{"olx"})
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/listing/"+strconv.FormatInt(rows[0].ID, 10)+"/state?value=sold", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "Valor inválido" {
		t.Errorf("body = %q, want %q", got, "Valor inválido")
	}

	srv.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/listing/999/state?value=contacted", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("status for unknown listing = %d, want 404", rec2.Code)
	}
	if got := strings.TrimSpace(rec2.Body.String()); got != "Anúncio não encontrado" {
		t.Errorf("body = %q, want %q", got, "Anúncio não encontrado")
	}
}

func TestHealthReportsSourceWithoutRuns(t *testing.T) {
	code, body := get(t, NewServer(emptyStore(t), []string{"olx"}), "/health")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	row := healthRow(t, body, "olx")
	if row.status != "unknown" {
		t.Errorf("status = %q, want unknown", row.status)
	}
	if row.counts != "sem coletas" {
		t.Errorf("counts = %q, want \"sem coletas\"", row.counts)
	}
	if row.last != "nunca" {
		t.Errorf("last collection = %q, want \"nunca\"", row.last)
	}
}

func TestHealthLooksBeyondTheSuspectWindow(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for i := range 10 {
		at := base.Add(time.Duration(i) * time.Hour)
		if err := s.RecordRun("olx", at, at, 268, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	for i := range 5 {
		at := base.Add(time.Duration(20+i) * time.Hour)
		if err := s.RecordRun("olx", at, at, 0, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}

	_, body := get(t, NewServer(s, []string{"olx"}), "/health")
	row := healthRow(t, body, "olx")
	if row.status != "suspect" {
		t.Errorf("status = %q, want suspect: a source empty for five runs after a productive history is broken", row.status)
	}
	if !strings.HasPrefix(row.counts, "0, 0, 0, 0, 0, 268") {
		t.Errorf("counts = %q, want the empty runs followed by the productive history", row.counts)
	}
}

func TestHealthStripAppearsOnEveryPageWithLastCollection(t *testing.T) {
	s := seededStore(t)
	at := time.Now().Add(-2*time.Hour - 10*time.Minute)
	if err := s.RecordRun("olx", at, at, 268, "ok", ""); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	srv := NewServer(s, []string{"olx", "mercadolivre"})

	for _, path := range []string{"/", "/maybe", "/rejected", "/health"} {
		_, body := get(t, srv, path)
		lights := stripLights(t, body)
		if len(lights) != 2 {
			t.Fatalf("%s: strip has %d lights, want 2: %v", path, len(lights), lights)
		}
		if lights[0].name != "olx" || lights[0].status != "ok" {
			t.Errorf("%s: first light = %+v, want olx/ok", path, lights[0])
		}
		if lights[0].when != "há 2h" {
			t.Errorf("%s: olx collected 2h10 ago, strip says %q", path, lights[0].when)
		}
		if lights[1].name != "mercadolivre" || lights[1].status != "unknown" {
			t.Errorf("%s: second light = %+v, want mercadolivre/unknown", path, lights[1])
		}
		if lights[1].when != "sem coletas" {
			t.Errorf("%s: a source that never ran says %q", path, lights[1].when)
		}
	}
}

func TestHumanSince(t *testing.T) {
	cases := []struct {
		elapsed time.Duration
		want    string
	}{
		{20 * time.Second, "agora"},
		{5 * time.Minute, "há 5min"},
		{59 * time.Minute, "há 59min"},
		{90 * time.Minute, "há 1h"},
		{47 * time.Hour, "há 47h"},
		{50 * time.Hour, "há 2d"},
	}
	for _, c := range cases {
		if got := humanSince(c.elapsed); got != c.want {
			t.Errorf("humanSince(%s) = %q, want %q", c.elapsed, got, c.want)
		}
	}
}

func TestActiveTabIsMarked(t *testing.T) {
	srv := NewServer(seededStore(t), []string{"olx"})
	cases := map[string]string{"/": "Match", "/maybe": "Talvez", "/rejected": "Descartados", "/health": "Saúde"}
	for path, label := range cases {
		_, body := get(t, srv, path)
		pattern := regexp.MustCompile(`<a class="aba ativa"[^>]*>` + label + `</a>`)
		if !pattern.MatchString(body) {
			t.Errorf("%s should mark %q as the active tab", path, label)
		}
		if strings.Count(body, `class="aba ativa"`) != 1 {
			t.Errorf("%s should mark exactly one active tab", path)
		}
	}
}
