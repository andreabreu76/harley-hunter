package config

import "testing"

func TestLoadReadsCriteria(t *testing.T) {
	cfg, err := Load("testdata/config.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.DatabasePath != "hunter.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "hunter.db")
	}
	if len(cfg.Sources) != 2 || cfg.Sources[0] != "olx" {
		t.Errorf("Sources = %v, want [olx mercadolivre]", cfg.Sources)
	}
	if cfg.Match.MaxPriceCents != 7500000 {
		t.Errorf("MaxPriceCents = %d, want 7500000", cfg.Match.MaxPriceCents)
	}
	if cfg.Match.MaybeMaxPriceCents != 8500000 {
		t.Errorf("MaybeMaxPriceCents = %d, want 8500000", cfg.Match.MaybeMaxPriceCents)
	}
	if len(cfg.Match.Years) != 2 || cfg.Match.Years[0] != 2014 {
		t.Errorf("Years = %v, want [2014 2015]", cfg.Match.Years)
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.yaml"); err == nil {
		t.Fatal("Load should return an error for a missing file")
	}
}

func TestLoadRejectsEmptyMatchCriteria(t *testing.T) {
	if _, err := Load("testdata/no-match.yaml"); err == nil {
		t.Fatal("Load should reject a config with no match criteria")
	}
}

func TestLoadReadsSourceURLsAndDevtoolsURL(t *testing.T) {
	cfg, err := Load("testdata/config.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.DevtoolsURL != "http://127.0.0.1:9222" {
		t.Errorf("DevtoolsURL = %q, want %q", cfg.DevtoolsURL, "http://127.0.0.1:9222")
	}
	if len(cfg.SourceURLs["olx"]) != 2 {
		t.Errorf("SourceURLs[olx] = %v, want 2 urls", cfg.SourceURLs["olx"])
	}
	if len(cfg.SourceURLs["mercadolivre"]) != 1 {
		t.Errorf("SourceURLs[mercadolivre] = %v, want 1 url", cfg.SourceURLs["mercadolivre"])
	}
}

func TestLoadRejectsEnabledSourceWithoutURLs(t *testing.T) {
	if _, err := Load("testdata/no-urls.yaml"); err == nil {
		t.Fatal("Load should reject an enabled source that has no urls: it would crawl nothing and report success")
	}
}

func TestLoadReadsTheAlertCapUnderItsCurrentName(t *testing.T) {
	cfg, err := Load("testdata/config.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Crawl.MaxAlertsPerRun != 5 {
		t.Errorf("MaxAlertsPerRun = %d, want 5 from max_alerts_per_run", cfg.Crawl.MaxAlertsPerRun)
	}
}
