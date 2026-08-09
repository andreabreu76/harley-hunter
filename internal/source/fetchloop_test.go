package source

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func oneListing(id string) []model.RawListing {
	return []model.RawListing{{Source: "test", ExternalID: id}}
}

func parserReturning(listings []model.RawListing, err error) pageParser {
	return func(io.Reader) ([]model.RawListing, error) { return listings, err }
}

func TestFetchPagesCollectsEveryURLInOrder(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page"}
	urls := []string{"https://test/a", "https://test/b", "https://test/c"}

	listings, err := fetchPages(context.Background(), fetcher, urls, 0, parserReturning(oneListing("x"), nil))
	if err != nil {
		t.Fatalf("fetchPages: %v", err)
	}
	if got, want := len(listings), 3; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	if !slices.Equal(fetcher.asked, urls) {
		t.Errorf("asked for %q, want %q", fetcher.asked, urls)
	}
}

func TestFetchPagesHandsTheBodyToTheParser(t *testing.T) {
	fetcher := &fakePageFetcher{page: "the body"}
	var seen string

	_, err := fetchPages(context.Background(), fetcher, []string{"https://test/a"}, 0,
		func(body io.Reader) ([]model.RawListing, error) {
			raw, readErr := io.ReadAll(body)
			seen = string(raw)
			return oneListing("x"), readErr
		})
	if err != nil {
		t.Fatalf("fetchPages: %v", err)
	}
	if seen != "the body" {
		t.Errorf("parser saw %q, want the fetched page", seen)
	}
}

func TestFetchPagesChecksTheContextBeforeTheFirstFetch(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	listings, err := fetchPages(ctx, fetcher, []string{"https://test/a", "https://test/b"}, 0, parserReturning(oneListing("x"), nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked for %q, want no request at all on a cancelled context", fetcher.asked)
	}
	if len(listings) != 0 {
		t.Errorf("len(listings) = %d, want 0", len(listings))
	}
}

func TestFetchPagesWaitsBetweenURLs(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page"}
	delay := 40 * time.Millisecond

	started := time.Now()
	if _, err := fetchPages(context.Background(), fetcher, []string{"https://test/a", "https://test/b", "https://test/c"}, delay, parserReturning(oneListing("x"), nil)); err != nil {
		t.Fatalf("fetchPages: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 2*delay {
		t.Errorf("elapsed = %v, want at least %v: the delay between urls is gone", elapsed, 2*delay)
	}
}

func TestFetchPagesDoesNotWaitBeforeTheFirstURL(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page"}

	started := time.Now()
	if _, err := fetchPages(context.Background(), fetcher, []string{"https://test/a"}, time.Hour, parserReturning(oneListing("x"), nil)); err != nil {
		t.Fatalf("fetchPages: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("elapsed = %v, want no wait before the only url", elapsed)
	}
}

func TestFetchPagesStopsWaitingWhenTheContextIsCancelled(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page"}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	listings, err := fetchPages(ctx, fetcher, []string{"https://test/a", "https://test/b"}, time.Hour, parserReturning(oneListing("x"), nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("elapsed = %v: the wait ignored the cancelled context", elapsed)
	}
	if got, want := len(listings), 1; got != want {
		t.Errorf("len(listings) = %d, want %d: the first url's listings survive the cancellation", got, want)
	}
}

func TestFetchPagesKeepsWhatItGatheredWhenAFetchFails(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page", failOn: "https://test/b"}

	listings, err := fetchPages(context.Background(), fetcher, []string{"https://test/a", "https://test/b", "https://test/c"}, 0, parserReturning(oneListing("x"), nil))
	if err == nil {
		t.Fatal("fetchPages should report the fetch failure")
	}
	if got, want := len(listings), 1; got != want {
		t.Fatalf("len(listings) = %d, want %d", got, want)
	}
	if len(fetcher.asked) != 2 {
		t.Errorf("asked for %q, want it to stop at the failing url", fetcher.asked)
	}
}

func TestFetchPagesNamesTheURLWhenTheParserFails(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page"}
	boom := errors.New("payload not found")

	_, err := fetchPages(context.Background(), fetcher, []string{"https://test/a"}, 0, parserReturning(nil, boom))
	if err == nil {
		t.Fatal("fetchPages should report the parser failure")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error %v does not wrap the parser error", err)
	}
	if !strings.Contains(err.Error(), "https://test/a") {
		t.Errorf("error %q does not name the url", err)
	}
}

func TestFetchPagesKeepsThePartialListingsAParserReturnsWithItsError(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page"}
	overflow := errors.New("search spans 3 pages and only page 1 is read")

	listings, err := fetchPages(context.Background(), fetcher, []string{"https://test/a", "https://test/b"}, 0,
		parserReturning(oneListing("partial"), overflow))
	if !errors.Is(err, overflow) {
		t.Fatalf("error = %v, want the overflow error", err)
	}
	if got, want := len(listings), 1; got != want {
		t.Fatalf("len(listings) = %d, want %d: a parser that returns rows with its error keeps them", got, want)
	}
	if got, want := listings[0].ExternalID, "partial"; got != want {
		t.Errorf("ExternalID = %q, want %q", got, want)
	}
	if len(fetcher.asked) != 1 {
		t.Errorf("asked for %q, want it to stop at the first url", fetcher.asked)
	}
}

func TestFetchPagesReturnsNothingForNoURLs(t *testing.T) {
	fetcher := &fakePageFetcher{page: "page"}

	listings, err := fetchPages(context.Background(), fetcher, nil, 0, parserReturning(oneListing("x"), nil))
	if err != nil {
		t.Fatalf("fetchPages: %v", err)
	}
	if len(listings) != 0 {
		t.Errorf("len(listings) = %d, want 0", len(listings))
	}
	if len(fetcher.asked) != 0 {
		t.Errorf("asked for %q, want no request", fetcher.asked)
	}
}
