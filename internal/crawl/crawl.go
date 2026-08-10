package crawl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/fipe"
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

const DefaultAlertsPerRun = 5

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
	Expired       int
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
		result     SourceResult
		items      []model.RawListing
		started    time.Time
		finished   time.Time
		errMessage string
	}

	gathered := make([]fetched, len(sources))
	var (
		wg        sync.WaitGroup
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
			gathered[i] = fetched{
				result:     result,
				items:      items,
				started:    started,
				finished:   finished,
				errMessage: errMessage,
			}
		}(i, src)
	}
	wg.Wait()

	report := Report{}
	now := time.Now()
	for _, f := range gathered {
		report.Results = append(report.Results, f.result)
		stored := 0
		for _, raw := range f.items {
			listing := normalize.Normalize(raw)
			listing.Verdict, listing.VerdictReason = match.Evaluate(listing, cfg.Match)

			res, err := s.Upsert(listing, now)
			if err != nil {
				storeErrs = append(storeErrs, fmt.Errorf("storing %s/%s: %w", raw.Source, raw.ExternalID, err))
				continue
			}
			stored++
			if listing.Verdict != model.VerdictMatch {
				continue
			}
			if res.IsNew {
				report.NewMatches++
			}
		}
		if recErr := s.RecordRun(f.result.Source, f.started, f.finished, stored, f.result.Status, f.errMessage); recErr != nil {
			storeErrs = append(storeErrs, recErr)
		}
	}

	expired, err := s.ExpireUnseen(store.ExpiryRounds)
	if err != nil {
		storeErrs = append(storeErrs, err)
	}
	report.Expired = expired

	report.StoreFailures = len(storeErrs)
	if len(storeErrs) > 0 {
		report.StoreErr = storeErrs[0]
	}
	report.SharedCause = sharedCause(report.Results)
	return report, nil
}

func Notify(ctx context.Context, s *store.Store, n notify.Notifier, limit int, refs *fipe.Table) (int, error) {
	if limit <= 0 {
		limit = DefaultAlertsPerRun
	}
	pending, err := s.PendingAlerts()
	if err != nil {
		return 0, err
	}
	sort.SliceStable(pending, func(i, j int) bool {
		return urgency(pending[i], refs) > urgency(pending[j], refs)
	})

	sent := 0
	sightings := make(map[string]map[string]int, len(pending))
	alerts := make(map[string]int, len(pending))
	for _, row := range pending {
		key, dedupable := dedupKey(row)
		if dedupable {
			adsFromSource := recordSighting(sightings, key, row.Source)
			if adsFromSource <= alerts[key] {
				if err := s.MarkSilenced(row.ID, row.PriceCents); err != nil {
					return sent, err
				}
				continue
			}
		}
		if sent >= limit {
			break
		}
		message := notify.FormatAlert(row)
		if previous, ok := pendingDrop(row); ok {
			message = notify.FormatPriceDrop(row, previous)
		}
		if err := n.Send(ctx, notify.Alert{Message: message, URL: row.URL}); err != nil {
			return sent, fmt.Errorf("sending alert for listing %d: %w", row.ID, err)
		}
		if err := s.MarkNotified(row.ID, row.PriceCents); err != nil {
			return sent, err
		}
		if dedupable {
			alerts[key]++
		}
		sent++
	}
	return sent, nil
}

func pendingDrop(row store.Row) (int64, bool) {
	if row.PriceCents == nil || row.NotifiedPriceCents == nil {
		return 0, false
	}
	if *row.PriceCents >= *row.NotifiedPriceCents {
		return 0, false
	}
	return *row.NotifiedPriceCents, true
}

func urgency(row store.Row, refs *fipe.Table) float64 {
	if row.PriceCents == nil {
		return 0
	}
	if previous, ok := pendingDrop(row); ok {
		return float64(previous-*row.PriceCents) / float64(previous)
	}
	if row.Year == nil {
		return 0
	}
	ref, ok := refs.Lookup(row.Bike, row.Variant, *row.Year)
	if !ok || ref.PriceCents <= *row.PriceCents {
		return 0
	}
	return float64(ref.PriceCents-*row.PriceCents) / float64(ref.PriceCents)
}

func dedupKey(row store.Row) (string, bool) {
	if row.Fingerprint == "" || row.Km == nil {
		return "", false
	}
	return row.Fingerprint, true
}

func recordSighting(sightings map[string]map[string]int, key, source string) int {
	if sightings[key] == nil {
		sightings[key] = make(map[string]int)
	}
	sightings[key][source]++
	return sightings[key][source]
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
