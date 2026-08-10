package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
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

type browserTab interface {
	Load(ctx context.Context, url string, settle time.Duration) (string, error)
	ID() string
	PageTargetIDs(ctx context.Context) ([]string, error)
	CloseTargetByID(ctx context.Context, id string) error
	Close()
}

type BrowserFetcher struct {
	devtoolsURL string
	settle      time.Duration
	openTab     func(devtoolsURL string) (browserTab, error)
	mu          sync.Mutex
	tab         browserTab
	found       map[string]bool
}

func NewBrowserFetcher(devtoolsURL string) *BrowserFetcher {
	if devtoolsURL == "" {
		devtoolsURL = defaultDevtoolsURL
	}
	return &BrowserFetcher{devtoolsURL: devtoolsURL, settle: browserSettleDelay, openTab: openChromeTab}
}

func (b *BrowserFetcher) FetchPage(ctx context.Context, url string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.tab == nil {
		tab, err := b.openTab(b.devtoolsURL)
		if err != nil {
			return "", fmt.Errorf("opening a tab on the browser at %s: %w", b.devtoolsURL, err)
		}
		b.tab = tab
		b.found = tabsAlreadyOpen(ctx, tab)
	}

	html, err := b.tab.Load(ctx, url, b.settle)
	if err != nil {
		return "", fmt.Errorf("fetching %s through the browser: %w", url, err)
	}
	return html, nil
}

func tabsAlreadyOpen(ctx context.Context, tab browserTab) map[string]bool {
	found := make(map[string]bool)
	ids, err := tab.PageTargetIDs(ctx)
	if err != nil {
		return found
	}
	for _, id := range ids {
		if id != tab.ID() {
			found[id] = true
		}
	}
	return found
}

func (b *BrowserFetcher) ReleaseTabs(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.tab == nil {
		return nil
	}

	ids, err := b.tab.PageTargetIDs(ctx)
	if err != nil {
		return fmt.Errorf("listing the tabs of the browser at %s: %w", b.devtoolsURL, err)
	}

	own, held := b.tab.ID(), 0
	var failed error
	for _, id := range ids {
		switch {
		case id == own:
		case b.found[id]:
			held++
		default:
			if closeErr := b.tab.CloseTargetByID(ctx, id); closeErr != nil && failed == nil {
				failed = fmt.Errorf("closing the tab %s the round opened: %w", id, closeErr)
			}
		}
	}

	if held > 0 {
		b.tab.Close()
		b.tab = nil
		b.found = nil
	}
	return failed
}

type chromeTab struct {
	ctx         context.Context
	cancelTab   context.CancelFunc
	cancelAlloc context.CancelFunc
	id          string
}

func openChromeTab(devtoolsURL string) (browserTab, error) {
	id, err := newDevtoolsTab(devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), devtoolsURL)
	tabCtx, cancelTab := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(id)))
	if err := chromedp.Run(tabCtx); err != nil {
		cancelTab()
		cancelAlloc()
		return nil, err
	}
	return &chromeTab{
		ctx:         tabCtx,
		cancelTab:   cancelTab,
		cancelAlloc: cancelAlloc,
		id:          id,
	}, nil
}

func newDevtoolsTab(devtoolsURL string) (string, error) {
	endpoint := strings.TrimRight(devtoolsURL, "/") + "/json/new?about:blank"
	request, err := http.NewRequest(http.MethodPut, endpoint, nil)
	if err != nil {
		return "", err
	}

	response, err := (&http.Client{Timeout: httpFetchTimeout}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the browser answered %d %s", response.StatusCode, http.StatusText(response.StatusCode))
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", errors.New("the browser opened a tab without an id")
	}
	return created.ID, nil
}

func (t *chromeTab) ID() string { return t.id }

func (t *chromeTab) PageTargetIDs(context.Context) ([]string, error) {
	infos, err := chromedp.Targets(t.ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, info := range infos {
		if info.Type == "page" {
			ids = append(ids, info.TargetID.String())
		}
	}
	return ids, nil
}

func (t *chromeTab) CloseTargetByID(_ context.Context, id string) error {
	return chromedp.Run(t.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return target.CloseTarget(target.ID(id)).Do(cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Browser))
	}))
}

func (t *chromeTab) Close() {
	t.CloseTargetByID(context.Background(), t.id)
	t.cancelTab()
	t.cancelAlloc()
}

func (t *chromeTab) Load(ctx context.Context, url string, settle time.Duration) (string, error) {
	runCtx, cancelRun := context.WithCancel(t.ctx)
	defer cancelRun()
	defer context.AfterFunc(ctx, cancelRun)()

	var html string
	if err := chromedp.Run(runCtx,
		chromedp.Navigate(url),
		chromedp.Sleep(settle),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		if callerErr := ctx.Err(); callerErr != nil {
			return "", callerErr
		}
		return "", err
	}
	return html, nil
}
