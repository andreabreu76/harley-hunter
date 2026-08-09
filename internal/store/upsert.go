package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

type UpsertResult struct {
	ID            int64
	IsNew         bool
	PriceChanged  bool
	PreviousCents *int64
}

func (s *Store) Upsert(l model.Listing, now time.Time) (UpsertResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return UpsertResult{}, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var id int64
	var existingPrice *int64
	err = tx.QueryRow(
		"SELECT id, price_cents FROM listings WHERE source = ? AND external_id = ?",
		l.Source, l.ExternalID).Scan(&id, &existingPrice)

	reason, marshalErr := json.Marshal(l.VerdictReason)
	if marshalErr != nil {
		return UpsertResult{}, fmt.Errorf("encoding verdict reason: %w", marshalErr)
	}

	result := UpsertResult{}

	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, insertErr := tx.Exec(
			`INSERT INTO listings
             (source, external_id, url, title, raw_text, bike, variant, year, price_cents,
              km, city, state, image_url, verdict, verdict_reason, fingerprint,
              first_seen_at, last_seen_at)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			l.Source, l.ExternalID, l.URL, l.Title, l.RawText, l.Bike, l.Variant,
			l.Year, l.PriceCents, l.Km, l.City, l.State, l.ImageURL,
			string(l.Verdict), string(reason), l.Fingerprint, now, now)
		if insertErr != nil {
			return UpsertResult{}, fmt.Errorf("inserting listing: %w", insertErr)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return UpsertResult{}, fmt.Errorf("reading inserted id: %w", err)
		}
		result.IsNew = true

	case err != nil:
		return UpsertResult{}, fmt.Errorf("looking up listing: %w", err)

	default:
		if _, updateErr := tx.Exec(
			`UPDATE listings SET url = ?, title = ?, raw_text = ?, bike = ?, variant = ?,
             year = ?, price_cents = ?, km = ?, city = ?, state = ?, image_url = ?,
             verdict = ?, verdict_reason = ?, fingerprint = ?, status = 'active', last_seen_at = ?
             WHERE id = ?`,
			l.URL, l.Title, l.RawText, l.Bike, l.Variant, l.Year, l.PriceCents, l.Km,
			l.City, l.State, l.ImageURL, string(l.Verdict), string(reason),
			l.Fingerprint, now, id); updateErr != nil {
			return UpsertResult{}, fmt.Errorf("updating listing: %w", updateErr)
		}
		if l.PriceCents != nil && (existingPrice == nil || *existingPrice != *l.PriceCents) {
			result.PriceChanged = true
			result.PreviousCents = existingPrice
		}
	}

	if l.PriceCents != nil && (result.IsNew || result.PriceChanged) {
		if _, histErr := tx.Exec(
			"INSERT INTO price_history (listing_id, price_cents, observed_at) VALUES (?, ?, ?)",
			id, *l.PriceCents, now); histErr != nil {
			return UpsertResult{}, fmt.Errorf("recording price history: %w", histErr)
		}
	}

	if err := tx.Commit(); err != nil {
		return UpsertResult{}, fmt.Errorf("committing transaction: %w", err)
	}

	result.ID = id
	return result, nil
}
