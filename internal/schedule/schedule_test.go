package schedule

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
)

func TestDue(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	interval := 12 * time.Hour

	cases := []struct {
		name    string
		lastRun time.Time
		everRan bool
		want    bool
	}{
		{"never ran", time.Time{}, false, true},
		{"ran eleven hours ago", now.Add(-11 * time.Hour), true, false},
		{"ran exactly the interval ago", now.Add(-12 * time.Hour), true, true},
		{"ran thirteen hours ago", now.Add(-13 * time.Hour), true, true},
		{"clock went backwards", now.Add(time.Hour), true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Due(now, c.lastRun, c.everRan, interval); got != c.want {
				t.Errorf("Due = %v, want %v", got, c.want)
			}
		})
	}
}

func watcherFor(t *testing.T, body string) *config.Watcher {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := config.NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	return w
}

const completeConfig = `sources:
  - olx
source_urls:
  olx:
    - https://www.olx.com.br/x
match:
  max_age_years: 10
  max_price_cents: 4500000
crawl:
  interval_hours: 12
`

func runOnce(t *testing.T, r *Runner) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunnerCollectsWhenTheIntervalHasPassed(t *testing.T) {
	collected := 0
	r := &Runner{
		Config:  watcherFor(t, completeConfig),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error { collected++; return nil },
		Now:     time.Now,
		Tick:    time.Millisecond,
		Warn:    io.Discard,
	}
	runOnce(t, r)
	if collected == 0 {
		t.Fatal("the runner never collected on a fresh database")
	}
}

func TestRunnerDoesNotCollectWhileTheConfigIsIncomplete(t *testing.T) {
	collected := 0
	r := &Runner{
		Config:  watcherFor(t, "sources:\n  - olx\n"),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error { collected++; return nil },
		Now:     time.Now,
		Tick:    time.Millisecond,
		Warn:    io.Discard,
	}
	runOnce(t, r)
	if collected != 0 {
		t.Errorf("the runner collected %d times with an incomplete config", collected)
	}
}

func TestRunnerCollectsWhenTheDatabaseCannotBeRead(t *testing.T) {
	collected := 0
	r := &Runner{
		Config:  watcherFor(t, completeConfig),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, errors.New("database is locked") },
		Collect: func(config.Config) error { collected++; return nil },
		Now:     time.Now,
		Tick:    time.Millisecond,
		Warn:    io.Discard,
	}
	runOnce(t, r)
	if collected == 0 {
		t.Fatal("a database error stopped the hunt instead of erring towards collecting")
	}
}

func TestRunnerDoesNotRetryEveryTickAfterAFailedRound(t *testing.T) {
	collected := 0
	r := &Runner{
		Config:  watcherFor(t, completeConfig),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error { collected++; return errors.New("chrome is gone") },
		Now:     time.Now,
		Tick:    time.Millisecond,
		Warn:    io.Discard,
	}
	runOnce(t, r)
	if collected != 1 {
		t.Errorf("collected %d times after a failure that recorded no round, want 1", collected)
	}
}

func TestRunnerDoesNotOverlapRounds(t *testing.T) {
	inFlight := 0
	overlapped := false
	r := &Runner{
		Config:  watcherFor(t, completeConfig),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error {
			inFlight++
			if inFlight > 1 {
				overlapped = true
			}
			time.Sleep(20 * time.Millisecond)
			inFlight--
			return nil
		},
		Now:  time.Now,
		Tick: time.Millisecond,
		Warn: io.Discard,
	}
	runOnce(t, r)
	if overlapped {
		t.Error("two rounds ran at the same time")
	}
}

func TestRunnerSaysWhyItIsNotCollectingOncePerReason(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("sources:\n  - olx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	watcher, err := config.NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	var warned bytes.Buffer
	r := &Runner{
		Config:  watcher,
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error { t.Error("collected with a config that cannot collect"); return nil },
		Now:     time.Now,
		Warn:    &warned,
	}

	r.step()
	r.step()
	r.step()
	if got := strings.Count(warned.String(), "\n"); got != 1 {
		t.Errorf("three ticks with the same broken config wrote %d lines, want 1:\n%s", got, warned.String())
	}
	if !strings.Contains(warned.String(), "max_age_years") {
		t.Errorf("the log does not say what the config is missing:\n%s", warned.String())
	}

	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	r.step()
	r.step()
	if got := strings.Count(warned.String(), "\n"); got != 2 {
		t.Errorf("a config truncated to nothing wrote %d lines in total, want 2:\n%s", got, warned.String())
	}
	if !strings.Contains(warned.String(), "no sources enabled") {
		t.Errorf("the log does not say the config lost its sources:\n%s", warned.String())
	}
}
