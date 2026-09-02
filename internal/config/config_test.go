package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, dir, databasePath string) string {
	t.Helper()
	body := fmt.Sprintf(`database_path: %s
sources:
  - olx
source_urls:
  olx:
    - https://www.olx.com.br/autos-e-pecas/motos/estado-pr?q=harley
match:
  max_age_years: 10
  max_price_cents: 4500000
`, databasePath)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

func TestLoadReadsCriteria(t *testing.T) {
	cfg, err := Load("testdata/config.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	want, err := filepath.Abs(filepath.Join("testdata", "hunter.db"))
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if cfg.DatabasePath != want {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
	if len(cfg.Sources) != 2 || cfg.Sources[0] != "olx" {
		t.Errorf("Sources = %v, want [olx mercadolivre]", cfg.Sources)
	}
	if cfg.Match.MaxPriceCents != 4500000 {
		t.Errorf("MaxPriceCents = %d, want 4500000", cfg.Match.MaxPriceCents)
	}
	if cfg.Match.MaybeMaxPriceCents != 5500000 {
		t.Errorf("MaybeMaxPriceCents = %d, want 5500000", cfg.Match.MaybeMaxPriceCents)
	}
	if cfg.Match.MaxAgeYears != 10 {
		t.Errorf("MaxAgeYears = %d, want 10", cfg.Match.MaxAgeYears)
	}
	if want := time.Now().Year() - 10; cfg.Match.MinYear != want {
		t.Errorf("MinYear = %d, want %d: the cut follows the financing rule, not a literal list", cfg.Match.MinYear, want)
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.yaml"); err == nil {
		t.Fatal("Load should return an error for a missing file")
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

func TestLoadResolvesARelativeDatabasePathAgainstTheConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "hunter.db")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !filepath.IsAbs(cfg.DatabasePath) {
		t.Fatalf("DatabasePath = %q, want an absolute path so the cwd cannot swap the database", cfg.DatabasePath)
	}
	want := filepath.Join(dir, "hunter.db")
	if cfg.DatabasePath != want {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}

func TestLoadKeepsAnAbsoluteDatabasePathUntouched(t *testing.T) {
	dir := t.TempDir()
	absolute := filepath.Join(dir, "elsewhere", "hunter.db")
	path := writeConfig(t, dir, absolute)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.DatabasePath != absolute {
		t.Errorf("DatabasePath = %q, want %q untouched", cfg.DatabasePath, absolute)
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

func TestLoadAcceptsAnIncompleteConfigSoTheDaemonCanStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("sources:\n  - olx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load on an incomplete config returned error: %v", err)
	}
	if want := filepath.Join(dir, "hunter.db"); cfg.DatabasePath != want {
		t.Errorf("DatabasePath = %q, want the default next to the config", cfg.DatabasePath)
	}
	if cfg.Crawl.IntervalHours != 12 {
		t.Errorf("IntervalHours = %d, want the default of 12", cfg.Crawl.IntervalHours)
	}
	if cfg.DevtoolsURL != "" {
		t.Errorf("DevtoolsURL = %q, want empty so the daemon manages Chrome", cfg.DevtoolsURL)
	}
}

func TestValidateRejectsWhatCannotCollect(t *testing.T) {
	base := Config{
		DatabasePath: "/tmp/hunter.db",
		Sources:      []string{"olx"},
		SourceURLs:   map[string][]string{"olx": {"https://www.olx.com.br/x"}},
		Match:        MatchCriteria{MaxAgeYears: 10, MinYear: 2016, MaxPriceCents: 4500000},
	}
	if err := Validate(base); err != nil {
		t.Fatalf("Validate on a complete config returned error: %v", err)
	}

	cases := []struct {
		name   string
		break_ func(*Config)
		want   string
	}{
		{"no source", func(c *Config) { c.Sources = nil }, "sources"},
		{"no max age", func(c *Config) { c.Match.MaxAgeYears = 0 }, "year"},
		{"no max price", func(c *Config) { c.Match.MaxPriceCents = 0 }, "price"},
		{"source without urls", func(c *Config) { c.SourceURLs = map[string][]string{} }, "olx"},
		{"no database path", func(c *Config) { c.DatabasePath = "" }, "database_path"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := base
			c.break_(&cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("Validate accepted a config with %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestEnsureFileWritesASkeletonThatLoadsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := EnsureFile(path); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load of the skeleton returned error: %v", err)
	}
	if len(cfg.Sources) != 6 {
		t.Errorf("skeleton enabled %d sources, want the six known ones", len(cfg.Sources))
	}
	if err := Validate(cfg); err == nil {
		t.Error("Validate accepted the skeleton, but a fresh install has no target yet")
	}
}

func TestEnsureFileLeavesAnExistingConfigAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("sources:\n  - olx\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureFile(path); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Errorf("EnsureFile rewrote an existing config:\n%s", after)
	}
}

func TestMinYearFollowsTheFinancingRule(t *testing.T) {
	cases := []struct {
		maxAge int
		now    time.Time
		want   int
	}{
		{10, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), 2016},
		{10, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), 2017},
		{0, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), 0},
	}
	for _, c := range cases {
		if got := minYearFor(c.maxAge, c.now); got != c.want {
			t.Errorf("minYearFor(%d, %s) = %d, want %d", c.maxAge, c.now.Format("2006-01-02"), got, c.want)
		}
	}
}
