package store

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
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

func TestOrderingIsDeterministicWhenTimestampsTie(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	var ids []int64
	for i := 0; i < 5; i++ {
		l := sample(7200000)
		l.ExternalID = fmt.Sprintf("tied-%d", i)
		res, err := s.Upsert(l, now)
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		ids = append(ids, res.ID)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if got := idsOf(pending); !equalIDs(got, ids) {
		t.Errorf("PendingNotifications order = %v, want %v (oldest first, id ascending on ties)", got, ids)
	}

	listed, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if got := idsOf(listed); !equalIDs(got, reversedIDs(ids)) {
		t.Errorf("ListByVerdict order = %v, want %v (newest first, id descending on ties)", got, reversedIDs(ids))
	}

	for _, c := range []int{10, 20, 30, 40} {
		if err := s.RecordRun("olx", now, now, c, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	counts, err := s.RecentRunCounts("olx", 10)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	want := []int{40, 30, 20, 10}
	if len(counts) != len(want) {
		t.Fatalf("counts = %v, want %v", counts, want)
	}
	for i := range want {
		if counts[i] != want[i] {
			t.Errorf("RecentRunCounts = %v, want %v (most recent first on tied timestamps)", counts, want)
			break
		}
	}
}

func TestCompetingWriterWaitsInsteadOfFailing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	crawler, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { crawler.Close() })
	dashboard, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dashboard.Close() })

	tx, err := dashboard.db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO listings (source, external_id, url, title, bike, variant, verdict, first_seen_at, last_seen_at)
         VALUES ('olx', 'holder', 'u', 't', 'b', 'v', 'match', ?, ?)`,
		time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("holding the write lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := crawler.Upsert(sample(6900000), time.Now())
		done <- err
	}()

	select {
	case err := <-done:
		tx.Rollback()
		t.Fatalf("competing writer gave up immediately instead of waiting: %v", err)
	case <-time.After(250 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("competing writer failed after the lock was released: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("competing writer never completed")
	}
}

func idsOf(rows []Row) []int64 {
	out := make([]int64, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func reversedIDs(ids []int64) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[len(ids)-1-i] = id
	}
	return out
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func pricesOf(points []PricePoint) []int64 {
	out := make([]int64, len(points))
	for i, p := range points {
		out[i] = p.PriceCents
	}
	return out
}
