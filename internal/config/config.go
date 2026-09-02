package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultIntervalHours = 12

type MatchCriteria struct {
	MaxAgeYears        int   `yaml:"max_age_years"`
	MaxPriceCents      int64 `yaml:"max_price_cents"`
	MaybeMaxPriceCents int64 `yaml:"maybe_max_price_cents"`
	MinYear            int   `yaml:"-"`
}

type CrawlSettings struct {
	IntervalHours   int `yaml:"interval_hours"`
	TimeoutSeconds  int `yaml:"timeout_seconds"`
	MaxConcurrent   int `yaml:"max_concurrent"`
	MaxAlertsPerRun int `yaml:"max_alerts_per_run"`
}

type Config struct {
	DatabasePath string              `yaml:"database_path"`
	Sources      []string            `yaml:"sources"`
	SourceURLs   map[string][]string `yaml:"source_urls"`
	DevtoolsURL  string              `yaml:"devtools_url"`
	Match        MatchCriteria       `yaml:"match"`
	Crawl        CrawlSettings       `yaml:"crawl"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Config{}, fmt.Errorf("resolving the config directory: %w", err)
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = filepath.Join(dir, "hunter.db")
	} else if !filepath.IsAbs(cfg.DatabasePath) {
		cfg.DatabasePath = filepath.Join(dir, cfg.DatabasePath)
	}
	if cfg.Crawl.IntervalHours <= 0 {
		cfg.Crawl.IntervalHours = defaultIntervalHours
	}
	cfg.Match.MinYear = minYearFor(cfg.Match.MaxAgeYears, time.Now())
	return cfg, nil
}

func minYearFor(maxAgeYears int, now time.Time) int {
	if maxAgeYears <= 0 {
		return 0
	}
	return now.Year() - maxAgeYears
}

func Validate(cfg Config) error {
	if cfg.DatabasePath == "" {
		return fmt.Errorf("config has no database_path: sqlite would open a throwaway database and every round would re-notify the same listings")
	}
	if len(cfg.Sources) == 0 {
		return fmt.Errorf("config has no sources enabled")
	}
	if cfg.Match.MaxAgeYears <= 0 {
		return fmt.Errorf("config has no max_age_years: without it there is no oldest model year the financing still takes")
	}
	if cfg.Match.MaxPriceCents <= 0 {
		return fmt.Errorf("config has no max price")
	}
	for _, name := range cfg.Sources {
		if len(cfg.SourceURLs[name]) == 0 {
			return fmt.Errorf("source %q is enabled but has no urls under source_urls", name)
		}
	}
	return nil
}
