package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const skeleton = `sources:
  - olx
  - mercadolivre
  - webmotors
  - mobiauto
  - instagram
  - marketplace
source_urls: {}
match:
  max_age_years: 0
  max_price_cents: 0
  maybe_max_price_cents: 0
crawl:
  interval_hours: 12
  timeout_seconds: 240
  max_concurrent: 4
  max_alerts_per_run: 5
`

func EnsureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking the config file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating the config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(skeleton), 0o644); err != nil {
		return fmt.Errorf("writing the first config: %w", err)
	}
	return nil
}
