package store

import (
	"fmt"
	"strconv"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/fipe"
)

func (s *Store) FipeReferences() ([]fipe.Reference, error) {
	rows, err := s.db.Query(
		`SELECT code, label, bike, variant, year, price_cents, month
         FROM fipe_refs ORDER BY bike, variant, year`)
	if err != nil {
		return nil, fmt.Errorf("querying fipe references: %w", err)
	}
	defer rows.Close()

	var refs []fipe.Reference
	for rows.Next() {
		var r fipe.Reference
		if err := rows.Scan(&r.Code, &r.Label, &r.Bike, &r.Variant, &r.Year, &r.PriceCents, &r.Month); err != nil {
			return nil, fmt.Errorf("scanning fipe reference: %w", err)
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

func (s *Store) SaveFipeReference(r fipe.Reference, fetchedAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO fipe_refs (code, label, bike, variant, year, price_cents, month, fetched_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT (bike, variant, year) DO UPDATE SET
             code = excluded.code, label = excluded.label, price_cents = excluded.price_cents,
             month = excluded.month, fetched_at = excluded.fetched_at`,
		r.Code, r.Label, r.Bike, r.Variant, r.Year, r.PriceCents, r.Month, fetchedAt.UTC())
	if err != nil {
		return fmt.Errorf("saving fipe reference: %w", err)
	}
	return nil
}

func (s *Store) FipeFetchedAt() (map[string]time.Time, error) {
	rows, err := s.db.Query("SELECT bike, variant, year, fetched_at FROM fipe_refs")
	if err != nil {
		return nil, fmt.Errorf("querying fipe freshness: %w", err)
	}
	defer rows.Close()

	fetched := make(map[string]time.Time)
	for rows.Next() {
		var bike, variant string
		var year int
		var at time.Time
		if err := rows.Scan(&bike, &variant, &year, &at); err != nil {
			return nil, fmt.Errorf("scanning fipe freshness: %w", err)
		}
		fetched[bike+"|"+variant+"|"+strconv.Itoa(year)] = at.UTC()
	}
	return fetched, rows.Err()
}
