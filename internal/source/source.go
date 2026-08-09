package source

import (
	"context"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"

type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]model.RawListing, error)
}
