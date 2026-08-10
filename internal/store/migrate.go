package store

import (
	"database/sql"
	"fmt"
)

var addedColumns = []struct {
	table  string
	column string
	ddl    string
	seed   string
}{
	{table: "listings", column: "phone", ddl: "ALTER TABLE listings ADD COLUMN phone TEXT"},
	{table: "listings", column: "published_at", ddl: "ALTER TABLE listings ADD COLUMN published_at TIMESTAMP"},
	{
		table:  "listings",
		column: "notified_price_cents",
		ddl:    "ALTER TABLE listings ADD COLUMN notified_price_cents INTEGER",
		seed:   "UPDATE listings SET notified_price_cents = price_cents WHERE notified = 1 AND price_cents IS NOT NULL",
	},
}

func addMissingColumns(db *sql.DB) error {
	for _, c := range addedColumns {
		present, err := hasColumn(db, c.table, c.column)
		if err != nil {
			return err
		}
		if present {
			continue
		}
		if _, err := db.Exec(c.ddl); err != nil {
			return fmt.Errorf("adding column %s.%s: %w", c.table, c.column, err)
		}
		if c.seed == "" {
			continue
		}
		if _, err := db.Exec(c.seed); err != nil {
			return fmt.Errorf("seeding column %s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return false, fmt.Errorf("reading columns of %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, fmt.Errorf("scanning column of %s: %w", table, err)
		}
		if name == column {
			return true, rows.Err()
		}
	}
	return false, rows.Err()
}
