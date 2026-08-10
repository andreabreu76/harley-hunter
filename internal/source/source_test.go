package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}

type fakePageFetcher struct {
	page   string
	err    error
	failOn string
	asked  []string
}

func (f *fakePageFetcher) FetchPage(ctx context.Context, url string) (string, error) {
	f.asked = append(f.asked, url)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if f.err != nil {
		return "", f.err
	}
	if f.failOn == url {
		return "", errors.New("browser lost the tab for " + url)
	}
	return f.page, nil
}

func TestHTTPFetcherStopsReadingAnEndlessBody(t *testing.T) {
	previous := maxPageBytes
	maxPageBytes = 64
	t.Cleanup(func() { maxPageBytes = previous })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", 5000))
	}))
	defer server.Close()

	page, err := NewHTTPFetcher().FetchPage(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if int64(len(page)) != maxPageBytes {
		t.Errorf("len(page) = %d, want it capped at %d", len(page), maxPageBytes)
	}
}
