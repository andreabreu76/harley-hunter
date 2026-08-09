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

var (
	metaPattern      = regexp.MustCompile(`(?s)<div class="meta">(.*?)</div>`)
	spacePattern     = regexp.MustCompile(`\s+`)
	healthRowPattern = regexp.MustCompile(`(?s)<td>(\w+)</td>\s*<td[^>]*>(\w+)</td>\s*<td>(.*?)</td>`)
)

type healthCell struct {
	status string
	counts string
}

func healthRow(t *testing.T, body, source string) healthCell {
	t.Helper()
	for _, m := range healthRowPattern.FindAllStringSubmatch(body, -1) {
		if m[1] == source {
			return healthCell{status: m[2], counts: strings.TrimSpace(m[3])}
		}
	}
	t.Fatalf("health page has no row for source %q, body:\n%s", source, body)
	return healthCell{}
}

func metaLines(body string) []string {
	var lines []string
	for _, m := range metaPattern.FindAllStringSubmatch(body, -1) {
		lines = append(lines, strings.TrimSpace(spacePattern.ReplaceAllString(m[1], " ")))
	}
	return lines
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

func TestEmptyListShowsPlaceholder(t *testing.T) {
	code, body := get(t, NewServer(emptyStore(t), []string{"olx"}), "/")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !strings.Contains(body, "Nenhum anúncio nesta categoria.") {
		t.Error("an empty list should tell the reader the category is empty")
	}
}

func TestListPutsRioFirstAndKeepsOrderInsideRegion(t *testing.T) {
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
	want := []string{"Glide Rio", "Glide Niteroi", "Glide Sao Paulo", "Glide Curitiba", "Glide Brasilia"}
	at := -1
	for _, title := range want {
		i := strings.Index(body, title)
		if i < 0 {
			t.Fatalf("list should contain %q", title)
		}
		if i < at {
			t.Errorf("%q rendered out of order, want %v", title, want)
		}
		at = i
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
	if strings.Contains(body, "curitiba/") {
		t.Error("a city without a state should not render a trailing slash")
	}
	lines := metaLines(body)
	if len(lines) == 0 {
		t.Fatal("list should render a meta line")
	}
	if want := "curitiba · olx · new"; lines[0] != want {
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
	if want := "RJ · olx · new"; lines[0] != want {
		t.Errorf("meta line = %q, want %q", lines[0], want)
	}
}

func TestPriceDropBadgeOnlyAfterARealDrop(t *testing.T) {
	s := emptyStore(t)
	first := int64(7500000)
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	steady := model.Listing{
		Source: "olx", ExternalID: "steady", URL: "https://example.com/steady",
		Title: "Glide Sem Queda", PriceCents: &first, City: "curitiba", State: "PR",
		Verdict: model.VerdictMatch,
	}
	upsert(t, s, steady, base)

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	if strings.Contains(body, "baixou") {
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
	if !strings.Contains(body, "baixou R$ 5.000") {
		t.Errorf("a listing whose price fell should show the drop, body:\n%s", body)
	}
}

func TestDetailShowsPriceHistoryInLocalTime(t *testing.T) {
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
	if !strings.Contains(body, "R$ 72.000") {
		t.Error("detail should show the price")
	}
	local := observed.In(time.Local).Format("02/01/2006 15:04")
	if !strings.Contains(body, local) {
		t.Errorf("price history should show %q, body:\n%s", local, body)
	}
	utc := observed.Format("02/01/2006 15:04")
	if utc != local && strings.Contains(body, utc) {
		t.Errorf("price history should not show the UTC timestamp %q", utc)
	}
}

func TestDetailOfMissingListingIsNotFound(t *testing.T) {
	code, _ := get(t, NewServer(emptyStore(t), []string{"olx"}), "/listing/999")
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func TestDetailWithUnparsableIDIsBadRequest(t *testing.T) {
	code, _ := get(t, NewServer(emptyStore(t), []string{"olx"}), "/listing/abc")
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
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

	srv.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/listing/999/state?value=contacted", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("status for unknown listing = %d, want 404", rec2.Code)
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
