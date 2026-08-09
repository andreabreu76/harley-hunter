package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultDevtoolsURL = "http://127.0.0.1:9222"

type MatchCriteria struct {
	Years              []int `yaml:"years"`
	MaybeYears         []int `yaml:"maybe_years"`
	MaxPriceCents      int64 `yaml:"max_price_cents"`
	MaybeMaxPriceCents int64 `yaml:"maybe_max_price_cents"`
}

type CrawlSettings struct {
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
	if len(cfg.Sources) == 0 {
		return Config{}, fmt.Errorf("config has no sources enabled")
	}
	if len(cfg.Match.Years) == 0 {
		return Config{}, fmt.Errorf("config has no target years")
	}
	if cfg.Match.MaxPriceCents <= 0 {
		return Config{}, fmt.Errorf("config has no max price")
	}
	for _, name := range cfg.Sources {
		if len(cfg.SourceURLs[name]) == 0 {
			return Config{}, fmt.Errorf("source %q is enabled but has no urls under source_urls", name)
		}
	}
	if cfg.DevtoolsURL == "" {
		cfg.DevtoolsURL = defaultDevtoolsURL
	}
	return cfg, nil
}
