package source

import (
	"context"
	"fmt"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/chromedp/chromedp"
)

const (
	defaultDevtoolsURL = "http://127.0.0.1:9222"
	browserSettleDelay = 3 * time.Second
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"

type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]model.RawListing, error)
}

type PageFetcher interface {
	FetchPage(ctx context.Context, url string) (string, error)
}

type BrowserFetcher struct {
	devtoolsURL string
	settle      time.Duration
}

func NewBrowserFetcher(devtoolsURL string) *BrowserFetcher {
	if devtoolsURL == "" {
		devtoolsURL = defaultDevtoolsURL
	}
	return &BrowserFetcher{devtoolsURL: devtoolsURL, settle: browserSettleDelay}
}

func (b *BrowserFetcher) FetchPage(ctx context.Context, url string) (string, error) {
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, b.devtoolsURL)
	defer cancelAlloc()

	tabCtx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()

	var html string
	err := chromedp.Run(tabCtx,
		chromedp.Navigate(url),
		chromedp.Sleep(b.settle),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return "", fmt.Errorf("fetching %s through the browser: %w", url, err)
	}
	return html, nil
}
