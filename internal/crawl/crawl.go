package crawl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/match"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
	"github.com/andreabreu76/harley-hunter/internal/notify"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

const (
	StatusOK    = "ok"
	StatusError = "error"
)

const (
	HealthOK      = "ok"
	HealthSuspect = "suspect"
	HealthUnknown = "unknown"
)

const (
	healthWindow        = 5
	emptyRunsForSuspect = 2
	minVolumeForSuspect = 3
	defaultTimeout      = 90 * time.Second
	defaultConcurrency  = 4
)

const DefaultSMSPerRun = 5

const pendingOversample = 3

type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]model.RawListing, error)
}

type SourceResult struct {
	Source    string
	ItemCount int
	Status    string
	Err       error
}

type Report struct {
	Results       []SourceResult
	NewMatches    int
	StoreFailures int
	StoreErr      error
	SharedCause   error
}

func Run(ctx context.Context, sources []Source, s *store.Store, cfg config.Config) (Report, error) {
	timeout := time.Duration(cfg.Crawl.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	concurrency := cfg.Crawl.MaxConcurrent
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	type fetched struct {
		result SourceResult
		items  []model.RawListing
	}

	gathered := make([]fetched, len(sources))
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		storeErrs []error
		sem       = make(chan struct{}, concurrency)
	)

	for i, src := range sources {
		wg.Add(1)
		go func(i int, src Source) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sourceCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			started := time.Now()
			items, err := fetchSafely(sourceCtx, src)
			finished := time.Now()

			result := SourceResult{Source: src.Name(), ItemCount: len(items), Status: StatusOK, Err: err}
			errMessage := ""
			if err != nil {
				result.Status = StatusError
				errMessage = err.Error()
			}
			if recErr := s.RecordRun(src.Name(), started, finished, len(items), result.Status, errMessage); recErr != nil {
				mu.Lock()
				storeErrs = append(storeErrs, recErr)
				mu.Unlock()
			}
			gathered[i] = fetched{result: result, items: items}
		}(i, src)
	}
	wg.Wait()

	report := Report{}
	now := time.Now()
	for _, f := range gathered {
		report.Results = append(report.Results, f.result)
		for _, raw := range f.items {
			listing := normalize.Normalize(raw)
			listing.Verdict, listing.VerdictReason = match.Evaluate(listing, cfg.Match)

			res, err := s.Upsert(listing, now)
			if err != nil {
				storeErrs = append(storeErrs, fmt.Errorf("storing %s/%s: %w", raw.Source, raw.ExternalID, err))
				continue
			}
			if res.IsNew && listing.Verdict == model.VerdictMatch {
				report.NewMatches++
			}
		}
	}

	report.StoreFailures = len(storeErrs)
	if len(storeErrs) > 0 {
		report.StoreErr = storeErrs[0]
	}
	report.SharedCause = sharedCause(report.Results)
	return report, nil
}

func Notify(ctx context.Context, s *store.Store, n notify.Notifier, limit int) (int, error) {
	if limit <= 0 {
		limit = DefaultSMSPerRun
	}
	pending, err := s.PendingNotifications(limit * pendingOversample)
	if err != nil {
		return 0, err
	}

	sent := 0
	seen := make(map[string]bool, len(pending))
	for _, row := range pending {
		key, dedupable := dedupKey(row)
		if dedupable && seen[key] {
			if err := s.MarkNotified(row.ID); err != nil {
				return sent, err
			}
			continue
		}
		if sent >= limit {
			break
		}
		if err := n.Send(ctx, notify.FormatAlert(row)); err != nil {
			return sent, fmt.Errorf("sending alert for listing %d: %w", row.ID, err)
		}
		if err := s.MarkNotified(row.ID); err != nil {
			return sent, err
		}
		if dedupable {
			seen[key] = true
		}
		sent++
	}
	return sent, nil
}

func dedupKey(row store.Row) (string, bool) {
	if row.Fingerprint == "" || row.Km == nil {
		return "", false
	}
	return row.Fingerprint, true
}

func fetchSafely(ctx context.Context, src Source) (items []model.RawListing, err error) {
	defer func() {
		if r := recover(); r != nil {
			items = nil
			err = fmt.Errorf("source panicked: %v", r)
		}
	}()
	return src.Fetch(ctx)
}

func sharedCause(results []SourceResult) error {
	if len(results) < 2 {
		return nil
	}
	var cause error
	for _, r := range results {
		if r.Err == nil {
			return nil
		}
		root := rootCause(r.Err)
		if cause == nil {
			cause = root
			continue
		}
		if root.Error() != cause.Error() {
			return nil
		}
	}
	return cause
}

func rootCause(err error) error {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
}

func HealthStatus(counts []int) string {
	if len(counts) == 0 {
		return HealthUnknown
	}
	recent := counts
	if len(recent) > healthWindow {
		recent = recent[:healthWindow]
	}

	empty := 0
	for _, c := range recent {
		if c != 0 {
			break
		}
		empty++
	}
	if empty < emptyRunsForSuspect {
		return HealthOK
	}

	total, productive := 0, 0
	for _, c := range counts {
		if c > 0 {
			total += c
			productive++
		}
	}
	if productive == 0 || total/productive < minVolumeForSuspect {
		return HealthOK
	}
	return HealthSuspect
}
