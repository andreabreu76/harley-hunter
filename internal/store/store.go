package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Row struct {
	ID              int64
	Source          string
	ExternalID      string
	URL             string
	Title           string
	Bike            string
	Variant         string
	Year            *int
	PriceCents      *int64
	Km              *int
	City            string
	State           string
	ImageURL        string
	Verdict         model.Verdict
	UserState       string
	Fingerprint     string
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	FirstPriceCents *int64
}

type PricePoint struct {
	PriceCents int64
	ObservedAt time.Time
}

const schema = `
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

CREATE INDEX IF NOT EXISTS idx_listings_verdict ON listings (verdict, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_listings_fingerprint ON listings (fingerprint);

CREATE TABLE IF NOT EXISTS price_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    listing_id INTEGER NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    price_cents INTEGER NOT NULL,
    observed_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_price_history_listing ON price_history (listing_id, observed_at);

CREATE TABLE IF NOT EXISTS source_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    started_at TIMESTAMP NOT NULL,
    finished_at TIMESTAMP NOT NULL,
    item_count INTEGER NOT NULL,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_source_runs_source ON source_runs (source, started_at DESC);
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

const rowColumns = `
    l.id, l.source, l.external_id, l.url, l.title, l.bike, l.variant, l.year,
    l.price_cents, l.km, l.city, l.state, l.image_url, l.verdict, l.user_state,
    l.fingerprint, l.first_seen_at, l.last_seen_at,
    (SELECT price_cents FROM price_history p WHERE p.listing_id = l.id ORDER BY p.observed_at ASC, p.id ASC LIMIT 1)
`

func scanRow(scanner interface{ Scan(...any) error }) (Row, error) {
	var r Row
	err := scanner.Scan(&r.ID, &r.Source, &r.ExternalID, &r.URL, &r.Title, &r.Bike,
		&r.Variant, &r.Year, &r.PriceCents, &r.Km, &r.City, &r.State, &r.ImageURL,
		&r.Verdict, &r.UserState, &r.Fingerprint, &r.FirstSeenAt, &r.LastSeenAt,
		&r.FirstPriceCents)
	return r, err
}

func (s *Store) ListByVerdict(v model.Verdict) ([]Row, error) {
	query := "SELECT " + rowColumns + " FROM listings l WHERE l.verdict = ? ORDER BY l.first_seen_at DESC"
	rows, err := s.db.Query(query, string(v))
	if err != nil {
		return nil, fmt.Errorf("querying listings: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

func (s *Store) PendingNotifications(limit int) ([]Row, error) {
	query := "SELECT " + rowColumns + ` FROM listings l
        WHERE l.verdict = 'match' AND l.notified = 0 AND l.status = 'active'
        ORDER BY l.first_seen_at ASC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("querying pending notifications: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

func (s *Store) MarkNotified(id int64) error {
	if _, err := s.db.Exec("UPDATE listings SET notified = 1 WHERE id = ?", id); err != nil {
		return fmt.Errorf("marking listing as notified: %w", err)
	}
	return nil
}

func (s *Store) SetUserState(id int64, state string) error {
	if _, err := s.db.Exec("UPDATE listings SET user_state = ? WHERE id = ?", state, id); err != nil {
		return fmt.Errorf("updating user state: %w", err)
	}
	return nil
}

func (s *Store) GetRow(id int64) (Row, []PricePoint, error) {
	query := "SELECT " + rowColumns + " FROM listings l WHERE l.id = ?"
	row, err := scanRow(s.db.QueryRow(query, id))
	if err != nil {
		return Row{}, nil, fmt.Errorf("loading listing: %w", err)
	}

	rows, err := s.db.Query(
		"SELECT price_cents, observed_at FROM price_history WHERE listing_id = ? ORDER BY observed_at ASC, id ASC", id)
	if err != nil {
		return Row{}, nil, fmt.Errorf("loading price history: %w", err)
	}
	defer rows.Close()

	var points []PricePoint
	for rows.Next() {
		var p PricePoint
		if err := rows.Scan(&p.PriceCents, &p.ObservedAt); err != nil {
			return Row{}, nil, fmt.Errorf("scanning price point: %w", err)
		}
		points = append(points, p)
	}
	return row, points, rows.Err()
}

func (s *Store) RecordRun(source string, started, finished time.Time, itemCount int, status, errMessage string) error {
	_, err := s.db.Exec(
		`INSERT INTO source_runs (source, started_at, finished_at, item_count, status, error)
         VALUES (?, ?, ?, ?, ?, ?)`,
		source, started.UTC(), finished.UTC(), itemCount, status, errMessage)
	if err != nil {
		return fmt.Errorf("recording source run: %w", err)
	}
	return nil
}

func (s *Store) RecentRunCounts(source string, limit int) ([]int, error) {
	rows, err := s.db.Query(
		"SELECT item_count FROM source_runs WHERE source = ? ORDER BY started_at DESC LIMIT ?", source, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recent runs: %w", err)
	}
	defer rows.Close()

	var counts []int
	for rows.Next() {
		var c int
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scanning run count: %w", err)
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
}

func collectRows(rows *sql.Rows) ([]Row, error) {
	var result []Row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning listing: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
