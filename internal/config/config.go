package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type MatchCriteria struct {
	Years              []int `yaml:"years"`
	MaybeYears         []int `yaml:"maybe_years"`
	MaxPriceCents      int64 `yaml:"max_price_cents"`
	MaybeMaxPriceCents int64 `yaml:"maybe_max_price_cents"`
}

type CrawlSettings struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`
	MaxConcurrent  int `yaml:"max_concurrent"`
	MaxSMSPerRun   int `yaml:"max_sms_per_run"`
}

type Config struct {
	DatabasePath string        `yaml:"database_path"`
	Sources      []string      `yaml:"sources"`
	Match        MatchCriteria `yaml:"match"`
	Crawl        CrawlSettings `yaml:"crawl"`
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
	return cfg, nil
}
