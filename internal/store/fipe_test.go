package store

import (
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/fipe"
	"github.com/andreabreu76/harley-hunter/internal/model"
)

func fortyEight2016() fipe.Reference {
	return fipe.Reference{
		Code: "810066-7", Label: "XL 1200X", Bike: model.BikeSportster1200, Variant: model.VariantFortyEight,
		Year: 2016, PriceCents: 4698800, Month: "setembro de 2026",
	}
}

func TestFipeReferencesRoundTrip(t *testing.T) {
	s := openTemp(t)
	now := time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)

	if err := s.SaveFipeReference(fortyEight2016(), now); err != nil {
		t.Fatalf("SaveFipeReference: %v", err)
	}

	refs, err := s.FipeReferences()
	if err != nil {
		t.Fatalf("FipeReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d references, want 1", len(refs))
	}
	if refs[0] != fortyEight2016() {
		t.Errorf("reference = %+v, want %+v", refs[0], fortyEight2016())
	}
}

func TestSavingTheSameComboReplacesTheOldQuote(t *testing.T) {
	s := openTemp(t)
	now := time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)

	if err := s.SaveFipeReference(fortyEight2016(), now); err != nil {
		t.Fatalf("SaveFipeReference: %v", err)
	}
	newer := fortyEight2016()
	newer.PriceCents = 7000000
	newer.Month = "setembro de 2026"
	if err := s.SaveFipeReference(newer, now.AddDate(0, 1, 0)); err != nil {
		t.Fatalf("SaveFipeReference: %v", err)
	}

	refs, err := s.FipeReferences()
	if err != nil {
		t.Fatalf("FipeReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d references, want the combo stored once", len(refs))
	}
	if refs[0].PriceCents != 7000000 || refs[0].Month != "setembro de 2026" {
		t.Errorf("reference = %+v, want the newer quote", refs[0])
	}
}

func TestFipeFetchedAtTellsWhichCombosAreStale(t *testing.T) {
	s := openTemp(t)
	august := time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)

	if err := s.SaveFipeReference(fortyEight2016(), august); err != nil {
		t.Fatalf("SaveFipeReference: %v", err)
	}

	fetched, err := s.FipeFetchedAt()
	if err != nil {
		t.Fatalf("FipeFetchedAt: %v", err)
	}
	at, ok := fetched["sportster_1200|forty_eight|2016"]
	if !ok {
		t.Fatalf("fetched map = %v, want the combo that was saved", fetched)
	}
	if !at.Equal(august) {
		t.Errorf("fetched at = %s, want %s", at, august)
	}
	if at.Location() != time.UTC {
		t.Errorf("fetched at zone = %s, want UTC", at.Location())
	}
}

func TestFipeReferencesOnADatabaseFromTheEarlierSchema(t *testing.T) {
	path := openLegacy(t)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	refs, err := s.FipeReferences()
	if err != nil {
		t.Fatalf("FipeReferences on the migrated database: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("got %d references, want none on a database that never had the table", len(refs))
	}
	if err := s.SaveFipeReference(fortyEight2016(), time.Now()); err != nil {
		t.Fatalf("SaveFipeReference on the migrated database: %v", err)
	}
}
