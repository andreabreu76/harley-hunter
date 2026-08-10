package store

import (
	"database/sql"
	"fmt"
)

var addedColumns = []struct {
	table  string
	column string
	ddl    string
}{
	{"listings", "phone", "ALTER TABLE listings ADD COLUMN phone TEXT"},
	{"listings", "published_at", "ALTER TABLE listings ADD COLUMN published_at TIMESTAMP"},
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
