package web

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

const starMarker = `class="estrela"`

func cardOf(t *testing.T, body, title string) string {
	t.Helper()
	for chunk := range strings.SplitSeq(body, "<article") {
		if strings.Contains(chunk, title) {
			return chunk
		}
	}
	t.Fatalf("no card rendered for %q", title)
	return ""
}

func twoBatches(t *testing.T, s *store.Store, base time.Time) int64 {
	t.Helper()
	record := func(at time.Time) {
		if err := s.RecordRun("olx", at, at.Add(time.Minute), 10, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	record(base)
	upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "old", URL: "https://example.com/old",
		Title: "Glide Antigo", City: "curitiba", State: "PR", Verdict: model.VerdictMatch,
	}, base.Add(2*time.Minute))

	record(base.Add(12 * time.Hour))
	return upsert(t, s, model.Listing{
		Source: "olx", ExternalID: "new", URL: "https://example.com/new",
		Title: "Glide Novo", City: "curitiba", State: "PR", Verdict: model.VerdictMatch,
	}, base.Add(12*time.Hour+2*time.Minute))
}

func TestListStarsOnlyTheBatchFromTheLatestRun(t *testing.T) {
	s := emptyStore(t)
	twoBatches(t, s, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))

	_, body := get(t, NewServer(s, fixed("olx")), "/")

	if !strings.Contains(cardOf(t, body, "Glide Novo"), starMarker) {
		t.Error("a listing discovered on the latest run must carry the star")
	}
	if strings.Contains(cardOf(t, body, "Glide Antigo"), starMarker) {
		t.Error("a listing already known before the latest run must not carry the star")
	}
}

func TestExportTellsWhichListingsCameFromTheLatestRun(t *testing.T) {
	s := emptyStore(t)
	twoBatches(t, s, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))

	decoded := exportOf(t, NewServer(s, fixed("olx")), "/export.json")

	got := map[string]bool{}
	for _, l := range decoded.Listings {
		got[l.ExternalID] = l.Fresh
	}
	if !got["new"] || got["old"] {
		t.Errorf("fresh by external id = %v, want new true and old false", got)
	}
}

func TestListingDetailStarsAFreshListing(t *testing.T) {
	s := emptyStore(t)
	id := twoBatches(t, s, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))

	_, body := get(t, NewServer(s, fixed("olx")), "/listing/"+strconv.FormatInt(id, 10))

	if !strings.Contains(body, starMarker) {
		t.Error("the detail page of a fresh listing must carry the star")
	}
}
