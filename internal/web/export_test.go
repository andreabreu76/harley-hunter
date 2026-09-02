package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/crawl"
	"github.com/andreabreu76/harley-hunter/internal/fipe"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func exportListingOf(id string, verdict model.Verdict, cents int64) model.Listing {
	year := 2015
	km := 31000
	return model.Listing{
		Source: model.SourceOLX, ExternalID: id, URL: "https://example.com/" + id,
		Title: "Harley Street Glide " + id, Bike: model.BikeSportster1200,
		Variant: model.VariantBase, Year: &year, PriceCents: &cents, Km: &km,
		City: "curitiba", State: "PR", Verdict: verdict,
	}
}

func exportStore(t *testing.T) *store.Store {
	t.Helper()
	s := emptyStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	upsert(t, s, exportListingOf("e1", model.VerdictMatch, 7200000), at)
	upsert(t, s, exportListingOf("e2", model.VerdictMaybe, 8100000), at)
	upsert(t, s, exportListingOf("e3", model.VerdictReject, 9900000), at)
	return s
}

type decodedExport struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Counts      map[string]int `json:"counts"`
	Sources     []struct {
		Name         string     `json:"name"`
		Status       string     `json:"status"`
		LastRunAt    *time.Time `json:"last_run_at"`
		RecentCounts []int      `json:"recent_counts"`
	} `json:"sources"`
	Fipe []struct {
		Label      string `json:"label"`
		Year       int    `json:"year"`
		PriceCents int64  `json:"price_cents"`
		Month      string `json:"month"`
	} `json:"fipe"`
	Listings []struct {
		ID         int64   `json:"id"`
		ExternalID string  `json:"external_id"`
		Verdict    string  `json:"verdict"`
		Status     string  `json:"status"`
		PriceCents *int64  `json:"price_cents"`
		Km         *int    `json:"km"`
		Phone      *string `json:"phone"`
		Fresh      bool    `json:"fresh"`

		PriceDropCents *int64 `json:"price_drop_cents"`
		PriceHistory   []struct {
			PriceCents int64     `json:"price_cents"`
			At         time.Time `json:"at"`
		} `json:"price_history"`

		Fipe *struct {
			Label       string   `json:"label"`
			Year        int      `json:"year"`
			PriceCents  int64    `json:"price_cents"`
			GapPercent  *float64 `json:"gap_percent"`
			BelowFipe   *bool    `json:"below_fipe"`
			BaseVariant bool     `json:"base_variant"`
		} `json:"fipe"`

		Reposts []struct {
			ID          int64     `json:"id"`
			Source      string    `json:"source"`
			Verdict     string    `json:"verdict"`
			PriceCents  *int64    `json:"price_cents"`
			Km          *int      `json:"km"`
			FirstSeenAt time.Time `json:"first_seen_at"`
		} `json:"reposts"`
	} `json:"listings"`
}

func exportOf(t *testing.T, srv http.Handler, path string) decodedExport {
	t.Helper()
	code, body := get(t, srv, path)
	if code != http.StatusOK {
		t.Fatalf("%s: status %d, body %s", path, code, body)
	}
	var decoded decodedExport
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("%s is not valid json: %v\n%s", path, err, body)
	}
	return decoded
}

func verdictsIn(decoded decodedExport) []string {
	found := make([]string, 0, len(decoded.Listings))
	for _, l := range decoded.Listings {
		found = append(found, l.Verdict)
	}
	return found
}

func recorderFor(t *testing.T, srv http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestExportDefaultsToMatchAndMaybe(t *testing.T) {
	srv := NewServer(exportStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Listings) != 2 {
		t.Fatalf("expected match and maybe, got %v", verdictsIn(decoded))
	}
	for _, verdict := range verdictsIn(decoded) {
		if verdict == "reject" {
			t.Errorf("the default export should leave rejects out, got %v", verdictsIn(decoded))
		}
	}
}

func TestExportNarrowsToASingleVerdict(t *testing.T) {
	srv := NewServer(exportStore(t), fixed(model.SourceOLX))

	for _, wanted := range []string{"match", "maybe"} {
		decoded := exportOf(t, srv, "/export.json?verdict="+wanted)
		if len(decoded.Listings) != 1 || decoded.Listings[0].Verdict != wanted {
			t.Errorf("verdict=%s returned %v", wanted, verdictsIn(decoded))
		}
	}
}

func TestExportAllBringsTheRejects(t *testing.T) {
	srv := NewServer(exportStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json?verdict=all")

	if len(decoded.Listings) != 3 {
		t.Fatalf("expected the three verdicts, got %v", verdictsIn(decoded))
	}
}

func TestExportRejectsAnUnknownVerdict(t *testing.T) {
	srv := NewServer(exportStore(t), fixed(model.SourceOLX))

	code, _ := get(t, srv, "/export.json?verdict=lixo")

	if code != http.StatusBadRequest {
		t.Errorf("expected 400 for an unknown verdict, got %d", code)
	}
}

func TestExportCountsTheWholeDatabaseNotTheSlice(t *testing.T) {
	srv := NewServer(exportStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json?verdict=match")

	if len(decoded.Listings) != 1 {
		t.Fatalf("expected a single listing, got %d", len(decoded.Listings))
	}
	for verdict, want := range map[string]int{"match": 1, "maybe": 1, "reject": 1} {
		if decoded.Counts[verdict] != want {
			t.Errorf("counts[%s]: expected %d, got %d", verdict, want, decoded.Counts[verdict])
		}
	}
}

func TestExportKeepsTheClosedListing(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	upsert(t, s, exportListingOf("e4", model.VerdictMatch, 7200000), base)
	expireEverythingUnseen(t, s, base)
	srv := NewServer(s, fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Listings) != 1 {
		t.Fatalf("a closed listing still belongs in the export, got %d", len(decoded.Listings))
	}
	if decoded.Listings[0].Status != "gone" {
		t.Errorf("the closed listing should carry its status, got %q", decoded.Listings[0].Status)
	}
}

func TestExportServesJSONContentType(t *testing.T) {
	srv := NewServer(exportStore(t), fixed(model.SourceOLX))

	rec := recorderFor(t, srv, "/export.json")

	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("unexpected content type %q", got)
	}
}

func TestExportOfAnEmptyDatabaseIsAnEmptyList(t *testing.T) {
	srv := NewServer(emptyStore(t), fixed(model.SourceOLX))

	code, body := get(t, srv, "/export.json")

	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if !strings.Contains(body, `"listings": []`) {
		t.Errorf("an empty database should export an empty array, got %s", body)
	}
	for _, verdict := range []string{"match", "maybe", "reject"} {
		if !strings.Contains(body, `"`+verdict+`": 0`) {
			t.Errorf("counts should carry %s at zero, got %s", verdict, body)
		}
	}
}

func TestExportKeepsAbsentValuesNull(t *testing.T) {
	s := emptyStore(t)
	bare := exportListingOf("e9", model.VerdictMatch, 0)
	bare.PriceCents = nil
	bare.Km = nil
	upsert(t, s, bare, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	srv := NewServer(s, fixed(model.SourceOLX))

	_, body := get(t, srv, "/export.json")

	for _, field := range []string{"price_cents", "km", "phone", "published_at"} {
		if !strings.Contains(body, `"`+field+`": null`) {
			t.Errorf("%s should be null when absent, got %s", field, body)
		}
	}
}

func TestExportDatesAreUTCWhateverTheLocalZone(t *testing.T) {
	useSaoPauloZone(t)
	srv := NewServer(exportStore(t), fixed(model.SourceOLX))

	_, body := get(t, srv, "/export.json")

	if !strings.Contains(body, `"first_seen_at": "2026-08-01T12:00:00Z"`) {
		t.Errorf("dates should be exported in UTC, got %s", body)
	}
}

func TestExportDoesNotEscapeURLs(t *testing.T) {
	s := emptyStore(t)
	tracked := exportListingOf("e8", model.VerdictMatch, 7200000)
	tracked.URL = "https://example.com/moto?ref=busca&pos=2"
	upsert(t, s, tracked, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	srv := NewServer(s, fixed(model.SourceOLX))

	_, body := get(t, srv, "/export.json")

	if !strings.Contains(body, "https://example.com/moto?ref=busca&pos=2") {
		t.Errorf("the url should survive unescaped, got %s", body)
	}
}

func droppedStore(t *testing.T) *store.Store {
	t.Helper()
	s := emptyStore(t)
	at := time.Date(2026, 7, 2, 11, 4, 0, 0, time.UTC)

	tracked := exportListingOf("d1", model.VerdictMatch, 7500000)
	upsert(t, s, tracked, at)
	dropped := int64(7100000)
	tracked.PriceCents = &dropped
	upsert(t, s, tracked, at.Add(48*time.Hour))

	upsert(t, s, exportListingOf("d2", model.VerdictMatch, 6900000), at)
	return s
}

func listingByExternalID(t *testing.T, decoded decodedExport, id string) int {
	t.Helper()
	for i, l := range decoded.Listings {
		if l.ExternalID == id {
			return i
		}
	}
	t.Fatalf("listing %s not found in the export", id)
	return -1
}

func TestExportCarriesTheWholePriceHistory(t *testing.T) {
	srv := NewServer(droppedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")
	tracked := decoded.Listings[listingByExternalID(t, decoded, "d1")]

	if len(tracked.PriceHistory) != 2 {
		t.Fatalf("expected 2 price points, got %d", len(tracked.PriceHistory))
	}
	if tracked.PriceHistory[0].PriceCents != 7500000 {
		t.Errorf("first point should be the original price, got %d",
			tracked.PriceHistory[0].PriceCents)
	}
	if tracked.PriceHistory[1].PriceCents != 7100000 {
		t.Errorf("second point should be the drop, got %d",
			tracked.PriceHistory[1].PriceCents)
	}
	if !tracked.PriceHistory[0].At.Before(tracked.PriceHistory[1].At) {
		t.Error("price points should come in observation order")
	}
}

func TestExportReportsThePriceDrop(t *testing.T) {
	srv := NewServer(droppedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")
	tracked := decoded.Listings[listingByExternalID(t, decoded, "d1")]

	if tracked.PriceDropCents == nil {
		t.Fatal("a listing that dropped should report the drop")
	}
	if *tracked.PriceDropCents != 400000 {
		t.Errorf("expected a drop of 400000 cents, got %d", *tracked.PriceDropCents)
	}
}

func TestExportLeavesTheDropNullWhenThePriceHeld(t *testing.T) {
	srv := NewServer(droppedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")
	steady := decoded.Listings[listingByExternalID(t, decoded, "d2")]

	if steady.PriceDropCents != nil {
		t.Errorf("a steady price should not report a drop, got %d", *steady.PriceDropCents)
	}
}

func TestExportGivesAnEmptyHistoryToAPricelessListing(t *testing.T) {
	s := emptyStore(t)
	bare := exportListingOf("d3", model.VerdictMatch, 0)
	bare.PriceCents = nil
	upsert(t, s, bare, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	srv := NewServer(s, fixed(model.SourceOLX))

	_, body := get(t, srv, "/export.json")

	if !strings.Contains(body, `"price_history": []`) {
		t.Errorf("a listing without history should export an empty array, got %s", body)
	}
}

func fipedStore(t *testing.T) *store.Store {
	t.Helper()
	s := emptyStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	reference := fipe.Reference{
		Code: "810055-1", Label: "FLHXS STREET GLIDE SPECIAL",
		Bike: model.BikeSportster1200, Variant: model.VariantBase,
		Year: 2015, PriceCents: 8000000, Month: "agosto de 2026",
	}
	if err := s.SaveFipeReference(reference, at); err != nil {
		t.Fatalf("SaveFipeReference: %v", err)
	}

	upsert(t, s, exportListingOf("f1", model.VerdictMatch, 7200000), at)
	upsert(t, s, exportListingOf("f2", model.VerdictMatch, 8800000), at)

	unmatched := exportListingOf("f3", model.VerdictMatch, 7200000)
	unmatched.Bike = model.BikeSportster883
	upsert(t, s, unmatched, at)

	yearless := exportListingOf("f4", model.VerdictMatch, 7200000)
	yearless.Year = nil
	upsert(t, s, yearless, at)

	priceless := exportListingOf("f5", model.VerdictMatch, 0)
	priceless.PriceCents = nil
	upsert(t, s, priceless, at)

	unknownVariant := exportListingOf("f6", model.VerdictMatch, 7200000)
	unknownVariant.Variant = model.VariantUnknown
	upsert(t, s, unknownVariant, at)

	return s
}

func TestExportListsTheFipeTableInTheEnvelope(t *testing.T) {
	srv := NewServer(fipedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Fipe) != 1 {
		t.Fatalf("expected the single stored reference, got %d", len(decoded.Fipe))
	}
	if decoded.Fipe[0].PriceCents != 8000000 || decoded.Fipe[0].Month != "agosto de 2026" {
		t.Errorf("unexpected reference %+v", decoded.Fipe[0])
	}
}

func TestExportMeasuresTheGapBelowFipe(t *testing.T) {
	srv := NewServer(fipedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")
	cheap := decoded.Listings[listingByExternalID(t, decoded, "f1")]

	if cheap.Fipe == nil {
		t.Fatal("a listing that matches the table should carry its reference")
	}
	if cheap.Fipe.GapPercent == nil || *cheap.Fipe.GapPercent != 10 {
		t.Errorf("7200000 against 8000000 is a 10%% gap, got %v", cheap.Fipe.GapPercent)
	}
	if cheap.Fipe.BelowFipe == nil || !*cheap.Fipe.BelowFipe {
		t.Error("7200000 is below a fipe of 8000000")
	}
	if cheap.Fipe.BaseVariant {
		t.Error("a listing with a known variant is not a base match")
	}
}

func TestExportMeasuresTheGapAboveFipe(t *testing.T) {
	srv := NewServer(fipedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")
	pricey := decoded.Listings[listingByExternalID(t, decoded, "f2")]

	if pricey.Fipe == nil || pricey.Fipe.GapPercent == nil {
		t.Fatal("a listing above fipe still carries its gap")
	}
	if *pricey.Fipe.GapPercent != 10 {
		t.Errorf("8800000 against 8000000 is a 10%% gap, got %v", *pricey.Fipe.GapPercent)
	}
	if pricey.Fipe.BelowFipe == nil || *pricey.Fipe.BelowFipe {
		t.Error("8800000 is above a fipe of 8000000")
	}
}

func TestExportLeavesFipeNullWithoutAMatch(t *testing.T) {
	srv := NewServer(fipedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")

	for _, id := range []string{"f3", "f4"} {
		listing := decoded.Listings[listingByExternalID(t, decoded, id)]
		if listing.Fipe != nil {
			t.Errorf("%s has no fipe match and should export null, got %+v", id, listing.Fipe)
		}
	}
}

func TestExportKeepsTheReferenceWithoutAnAskingPrice(t *testing.T) {
	srv := NewServer(fipedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")
	priceless := decoded.Listings[listingByExternalID(t, decoded, "f5")]

	if priceless.Fipe == nil {
		t.Fatal("a listing without a price still matches the table")
	}
	if priceless.Fipe.PriceCents != 8000000 {
		t.Errorf("the reference price should survive, got %d", priceless.Fipe.PriceCents)
	}
	if priceless.Fipe.GapPercent != nil || priceless.Fipe.BelowFipe != nil {
		t.Error("without an asking price there is no gap to report")
	}
}

func TestExportFlagsAMatchThroughTheBaseVariant(t *testing.T) {
	srv := NewServer(fipedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")
	unknown := decoded.Listings[listingByExternalID(t, decoded, "f6")]

	if unknown.Fipe == nil {
		t.Fatal("an unknown variant falls back to the base reference")
	}
	if !unknown.Fipe.BaseVariant {
		t.Error("a fallback match should be flagged as a base match")
	}
}

func repostedStore(t *testing.T) *store.Store {
	t.Helper()
	s := emptyStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	twin := exportListingOf("r1", model.VerdictMatch, 7200000)
	twin.Fingerprint = "street_glide|2015|6|curitiba"
	upsert(t, s, twin, at)

	elsewhere := exportListingOf("r2", model.VerdictReject, 7900000)
	elsewhere.Source = model.SourceMercadoLivre
	elsewhere.Fingerprint = twin.Fingerprint
	upsert(t, s, elsewhere, at.Add(-72*time.Hour))

	upsert(t, s, exportListingOf("r3", model.VerdictMatch, 6800000), at)
	return s
}

func TestExportCarriesTheRepostSiblingWithItsOwnFields(t *testing.T) {
	srv := NewServer(repostedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json?verdict=match")
	twin := decoded.Listings[listingByExternalID(t, decoded, "r1")]

	if len(twin.Reposts) != 1 {
		t.Fatalf("expected a single sibling, got %d", len(twin.Reposts))
	}
	sibling := twin.Reposts[0]
	if sibling.Source != model.SourceMercadoLivre {
		t.Errorf("the sibling source should travel with it, got %q", sibling.Source)
	}
	if sibling.Verdict != "reject" {
		t.Errorf("the sibling verdict should travel with it, got %q", sibling.Verdict)
	}
	if sibling.PriceCents == nil || *sibling.PriceCents != 7900000 {
		t.Errorf("the sibling price should travel with it, got %v", sibling.PriceCents)
	}
	if sibling.Km == nil || *sibling.Km != 31000 {
		t.Errorf("the sibling km should travel with it, got %v", sibling.Km)
	}
	if sibling.FirstSeenAt.IsZero() {
		t.Error("the sibling should carry when it was first seen")
	}
}

func TestExportCarriesASiblingLeftOutOfTheSlice(t *testing.T) {
	srv := NewServer(repostedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json?verdict=match")

	for _, listing := range decoded.Listings {
		if listing.Verdict == "reject" {
			t.Fatal("verdict=match should not export the reject as a listing")
		}
	}
	twin := decoded.Listings[listingByExternalID(t, decoded, "r1")]
	if len(twin.Reposts) != 1 {
		t.Errorf("a sibling outside the slice still belongs in reposts, got %d", len(twin.Reposts))
	}
}

func TestExportGivesAnEmptyRepostListToALoneListing(t *testing.T) {
	srv := NewServer(repostedStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json?verdict=match")
	alone := decoded.Listings[listingByExternalID(t, decoded, "r3")]

	if len(alone.Reposts) != 0 {
		t.Errorf("a listing without a twin has no reposts, got %d", len(alone.Reposts))
	}

	_, body := get(t, srv, "/export.json?verdict=match")
	if !strings.Contains(body, `"reposts": []`) {
		t.Errorf("an empty repost list should be an array, got %s", body)
	}
}

func sourceIn(t *testing.T, decoded decodedExport, name string) int {
	t.Helper()
	for i, s := range decoded.Sources {
		if s.Name == name {
			return i
		}
	}
	t.Fatalf("source %s not found in the export", name)
	return -1
}

func TestExportReportsTheHealthOfEachSource(t *testing.T) {
	s := exportStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	for i, count := range []int{9, 11, 10} {
		started := at.Add(time.Duration(i) * time.Hour)
		if err := s.RecordRun(model.SourceOLX, started, started.Add(time.Minute), count, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	srv := NewServer(s, fixed(model.SourceOLX, model.SourceWebmotors))

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Sources) != 2 {
		t.Fatalf("expected both configured sources, got %d", len(decoded.Sources))
	}
	olx := decoded.Sources[sourceIn(t, decoded, model.SourceOLX)]
	if olx.Status != crawl.HealthOK {
		t.Errorf("a source with productive runs should be ok, got %q", olx.Status)
	}
	if len(olx.RecentCounts) != 3 || olx.RecentCounts[0] != 10 {
		t.Errorf("recent counts should come newest first, got %v", olx.RecentCounts)
	}
	if olx.LastRunAt == nil || !olx.LastRunAt.Equal(at.Add(2*time.Hour).Add(time.Minute)) {
		t.Errorf("unexpected last run %v", olx.LastRunAt)
	}
}

func TestExportLeavesASilentSourceWithoutARun(t *testing.T) {
	srv := NewServer(exportStore(t), fixed(model.SourceOLX))

	decoded := exportOf(t, srv, "/export.json")
	olx := decoded.Sources[sourceIn(t, decoded, model.SourceOLX)]

	if olx.LastRunAt != nil {
		t.Errorf("a source that never ran has no last run, got %v", olx.LastRunAt)
	}

	_, body := get(t, srv, "/export.json")
	if !strings.Contains(body, `"recent_counts": []`) {
		t.Errorf("a source that never ran should carry an empty array, got %s", body)
	}
}
