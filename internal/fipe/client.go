package fipe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const fetchTimeout = 20 * time.Second

type httpFetcher struct {
	client *http.Client
}

func NewHTTPFetcher() PageFetcher {
	return &httpFetcher{client: &http.Client{Timeout: fetchTimeout}}
}

func (h *httpFetcher) FetchPage(ctx context.Context, url string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("building the fipe request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := h.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetching the fipe quote: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("reading the fipe quote: %w", err)
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return "", fmt.Errorf("fipe answered %d %s", response.StatusCode, http.StatusText(response.StatusCode))
	}
	return string(body), nil
}
