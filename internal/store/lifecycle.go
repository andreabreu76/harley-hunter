package store

import "fmt"

const (
	StatusActive = "active"
	StatusGone   = "gone"
)

const ExpiryRounds = 3

const repostFilter = "l.km IS NOT NULL AND l.fingerprint <> ''"

func (s *Store) ExpireUnseen(rounds int) (int, error) {
	if rounds <= 0 {
		rounds = ExpiryRounds
	}
	res, err := s.db.Exec(
		`UPDATE listings SET status = ?
         WHERE status = ?
           AND last_seen_at < (
               SELECT r.started_at FROM source_runs r
               WHERE r.source = listings.source AND r.status = 'ok' AND r.item_count > 0
               ORDER BY r.started_at DESC, r.id DESC
               LIMIT 1 OFFSET ?)`,
		StatusGone, StatusActive, rounds-1)
	if err != nil {
		return 0, fmt.Errorf("expiring unseen listings: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("counting expired listings: %w", err)
	}
	return int(affected), nil
}

func (s *Store) RepostsOf(fingerprint string, excludeID int64) ([]Row, error) {
	if fingerprint == "" {
		return nil, nil
	}
	query := "SELECT " + rowColumns + ` FROM listings l
        WHERE l.fingerprint = ? AND l.id <> ? AND ` + repostFilter + `
        ORDER BY l.first_seen_at DESC, l.id DESC`
	rows, err := s.db.Query(query, fingerprint, excludeID)
	if err != nil {
		return nil, fmt.Errorf("querying reposts: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

func (s *Store) RepostGroups() (map[string][]Row, error) {
	query := "SELECT " + rowColumns + ` FROM listings l
        WHERE ` + repostFilter + ` AND l.fingerprint IN (
            SELECT fingerprint FROM listings
            WHERE km IS NOT NULL AND fingerprint <> ''
            GROUP BY fingerprint HAVING COUNT(*) > 1)
        ORDER BY l.first_seen_at DESC, l.id DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("querying repost groups: %w", err)
	}
	defer rows.Close()

	collected, err := collectRows(rows)
	if err != nil {
		return nil, err
	}
	groups := make(map[string][]Row)
	for _, r := range collected {
		groups[r.Fingerprint] = append(groups[r.Fingerprint], r)
	}
	return groups, nil
}
