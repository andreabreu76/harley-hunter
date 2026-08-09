package source

import (
	"context"
	"errors"
	"io"
	"strings"
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
