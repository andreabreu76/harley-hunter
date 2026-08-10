package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

type pageParser func(body io.Reader) ([]model.RawListing, error)

type Delay func() time.Duration

var ErrPartialPage = errors.New("the search did not fit in the page that was read")

func fetchPages(ctx context.Context, fetcher PageFetcher, urls []string, delay time.Duration, parse pageParser) ([]model.RawListing, error) {
	return FetchPages(ctx, fetcher, urls, func() time.Duration { return delay }, parse)
}

func FetchPages(ctx context.Context, fetcher PageFetcher, urls []string, delay Delay, parse func(body io.Reader) ([]model.RawListing, error)) ([]model.RawListing, error) {
	var all []model.RawListing
	var partial []error
	for i, url := range urls {
		if err := ctx.Err(); err != nil {
			return all, errors.Join(append(partial, err)...)
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return all, errors.Join(append(partial, ctx.Err())...)
			case <-time.After(delay()):
			}
		}

		page, err := fetcher.FetchPage(ctx, url)
		if err != nil {
			return all, errors.Join(append(partial, err)...)
		}

		listings, err := parse(strings.NewReader(page))
		all = append(all, listings...)
		if err == nil {
			continue
		}

		err = fmt.Errorf("parsing %s: %w", url, err)
		if !errors.Is(err, ErrPartialPage) {
			return all, errors.Join(append(partial, err)...)
		}
		partial = append(partial, err)
	}
	return all, errors.Join(partial...)
}
