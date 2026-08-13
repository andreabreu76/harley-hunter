package store

import (
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func freshOf(t *testing.T, s *Store, id int64) bool {
	t.Helper()
	row, _, err := s.GetRow(id)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	return row.Fresh
}

func seenAt(t *testing.T, s *Store, source, externalID string, at time.Time) int64 {
	t.Helper()
	listing := sample(7200000)
	listing.Source = source
	listing.ExternalID = externalID
	res, err := s.Upsert(listing, at)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	return res.ID
}

func TestFreshMarksListingFirstSeenOnTheLatestRun(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	runAt(t, s, "olx", base, 10, "ok")
	id := seenAt(t, s, "olx", "abc123", base.Add(2*time.Minute))

	if !freshOf(t, s, id) {
		t.Error("a listing first seen during the latest run must be fresh")
	}
}

func TestFreshClearsOnceANewProductiveRunPasses(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	runAt(t, s, "olx", base, 10, "ok")
	id := seenAt(t, s, "olx", "abc123", base.Add(2*time.Minute))
	runAt(t, s, "olx", base.Add(12*time.Hour), 10, "ok")

	if freshOf(t, s, id) {
		t.Error("a listing already carried by a later run is no longer fresh")
	}
}

func TestFreshSurvivesARunThatFailed(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	runAt(t, s, "olx", base, 10, "ok")
	id := seenAt(t, s, "olx", "abc123", base.Add(2*time.Minute))
	runAt(t, s, "olx", base.Add(12*time.Hour), 0, "error")

	if !freshOf(t, s, id) {
		t.Error("a failed run must not retire the last batch the owner never saw")
	}
}

func TestFreshIgnoresRunsFromOtherSources(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	runAt(t, s, "olx", base, 10, "ok")
	id := seenAt(t, s, "olx", "abc123", base.Add(2*time.Minute))
	runAt(t, s, "mercadolivre", base.Add(12*time.Hour), 10, "ok")

	if !freshOf(t, s, id) {
		t.Error("a run on another source must not retire this one's batch")
	}
}

func TestFreshIsFalseWithoutAnyProductiveRun(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	id := seenAt(t, s, "olx", "abc123", base)

	if freshOf(t, s, id) {
		t.Error("without a recorded run there is no batch to call fresh")
	}
}

func TestFreshTravelsThroughListByVerdict(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	runAt(t, s, "olx", base, 10, "ok")
	seenAt(t, s, "olx", "old", base.Add(2*time.Minute))
	runAt(t, s, "olx", base.Add(12*time.Hour), 10, "ok")
	seenAt(t, s, "olx", "new", base.Add(12*time.Hour+2*time.Minute))

	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.ExternalID] = r.Fresh
	}
	if !got["new"] || got["old"] {
		t.Errorf("fresh by external id = %v, want new true and old false", got)
	}
}
