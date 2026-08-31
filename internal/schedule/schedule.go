package schedule

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
)

const defaultTick = time.Minute

func Due(now, lastRun time.Time, everRan bool, interval time.Duration) bool {
	if !everRan {
		return true
	}
	return now.Sub(lastRun) >= interval
}

type Runner struct {
	Config      *config.Watcher
	LastRun     func() (time.Time, bool, error)
	Collect     func(config.Config) error
	Now         func() time.Time
	Tick        time.Duration
	Warn        io.Writer
	lastAttempt time.Time
}

func (r *Runner) Run(ctx context.Context) error {
	tick := r.Tick
	if tick <= 0 {
		tick = defaultTick
	}
	if r.Warn == nil {
		r.Warn = io.Discard
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.step()
		}
	}
}

func (r *Runner) step() {
	cfg := r.Config.Current()
	if err := config.Validate(cfg); err != nil {
		return
	}

	now := r.Now()
	lastRun, everRan, err := r.LastRun()
	if err != nil {
		fmt.Fprintf(r.Warn, "cannot tell when the last round started, collecting anyway: %v\n", err)
		lastRun, everRan = time.Time{}, false
	}
	if !r.lastAttempt.IsZero() && r.lastAttempt.After(lastRun) {
		lastRun, everRan = r.lastAttempt, true
	}

	interval := time.Duration(cfg.Crawl.IntervalHours) * time.Hour
	if !Due(now, lastRun, everRan, interval) {
		return
	}

	r.lastAttempt = now
	if err := r.Collect(cfg); err != nil {
		fmt.Fprintf(r.Warn, "round failed: %v\n", err)
	}
}
