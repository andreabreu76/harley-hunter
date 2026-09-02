package fipe

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

const quoteURL = "https://fipe.parallelum.com.br/api/v2/motorcycles/brands/77/models/"

type PageFetcher interface {
	FetchPage(ctx context.Context, url string) (string, error)
}

type Store interface {
	FipeFetchedAt() (map[string]time.Time, error)
	SaveFipeReference(Reference, time.Time) error
}

var outageOnce sync.Once

func Refresh(ctx context.Context, fetcher PageFetcher, s Store, now time.Time, minYear int) (int, error) {
	fetched, err := s.FipeFetchedAt()
	if err != nil {
		reportOutage("fipe: reading the stored references: %v", err)
		return 0, nil
	}

	stored := 0
	years := watchedYears(minYear, now)
	for _, ref := range models {
		for _, year := range years {
			if ctx.Err() != nil {
				return stored, nil
			}
			if !stale(fetched[key(ref.Bike, ref.Variant, year)], now) {
				continue
			}
			body, err := fetcher.FetchPage(ctx, quoteURL+ref.ModelCode+"/years/"+strconv.Itoa(year)+"-1")
			if err != nil {
				reportOutage("fipe: %v", err)
				continue
			}
			quote, err := ParseQuote(strings.NewReader(body))
			if err != nil {
				continue
			}
			reference := Reference{
				Code:       quote.Code,
				Label:      ref.Label,
				Bike:       ref.Bike,
				Variant:    ref.Variant,
				Year:       year,
				PriceCents: quote.PriceCents,
				Month:      quote.Month,
			}
			if err := s.SaveFipeReference(reference, now); err != nil {
				reportOutage("fipe: %v", err)
				continue
			}
			stored++
		}
	}
	return stored, nil
}

func watchedYears(minYear int, now time.Time) []int {
	if minYear <= 0 {
		return nil
	}
	var years []int
	for year := minYear; year <= now.Year(); year++ {
		years = append(years, year)
	}
	return years
}

func stale(fetchedAt, now time.Time) bool {
	if fetchedAt.IsZero() {
		return true
	}
	at, current := fetchedAt.UTC(), now.UTC()
	return at.Year() != current.Year() || at.Month() != current.Month()
}

func reportOutage(format string, args ...any) {
	outageOnce.Do(func() { log.Printf(format, args...) })
}
