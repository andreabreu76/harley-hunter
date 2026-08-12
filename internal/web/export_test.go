package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func exportListingOf(id string, verdict model.Verdict, cents int64) model.Listing {
	year := 2015
	km := 31000
	return model.Listing{
		Source: model.SourceOLX, ExternalID: id, URL: "https://example.com/" + id,
		Title: "Harley Street Glide " + id, Bike: model.BikeStreetGlide,
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
	Listings    []struct {
		ID         int64   `json:"id"`
		ExternalID string  `json:"external_id"`
		Verdict    string  `json:"verdict"`
		Status     string  `json:"status"`
		PriceCents *int64  `json:"price_cents"`
		Km         *int    `json:"km"`
		Phone      *string `json:"phone"`

		PriceDropCents *int64 `json:"price_drop_cents"`
		PriceHistory   []struct {
			PriceCents int64     `json:"price_cents"`
			At         time.Time `json:"at"`
		} `json:"price_history"`
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
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

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
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	for _, wanted := range []string{"match", "maybe"} {
		decoded := exportOf(t, srv, "/export.json?verdict="+wanted)
		if len(decoded.Listings) != 1 || decoded.Listings[0].Verdict != wanted {
			t.Errorf("verdict=%s returned %v", wanted, verdictsIn(decoded))
		}
	}
}

func TestExportAllBringsTheRejects(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json?verdict=all")

	if len(decoded.Listings) != 3 {
		t.Fatalf("expected the three verdicts, got %v", verdictsIn(decoded))
	}
}

func TestExportRejectsAnUnknownVerdict(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	code, _ := get(t, srv, "/export.json?verdict=lixo")

	if code != http.StatusBadRequest {
		t.Errorf("expected 400 for an unknown verdict, got %d", code)
	}
}

func TestExportCountsTheWholeDatabaseNotTheSlice(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

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
	srv := NewServer(s, []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Listings) != 1 {
		t.Fatalf("a closed listing still belongs in the export, got %d", len(decoded.Listings))
	}
	if decoded.Listings[0].Status != "gone" {
		t.Errorf("the closed listing should carry its status, got %q", decoded.Listings[0].Status)
	}
}

func TestExportServesJSONContentType(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	rec := recorderFor(t, srv, "/export.json")

	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("unexpected content type %q", got)
	}
}

func TestExportOfAnEmptyDatabaseIsAnEmptyList(t *testing.T) {
	srv := NewServer(emptyStore(t), []string{model.SourceOLX})

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
	srv := NewServer(s, []string{model.SourceOLX})

	_, body := get(t, srv, "/export.json")

	for _, field := range []string{"price_cents", "km", "phone", "published_at"} {
		if !strings.Contains(body, `"`+field+`": null`) {
			t.Errorf("%s should be null when absent, got %s", field, body)
		}
	}
}

func TestExportDatesAreUTCWhateverTheLocalZone(t *testing.T) {
	useSaoPauloZone(t)
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

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
	srv := NewServer(s, []string{model.SourceOLX})

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
	srv := NewServer(droppedStore(t), []string{model.SourceOLX})

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
	srv := NewServer(droppedStore(t), []string{model.SourceOLX})

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
	srv := NewServer(droppedStore(t), []string{model.SourceOLX})

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
	srv := NewServer(s, []string{model.SourceOLX})

	_, body := get(t, srv, "/export.json")

	if !strings.Contains(body, `"price_history": []`) {
		t.Errorf("a listing without history should export an empty array, got %s", body)
	}
}
