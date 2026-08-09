package source

import (
	"context"
	"io"
	"strings"
)

func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}

type fakePageFetcher struct {
	page  string
	err   error
	asked []string
}

func (f *fakePageFetcher) FetchPage(ctx context.Context, url string) (string, error) {
	f.asked = append(f.asked, url)
	if f.err != nil {
		return "", f.err
	}
	return f.page, nil
}
