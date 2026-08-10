package source

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

type pageParser func(body io.Reader) ([]model.RawListing, error)

type Delay func() time.Duration

func fetchPages(ctx context.Context, fetcher PageFetcher, urls []string, delay time.Duration, parse pageParser) ([]model.RawListing, error) {
	return FetchPages(ctx, fetcher, urls, func() time.Duration { return delay }, parse)
}

func FetchPages(ctx context.Context, fetcher PageFetcher, urls []string, delay Delay, parse func(body io.Reader) ([]model.RawListing, error)) ([]model.RawListing, error) {
	var all []model.RawListing
	for i, url := range urls {
		if err := ctx.Err(); err != nil {
			return all, err
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return all, ctx.Err()
			case <-time.After(delay()):
			}
		}

		page, err := fetcher.FetchPage(ctx, url)
		if err != nil {
			return all, err
		}

		listings, err := parse(strings.NewReader(page))
		all = append(all, listings...)
		if err != nil {
			return all, fmt.Errorf("parsing %s: %w", url, err)
		}
	}
	return all, nil
}
