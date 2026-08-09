package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/chromedp/chromedp"
)

const (
	defaultDevtoolsURL = "http://127.0.0.1:9222"
	browserSettleDelay = 3 * time.Second
	httpFetchTimeout   = 20 * time.Second
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"

type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]model.RawListing, error)
}

type PageFetcher interface {
	FetchPage(ctx context.Context, url string) (string, error)
}

type HTTPFetcher struct {
	client *http.Client
}

func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{client: &http.Client{Timeout: httpFetchTimeout}}
}

func (h *HTTPFetcher) FetchPage(ctx context.Context, url string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("building the request for %s: %w", url, err)
	}
	request.Header.Set("User-Agent", defaultUserAgent)
	request.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")

	response, err := h.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: the server answered %d %s", url, response.StatusCode, http.StatusText(response.StatusCode))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", url, err)
	}
	return string(body), nil
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
