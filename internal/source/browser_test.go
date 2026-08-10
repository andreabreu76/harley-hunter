package source

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTab struct {
	mu         sync.Mutex
	loaded     []string
	failOn     string
	active     int
	maxActive  int
	hold       time.Duration
	id         string
	targets    []string
	closed     []string
	selfClosed bool
}

func (f *fakeTab) ID() string {
	if f.id == "" {
		return "own"
	}
	return f.id
}

func (f *fakeTab) PageTargetIDs(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.targets...), nil
}

func (f *fakeTab) CloseTargetByID(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, id)
	return nil
}

func (f *fakeTab) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.selfClosed = true
}

func (f *fakeTab) Load(_ context.Context, url string, _ time.Duration) (string, error) {
	f.mu.Lock()
	f.loaded = append(f.loaded, url)
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	failing := f.failOn == url
	f.mu.Unlock()

	time.Sleep(f.hold)

	f.mu.Lock()
	f.active--
	f.mu.Unlock()

	if failing {
		return "", errors.New("tab lost the page")
	}
	return "<html>" + url + "</html>", nil
}

func fetcherWithTab(tab *fakeTab) (*BrowserFetcher, *int) {
	opened := 0
	fetcher := NewBrowserFetcher("http://127.0.0.1:9222")
	fetcher.openTab = func(string) (browserTab, error) {
		opened++
		return tab, nil
	}
	return fetcher, &opened
}

func TestBrowserFetcherOpensOneTabForEveryPage(t *testing.T) {
	tab := &fakeTab{}
	fetcher, opened := fetcherWithTab(tab)

	for _, url := range []string{"https://test/a", "https://test/b", "https://test/c"} {
		if _, err := fetcher.FetchPage(context.Background(), url); err != nil {
			t.Fatalf("FetchPage(%s): %v", url, err)
		}
	}

	if *opened != 1 {
		t.Errorf("opened %d tabs, want 1: the tab has to live across pages", *opened)
	}
	if got, want := len(tab.loaded), 3; got != want {
		t.Errorf("loaded %d pages, want %d", got, want)
	}
}

func TestBrowserFetcherKeepsTheTabAfterAPageFails(t *testing.T) {
	tab := &fakeTab{failOn: "https://test/b"}
	fetcher, opened := fetcherWithTab(tab)

	if _, err := fetcher.FetchPage(context.Background(), "https://test/a"); err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if _, err := fetcher.FetchPage(context.Background(), "https://test/b"); err == nil {
		t.Fatal("FetchPage should report the failed page")
	}
	if _, err := fetcher.FetchPage(context.Background(), "https://test/c"); err != nil {
		t.Fatalf("FetchPage after a failure: %v", err)
	}

	if *opened != 1 {
		t.Errorf("opened %d tabs, want 1: a failed page must not cost the browser its tab", *opened)
	}
}

func TestBrowserFetcherReadsOnePageAtATime(t *testing.T) {
	tab := &fakeTab{hold: 20 * time.Millisecond}
	fetcher, _ := fetcherWithTab(tab)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fetcher.FetchPage(context.Background(), "https://test/page")
		}(i)
	}
	wg.Wait()

	tab.mu.Lock()
	defer tab.mu.Unlock()
	if tab.maxActive != 1 {
		t.Errorf("%d concurrent loads, want 1: one tab cannot serve two pages at once", tab.maxActive)
	}
	if got, want := len(tab.loaded), 6; got != want {
		t.Errorf("loaded %d pages, want %d", got, want)
	}
}

func TestBrowserFetcherReportsWhenTheTabCannotBeOpened(t *testing.T) {
	fetcher := NewBrowserFetcher("http://127.0.0.1:9222")
	fetcher.openTab = func(string) (browserTab, error) {
		return nil, errors.New("no browser is open")
	}

	_, err := fetcher.FetchPage(context.Background(), "https://test/a")
	if err == nil {
		t.Fatal("FetchPage should report that the tab could not be opened")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:9222") {
		t.Errorf("error = %q, want it to name the browser it could not use", err)
	}
}

func TestBrowserFetcherNamesTheURLWhenAPageFails(t *testing.T) {
	tab := &fakeTab{failOn: "https://test/a"}
	fetcher, _ := fetcherWithTab(tab)

	_, err := fetcher.FetchPage(context.Background(), "https://test/a")
	if err == nil {
		t.Fatal("FetchPage should report the failure")
	}
	if !strings.Contains(err.Error(), "https://test/a") {
		t.Errorf("error = %q, want it to name the url", err)
	}
}

func TestBrowserFetcherClosesTheTabsTheRoundOpened(t *testing.T) {
	tab := &fakeTab{targets: []string{"owner", "own"}}
	fetcher, _ := fetcherWithTab(tab)

	if _, err := fetcher.FetchPage(context.Background(), "https://test/a"); err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	tab.targets = []string{"owner", "own", "spawned-by-the-page", "another-spawn"}

	if err := fetcher.ReleaseTabs(context.Background()); err != nil {
		t.Fatalf("ReleaseTabs: %v", err)
	}

	if got, want := strings.Join(tab.closed, ","), "spawned-by-the-page,another-spawn"; got != want {
		t.Errorf("closed %q, want %q: only what the round opened", got, want)
	}
	if !tab.selfClosed {
		t.Error("the fetcher kept its own tab even though another tab was there to hold the browser open")
	}
}

func TestBrowserFetcherNeverClosesTheLastTab(t *testing.T) {
	tab := &fakeTab{targets: []string{"own"}}
	fetcher, _ := fetcherWithTab(tab)

	if _, err := fetcher.FetchPage(context.Background(), "https://test/a"); err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	tab.targets = []string{"own", "spawned-by-the-page"}

	if err := fetcher.ReleaseTabs(context.Background()); err != nil {
		t.Fatalf("ReleaseTabs: %v", err)
	}

	if got, want := strings.Join(tab.closed, ","), "spawned-by-the-page"; got != want {
		t.Errorf("closed %q, want %q", got, want)
	}
	if tab.selfClosed {
		t.Error("the fetcher closed the only tab left: the next round cannot open one")
	}
}

func TestBrowserFetcherLeavesAloneTheTabsItFound(t *testing.T) {
	tab := &fakeTab{targets: []string{"owner-instagram", "owner-facebook", "own"}}
	fetcher, _ := fetcherWithTab(tab)

	if _, err := fetcher.FetchPage(context.Background(), "https://test/a"); err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if err := fetcher.ReleaseTabs(context.Background()); err != nil {
		t.Fatalf("ReleaseTabs: %v", err)
	}

	if len(tab.closed) != 0 {
		t.Errorf("closed %q, want nothing: those tabs were open before the round", tab.closed)
	}
	if !tab.selfClosed {
		t.Error("the fetcher should hand its own tab back when the browser stays open without it")
	}
}

func TestBrowserFetcherReleaseWithoutAnyPageIsQuiet(t *testing.T) {
	tab := &fakeTab{targets: []string{"owner"}}
	fetcher, opened := fetcherWithTab(tab)

	if err := fetcher.ReleaseTabs(context.Background()); err != nil {
		t.Fatalf("ReleaseTabs: %v", err)
	}
	if *opened != 0 {
		t.Errorf("opened %d tabs, want 0: releasing must not open a browser it never used", *opened)
	}
}

func TestBrowserFetcherReturnsThePageItLoaded(t *testing.T) {
	tab := &fakeTab{}
	fetcher, _ := fetcherWithTab(tab)

	page, err := fetcher.FetchPage(context.Background(), "https://test/a")
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if want := "<html>https://test/a</html>"; page != want {
		t.Errorf("page = %q, want %q", page, want)
	}
}
