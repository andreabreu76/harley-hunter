package crawl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
	_ "modernc.org/sqlite"
)

type fakeSource struct {
	name  string
	items []model.RawListing
	err   error
}

func (f fakeSource) Name() string { return f.name }

func (f fakeSource) Fetch(ctx context.Context) ([]model.RawListing, error) {
	return f.items, f.err
}

type funcSource struct {
	name  string
	fetch func(ctx context.Context) ([]model.RawListing, error)
}

func (f funcSource) Name() string { return f.name }

func (f funcSource) Fetch(ctx context.Context) ([]model.RawListing, error) {
	return f.fetch(ctx)
}

func testConfig() config.Config {
	return config.Config{
		Match: config.MatchCriteria{
			MaxAgeYears:        10,
			MinYear:            2016,
			MaxPriceCents:      4500000,
			MaybeMaxPriceCents: 5500000,
		},
		Crawl: config.CrawlSettings{TimeoutSeconds: 5, MaxConcurrent: 2, MaxAlertsPerRun: 5},
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "crawl.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func harley(source, id string) model.RawListing {
	return model.RawListing{
		Source:       source,
		ExternalID:   id,
		URL:          "https://example.com/" + id,
		Title:        "Harley Davidson Iron 1200",
		PriceText:    "R$ 43.000",
		YearText:     "2019",
		KmText:       "12.000 km",
		LocationText: "Curitiba - PR",
	}
}

func TestRunStoresMatchesAndSurvivesFailingSource(t *testing.T) {
	s := openStore(t)
	good := fakeSource{name: "good", items: []model.RawListing{harley("good", "1")}}
	broken := fakeSource{name: "broken", err: errors.New("blocked")}

	report, err := Run(context.Background(), []Source{good, broken}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 source results, got %d", len(report.Results))
	}
	if report.NewMatches != 1 {
		t.Errorf("NewMatches = %d, want 1", report.NewMatches)
	}

	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 stored match, got %d", len(rows))
	}

	counts, err := s.RecentRunCounts("broken", 5)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	if len(counts) != 1 {
		t.Errorf("failing source should still record a run, got %d", len(counts))
	}
}

func TestRunKeepsResultsInSourceOrder(t *testing.T) {
	s := openStore(t)
	slow := funcSource{name: "slow", fetch: func(ctx context.Context) ([]model.RawListing, error) {
		time.Sleep(30 * time.Millisecond)
		return nil, nil
	}}
	quick := fakeSource{name: "quick"}

	report, err := Run(context.Background(), []Source{slow, quick}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}
	if report.Results[0].Source != "slow" || report.Results[1].Source != "quick" {
		t.Errorf("results = [%s %s], want [slow quick]", report.Results[0].Source, report.Results[1].Source)
	}
}

func TestRunKeepsPartialItemsFromFailingSource(t *testing.T) {
	s := openStore(t)
	partial := fakeSource{
		name:  "partial",
		items: []model.RawListing{harley("partial", "1")},
		err:   errors.New("third url blocked"),
	}

	report, err := Run(context.Background(), []Source{partial}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.NewMatches != 1 {
		t.Errorf("NewMatches = %d, want 1: partial results must survive the error", report.NewMatches)
	}
	if report.Results[0].Status != StatusError {
		t.Errorf("Status = %q, want %q", report.Results[0].Status, StatusError)
	}
	if report.Results[0].ItemCount != 1 {
		t.Errorf("ItemCount = %d, want 1", report.Results[0].ItemCount)
	}
}

func TestRunCountsRepeatedListingOnce(t *testing.T) {
	s := openStore(t)
	repeated := fakeSource{
		name:  "repeated",
		items: []model.RawListing{harley("repeated", "1"), harley("repeated", "1")},
	}

	report, err := Run(context.Background(), []Source{repeated}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.NewMatches != 1 {
		t.Errorf("NewMatches = %d, want 1: the same ad seen twice in a round is one match", report.NewMatches)
	}
	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("stored rows = %d, want 1", len(rows))
	}
}

func TestRunSurvivesPanickingSource(t *testing.T) {
	s := openStore(t)
	exploding := funcSource{name: "exploding", fetch: func(ctx context.Context) ([]model.RawListing, error) {
		panic("index out of range")
	}}
	good := fakeSource{name: "good", items: []model.RawListing{harley("good", "1")}}

	report, err := Run(context.Background(), []Source{exploding, good}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}
	if report.Results[0].Err == nil {
		t.Error("a panicking source must be reported as a failure, not as an empty success")
	}
	if report.NewMatches != 1 {
		t.Errorf("NewMatches = %d, want 1: a panic must not abort the round", report.NewMatches)
	}

	counts, err := s.RecentRunCounts("exploding", 5)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	if len(counts) != 1 {
		t.Errorf("a panicking source should still record a run, got %d", len(counts))
	}
}

func TestRunGivesEachSourceItsOwnDeadline(t *testing.T) {
	s := openStore(t)
	var deadlines []time.Time
	var mu sync.Mutex
	record := func(name string) Source {
		return funcSource{name: name, fetch: func(ctx context.Context) ([]model.RawListing, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Errorf("%s: fetch context has no deadline", name)
			}
			mu.Lock()
			deadlines = append(deadlines, deadline)
			mu.Unlock()
			return nil, nil
		}}
	}

	before := time.Now()
	if _, err := Run(context.Background(), []Source{record("a"), record("b")}, s, testConfig()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(deadlines) != 2 {
		t.Fatalf("expected 2 deadlines, got %d", len(deadlines))
	}
	want := 5 * time.Second
	for _, d := range deadlines {
		if got := d.Sub(before); got < want-time.Second || got > want+time.Second {
			t.Errorf("deadline is %v away, want about %v", got, want)
		}
	}
}

func TestRunRespectsMaxConcurrent(t *testing.T) {
	s := openStore(t)
	var mu sync.Mutex
	running, peak := 0, 0
	busy := func(name string) Source {
		return funcSource{name: name, fetch: func(ctx context.Context) ([]model.RawListing, error) {
			mu.Lock()
			running++
			if running > peak {
				peak = running
			}
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
			return nil, nil
		}}
	}

	sources := []Source{busy("a"), busy("b"), busy("c"), busy("d"), busy("e")}
	if _, err := Run(context.Background(), sources, s, testConfig()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if peak > 2 {
		t.Errorf("peak concurrency = %d, want at most 2", peak)
	}
}

func TestRunReportsSharedCauseWhenEverySourceFailsAlike(t *testing.T) {
	s := openStore(t)
	refused := "dial tcp 127.0.0.1:9222: connect: connection refused"
	a := fakeSource{name: "a", err: fmt.Errorf("fetching https://a/1 through the browser: %w", errors.New(refused))}
	b := fakeSource{name: "b", err: fmt.Errorf("fetching https://b/2 through the browser: %w", errors.New(refused))}

	report, err := Run(context.Background(), []Source{a, b}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.SharedCause == nil {
		t.Fatal("every source failing for the same reason must be reported as one shared cause")
	}
	if report.SharedCause.Error() != refused {
		t.Errorf("SharedCause = %q, want %q", report.SharedCause, refused)
	}
}

func TestRunHasNoSharedCauseWhenCausesDiffer(t *testing.T) {
	s := openStore(t)
	a := fakeSource{name: "a", err: errors.New("olx payload not found")}
	b := fakeSource{name: "b", err: errors.New("mercadolivre layout changed")}

	report, err := Run(context.Background(), []Source{a, b}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.SharedCause != nil {
		t.Errorf("SharedCause = %v, want nil: two unrelated failures are not one cause", report.SharedCause)
	}
}

func TestRunHasNoSharedCauseWhenOneSourceSucceeds(t *testing.T) {
	s := openStore(t)
	refused := errors.New("connection refused")
	a := fakeSource{name: "a", err: fmt.Errorf("fetching: %w", refused)}
	b := fakeSource{name: "b", items: []model.RawListing{harley("b", "1")}}

	report, err := Run(context.Background(), []Source{a, b}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.SharedCause != nil {
		t.Errorf("SharedCause = %v, want nil: a working source rules out a shared cause", report.SharedCause)
	}
}

func TestRunHasNoSharedCauseWithASingleSource(t *testing.T) {
	s := openStore(t)
	only := fakeSource{name: "only", err: errors.New("connection refused")}

	report, err := Run(context.Background(), []Source{only}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.SharedCause != nil {
		t.Errorf("SharedCause = %v, want nil: one source cannot share a cause", report.SharedCause)
	}
}

func TestRunReportsStoreFailuresInsteadOfSwallowingThem(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	s.Close()

	good := fakeSource{name: "good", items: []model.RawListing{harley("good", "1")}}
	report, err := Run(context.Background(), []Source{good}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.StoreFailures == 0 {
		t.Error("a failing store must be reported, not silently skipped")
	}
	if report.StoreErr == nil {
		t.Error("StoreErr must carry the first storage failure")
	}
	if report.NewMatches != 0 {
		t.Errorf("NewMatches = %d, want 0: nothing was stored", report.NewMatches)
	}
}

func TestHealthStatus(t *testing.T) {
	cases := []struct {
		name   string
		counts []int
		want   string
	}{
		{"healthy", []int{8, 7, 9}, "ok"},
		{"two empty runs after productive history", []int{0, 0, 9, 8, 7}, "suspect"},
		{"one empty run is tolerated", []int{0, 9, 8, 7, 6}, "ok"},
		{"low volume source is not suspect", []int{0, 0, 1, 2, 0}, "ok"},
		{"no history", nil, "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HealthStatus(c.counts); got != c.want {
				t.Errorf("HealthStatus(%v) = %q, want %q", c.counts, got, c.want)
			}
		})
	}
}

func TestHealthStatusStaysSuspectWhileTheWindowIsAllEmpty(t *testing.T) {
	counts := []int{0, 0, 0, 0, 0, 9, 9, 9}
	if got := HealthStatus(counts); got != "suspect" {
		t.Errorf("HealthStatus(%v) = %q, want %q: a source broken for longer must not read healthier", counts, got, "suspect")
	}
}

func TestHealthStatusStaysQuietWithoutVolumeEvidence(t *testing.T) {
	counts := []int{0, 0, 0, 0, 0}
	if got := HealthStatus(counts); got != "ok" {
		t.Errorf("HealthStatus(%v) = %q, want %q: no run ever produced volume, so there is nothing to compare against", counts, got, "ok")
	}
}

func TestRunDoesNotCallARoundProductiveWhenNothingCouldBeStored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "refuse.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	side, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening the second connection: %v", err)
	}
	defer side.Close()
	if _, err := side.Exec(
		`CREATE TRIGGER refuse_listings BEFORE INSERT ON listings
         BEGIN SELECT RAISE(ABORT, 'the listings table is refusing writes'); END`); err != nil {
		t.Fatalf("installing the trigger: %v", err)
	}

	only := fakeSource{name: "olx", items: []model.RawListing{harley("olx", "1"), harley("olx", "2")}}
	report, err := Run(context.Background(), []Source{only}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.StoreFailures == 0 {
		t.Fatal("the refused writes must be reported")
	}

	counts, err := s.RecentRunCounts("olx", 5)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	if len(counts) != 1 {
		t.Fatalf("counts = %v, want one recorded run", counts)
	}
	if counts[0] != 0 {
		t.Errorf("recorded item_count = %d, want 0: a round that stored nothing is not a round that saw the ads, and three of them expire the source", counts[0])
	}
}

func TestRunRecordsWhatItActuallyStored(t *testing.T) {
	s := openStore(t)
	only := fakeSource{name: "olx", items: []model.RawListing{harley("olx", "1"), harley("olx", "2")}}

	if _, err := Run(context.Background(), []Source{only}, s, testConfig()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	counts, err := s.RecentRunCounts("olx", 5)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	if len(counts) != 1 || counts[0] != 2 {
		t.Errorf("counts = %v, want [2]", counts)
	}
}
