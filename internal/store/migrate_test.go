package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

const phaseOneSchema = `
CREATE TABLE IF NOT EXISTS listings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    raw_text TEXT NOT NULL DEFAULT '',
    bike TEXT NOT NULL,
    variant TEXT NOT NULL,
    year INTEGER,
    price_cents INTEGER,
    km INTEGER,
    city TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    image_url TEXT NOT NULL DEFAULT '',
    verdict TEXT NOT NULL,
    verdict_reason TEXT NOT NULL DEFAULT '{}',
    fingerprint TEXT NOT NULL DEFAULT '',
    user_state TEXT NOT NULL DEFAULT 'new',
    notified INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    first_seen_at TIMESTAMP NOT NULL,
    last_seen_at TIMESTAMP NOT NULL,
    UNIQUE (source, external_id)
);
`

func openLegacy(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("creating legacy database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(phaseOneSchema); err != nil {
		t.Fatalf("applying legacy schema: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO listings (source, external_id, url, title, bike, variant, verdict, first_seen_at, last_seen_at)
         VALUES ('olx', 'abc123', 'https://olx.com.br/abc123', 'Harley Street Glide 2015', 'street_glide', 'base', 'match', ?, ?)`,
		time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seeding legacy row: %v", err)
	}
	return path
}

func TestOpenAddsMissingColumnsToADatabaseFromTheEarlierSchema(t *testing.T) {
	path := openLegacy(t)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	rows, err := s.ListByVerdict("match")
	if err != nil {
		t.Fatalf("ListByVerdict on the migrated database: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want the row that was already there", len(rows))
	}
	if rows[0].Phone != nil {
		t.Errorf("Phone = %q, want nil for a row written before the column existed", *rows[0].Phone)
	}

	l := sample(7200000)
	phone := "11982413574"
	l.Phone = &phone
	if _, err := s.Upsert(l, time.Now()); err != nil {
		t.Fatalf("Upsert into the migrated database: %v", err)
	}

	row, _, err := s.GetRow(rows[0].ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.Phone == nil || *row.Phone != phone {
		t.Fatalf("Phone = %v, want %q", row.Phone, phone)
	}
}

func TestOpenIsSafeToRunTwiceOverTheSameDatabase(t *testing.T) {
	path := openLegacy(t)

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	first.Close()

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	t.Cleanup(func() { second.Close() })

	if _, err := second.ListByVerdict("match"); err != nil {
		t.Fatalf("ListByVerdict after reopening: %v", err)
	}
}

func TestOpenAddsThePublishedDateColumnToADatabaseFromTheEarlierSchema(t *testing.T) {
	path := openLegacy(t)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	rows, err := s.ListByVerdict("match")
	if err != nil {
		t.Fatalf("ListByVerdict on the migrated database: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want the row that was already there", len(rows))
	}
	if rows[0].PublishedAt != nil {
		t.Errorf("PublishedAt = %s, want nil for a row written before the column existed", rows[0].PublishedAt)
	}

	published := time.Date(2026, 8, 4, 12, 49, 53, 0, time.UTC)
	l := sample(7200000)
	l.PublishedAt = &published
	if _, err := s.Upsert(l, time.Now()); err != nil {
		t.Fatalf("Upsert into the migrated database: %v", err)
	}

	row, _, err := s.GetRow(rows[0].ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.PublishedAt == nil || !row.PublishedAt.Equal(published) {
		t.Fatalf("PublishedAt = %v, want %s", row.PublishedAt, published)
	}
}

func TestOpenAnchorsAlreadyNotifiedRowsAtTheirCurrentPrice(t *testing.T) {
	path := openLegacy(t)

	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopening legacy database: %v", err)
	}
	if _, err := legacy.Exec("UPDATE listings SET notified = 1, price_cents = 7200000"); err != nil {
		t.Fatalf("marking the legacy row notified: %v", err)
	}
	legacy.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	rows, err := s.ListByVerdict("match")
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want the legacy row", len(rows))
	}
	if rows[0].NotifiedPriceCents == nil || *rows[0].NotifiedPriceCents != 7200000 {
		t.Errorf("NotifiedPriceCents = %v, want the price already communicated", rows[0].NotifiedPriceCents)
	}
}

func TestOpenDoesNotReanchorAPendingDropOnASecondRun(t *testing.T) {
	path := openLegacy(t)

	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopening legacy database: %v", err)
	}
	if _, err := legacy.Exec("UPDATE listings SET notified = 1, price_cents = 7200000"); err != nil {
		t.Fatalf("marking the legacy row notified: %v", err)
	}
	legacy.Close()

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	first.Close()

	dropped, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopening to drop the price: %v", err)
	}
	if _, err := dropped.Exec("UPDATE listings SET price_cents = 6800000"); err != nil {
		t.Fatalf("dropping the price: %v", err)
	}
	dropped.Close()

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	t.Cleanup(func() { second.Close() })

	rows, err := second.ListByVerdict("match")
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if rows[0].NotifiedPriceCents == nil || *rows[0].NotifiedPriceCents != 7200000 {
		t.Errorf("NotifiedPriceCents = %v, want the anchor to survive so the drop stays pending",
			rows[0].NotifiedPriceCents)
	}
}
