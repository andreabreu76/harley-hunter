package store

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOrderingSurvivesATimezoneChange(t *testing.T) {
	s := openTemp(t)
	utc := time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)
	brt := time.FixedZone("BRT", -3*3600)

	for i, count := range []int{1, 2, 3} {
		at := utc.Add(time.Duration(i) * time.Hour)
		if i == 1 {
			at = at.In(brt)
		}
		if err := s.RecordRun("olx", at, at, count, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}

	counts, err := s.RecentRunCounts("olx", 5)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	want := []int{3, 2, 1}
	if len(counts) != len(want) {
		t.Fatalf("counts = %v, want %v", counts, want)
	}
	for i := range want {
		if counts[i] != want[i] {
			t.Fatalf("counts = %v, want %v (most recent first)", counts, want)
		}
	}
}

func TestPriceHistoryStaysChronologicalAcrossTimezones(t *testing.T) {
	s := openTemp(t)
	utc := time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)
	brt := time.FixedZone("BRT", -3*3600)

	if _, err := s.Upsert(sample(7500000), utc); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if _, err := s.Upsert(sample(7300000), utc.Add(time.Hour).In(brt)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	res, err := s.Upsert(sample(7100000), utc.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	row, points, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	want := []int64{7500000, 7300000, 7100000}
	if len(points) != len(want) {
		t.Fatalf("got %d price points, want %d", len(points), len(want))
	}
	for i := range want {
		if points[i].PriceCents != want[i] {
			t.Fatalf("price points out of order: got %v, want %v", pricesOf(points), want)
		}
	}
	if row.FirstPriceCents == nil || *row.FirstPriceCents != 7500000 {
		t.Errorf("FirstPriceCents = %v, want 7500000", row.FirstPriceCents)
	}
}

func TestForeignKeysAreOnForEveryPooledConnection(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "fk.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	const connections = 8
	var wg sync.WaitGroup
	release := make(chan struct{})
	states := make([]int, connections)

	for i := range states {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn, err := s.db.Conn(t.Context())
			if err != nil {
				t.Errorf("Conn: %v", err)
				return
			}
			defer conn.Close()
			<-release
			if err := conn.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&states[i]); err != nil {
				t.Errorf("reading pragma: %v", err)
			}
		}(i)
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, on := range states {
		if on != 1 {
			t.Errorf("connection %d has foreign_keys off: %v", i, states)
			break
		}
	}
}

func pricesOf(points []PricePoint) []int64 {
	out := make([]int64, len(points))
	for i, p := range points {
		out[i] = p.PriceCents
	}
	return out
}
