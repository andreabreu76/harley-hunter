package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
	Phone           *string
	Verdict         model.Verdict
	UserState       string
	Status          string
	Fingerprint     string
	PublishedAt     *time.Time
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	Fresh           bool
	FirstPriceCents *int64

	Notified           bool
	NotifiedPriceCents *int64
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
    phone TEXT,
    verdict TEXT NOT NULL,
    verdict_reason TEXT NOT NULL DEFAULT '{}',
    fingerprint TEXT NOT NULL DEFAULT '',
    user_state TEXT NOT NULL DEFAULT 'new',
    notified INTEGER NOT NULL DEFAULT 0,
    notified_price_cents INTEGER,
    status TEXT NOT NULL DEFAULT 'active',
    published_at TIMESTAMP,
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

CREATE TABLE IF NOT EXISTS fipe_refs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL,
    label TEXT NOT NULL,
    bike TEXT NOT NULL,
    variant TEXT NOT NULL,
    year INTEGER NOT NULL,
    price_cents INTEGER NOT NULL,
    month TEXT NOT NULL DEFAULT '',
    fetched_at TIMESTAMP NOT NULL,
    UNIQUE (bike, variant, year)
);
`

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	if err := addMissingColumns(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

const rowColumns = `
    l.id, l.source, l.external_id, l.url, l.title, l.bike, l.variant, l.year,
    l.price_cents, l.km, l.city, l.state, l.image_url, l.phone, l.verdict, l.user_state,
    l.status, l.fingerprint, l.published_at, l.first_seen_at, l.last_seen_at,
    COALESCE(l.first_seen_at >= (
        SELECT r.started_at FROM source_runs r
        WHERE r.source = l.source AND r.status = 'ok' AND r.item_count > 0
        ORDER BY r.started_at DESC, r.id DESC LIMIT 1), 0),
    (SELECT price_cents FROM price_history p WHERE p.listing_id = l.id ORDER BY p.observed_at ASC, p.id ASC LIMIT 1),
    l.notified, l.notified_price_cents
`

func scanRow(scanner interface{ Scan(...any) error }) (Row, error) {
	var r Row
	err := scanner.Scan(&r.ID, &r.Source, &r.ExternalID, &r.URL, &r.Title, &r.Bike,
		&r.Variant, &r.Year, &r.PriceCents, &r.Km, &r.City, &r.State, &r.ImageURL,
		&r.Phone, &r.Verdict, &r.UserState, &r.Status, &r.Fingerprint, &r.PublishedAt,
		&r.FirstSeenAt, &r.LastSeenAt, &r.Fresh, &r.FirstPriceCents, &r.Notified, &r.NotifiedPriceCents)
	return r, err
}

func (s *Store) ListByVerdict(v model.Verdict) ([]Row, error) {
	query := "SELECT " + rowColumns + " FROM listings l WHERE l.verdict = ? ORDER BY l.first_seen_at DESC, l.id DESC"
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
        ORDER BY l.first_seen_at ASC, l.id ASC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("querying pending notifications: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

func (s *Store) PendingAlerts() ([]Row, error) {
	query := "SELECT " + rowColumns + ` FROM listings l
        WHERE l.verdict = 'match' AND l.status = 'active'
          AND (l.notified = 0
               OR (l.price_cents IS NOT NULL AND l.notified_price_cents IS NOT NULL
                   AND l.price_cents < l.notified_price_cents))
        ORDER BY l.first_seen_at ASC, l.id ASC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying pending alerts: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

func (s *Store) MarkNotified(id int64, priceCents *int64) error {
	_, err := s.db.Exec(
		`UPDATE listings SET notified = 1,
             notified_price_cents = CASE
                 WHEN ? IS NULL THEN notified_price_cents
                 WHEN notified_price_cents IS NULL THEN ?
                 WHEN ? < notified_price_cents THEN ?
                 ELSE notified_price_cents END
         WHERE id = ?`,
		priceCents, priceCents, priceCents, priceCents, id)
	if err != nil {
		return fmt.Errorf("marking listing as notified: %w", err)
	}
	return nil
}

func (s *Store) MarkSilenced(id int64, priceCents *int64) error {
	_, err := s.db.Exec(
		`UPDATE listings SET notified = 1,
             notified_price_cents = CASE
                 WHEN notified_price_cents IS NULL THEN ?
                 ELSE notified_price_cents END
         WHERE id = ?`,
		priceCents, id)
	if err != nil {
		return fmt.Errorf("silencing listing: %w", err)
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
		"SELECT item_count FROM source_runs WHERE source = ? ORDER BY started_at DESC, id DESC LIMIT ?", source, limit)
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

func (s *Store) CountByState() (map[string]int, error) {
	rows, err := s.db.Query("SELECT state, COUNT(*) FROM listings GROUP BY state")
	if err != nil {
		return nil, fmt.Errorf("counting listings by state: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, fmt.Errorf("scanning state count: %w", err)
		}
		counts[state] = count
	}
	return counts, rows.Err()
}

func (s *Store) CountByVerdict() (map[string]int, error) {
	rows, err := s.db.Query("SELECT verdict, COUNT(*) FROM listings GROUP BY verdict")
	if err != nil {
		return nil, fmt.Errorf("counting listings by verdict: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var verdict string
		var count int
		if err := rows.Scan(&verdict, &count); err != nil {
			return nil, fmt.Errorf("scanning verdict count: %w", err)
		}
		counts[verdict] = count
	}
	return counts, rows.Err()
}

func (s *Store) PriceHistoryFor(ids []int64) (map[int64][]PricePoint, error) {
	history := make(map[int64][]PricePoint, len(ids))
	if len(ids) == 0 {
		return history, nil
	}

	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")

	rows, err := s.db.Query(
		`SELECT listing_id, price_cents, observed_at FROM price_history
         WHERE listing_id IN (`+placeholders+`)
         ORDER BY listing_id, observed_at ASC, id ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("querying price history: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var p PricePoint
		if err := rows.Scan(&id, &p.PriceCents, &p.ObservedAt); err != nil {
			return nil, fmt.Errorf("scanning price point: %w", err)
		}
		history[id] = append(history[id], p)
	}
	return history, rows.Err()
}

func (s *Store) LastRunAt(source string) (time.Time, bool, error) {
	var at time.Time
	err := s.db.QueryRow(
		"SELECT finished_at FROM source_runs WHERE source = ? ORDER BY started_at DESC, id DESC LIMIT 1",
		source).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("querying last run: %w", err)
	}
	return at, true, nil
}

func (s *Store) LastRunStartedAt() (time.Time, bool, error) {
	var at time.Time
	err := s.db.QueryRow(
		"SELECT started_at FROM source_runs ORDER BY started_at DESC, id DESC LIMIT 1").Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("querying the last round: %w", err)
	}
	return at, true, nil
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
