package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andreabreu76/harley-hunter/internal/model"
)

const (
	mlCardSelector      = "li.ui-search-layout__item"
	mlEmptySelector     = ".ui-search-rescue--zrp"
	mlTitleSelector     = "a.poly-component__title"
	mlFractionSelector  = ".poly-price__current .andes-money-amount__fraction"
	mlCentsSelector     = ".poly-price__current .andes-money-amount__cents"
	mlAttributeSelector = ".poly-attributes_list__item"
	mlLocationSelector  = ".poly-component__location"
	mlImageSelector     = "img.poly-component__picture"
	mlRequestDelay      = 2 * time.Second
)

var mlItemID = regexp.MustCompile(`ML[A-Z]{1,2}-?\d{6,}`)

type MercadoLivre struct {
	fetcher  PageFetcher
	baseURLs []string
	delay    time.Duration
}

func NewMercadoLivre(fetcher PageFetcher, baseURLs []string) *MercadoLivre {
	if fetcher == nil {
		fetcher = NewBrowserFetcher("")
	}
	return &MercadoLivre{fetcher: fetcher, baseURLs: baseURLs, delay: mlRequestDelay}
}

func (m *MercadoLivre) Name() string { return "mercadolivre" }

func (m *MercadoLivre) Fetch(ctx context.Context) ([]model.RawListing, error) {
	var all []model.RawListing
	for i, url := range m.baseURLs {
		if err := ctx.Err(); err != nil {
			return all, err
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return all, ctx.Err()
			case <-time.After(m.delay):
			}
		}

		listings, err := m.fetchOne(ctx, url)
		if err != nil {
			return all, err
		}
		all = append(all, listings...)
	}
	return all, nil
}

func (m *MercadoLivre) fetchOne(ctx context.Context, url string) ([]model.RawListing, error) {
	page, err := m.fetcher.FetchPage(ctx, url)
	if err != nil {
		return nil, err
	}

	listings, err := ParseMercadoLivre(strings.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", url, err)
	}
	return listings, nil
}

func ParseMercadoLivre(body io.Reader) ([]model.RawListing, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("reading html: %w", err)
	}

	cards := doc.Find(mlCardSelector)
	if cards.Length() == 0 {
		if doc.Find(mlEmptySelector).Length() > 0 {
			return []model.RawListing{}, nil
		}
		return nil, errors.New("mercadolivre result list not found: page structure changed or request was blocked")
	}

	listings := make([]model.RawListing, 0, cards.Length())
	cards.Each(func(_ int, card *goquery.Selection) {
		link := card.Find(mlTitleSelector).First()
		href, _ := link.Attr("href")
		href = strings.TrimSpace(href)
		title := strings.TrimSpace(link.Text())
		if href == "" || title == "" {
			return
		}

		id := mlItemID.FindString(href)
		if id == "" {
			return
		}

		attributes := mlAttributes(card)
		image, _ := card.Find(mlImageSelector).First().Attr("src")

		listings = append(listings, model.RawListing{
			Source:       "mercadolivre",
			ExternalID:   strings.ReplaceAll(id, "-", ""),
			URL:          mlCanonicalURL(href),
			Title:        title,
			RawText:      attributes,
			PriceText:    mlPrice(card),
			YearText:     attributes,
			KmText:       attributes,
			LocationText: strings.TrimSpace(card.Find(mlLocationSelector).First().Text()),
			ImageURL:     strings.TrimSpace(image),
		})
	})

	if len(listings) == 0 {
		return nil, fmt.Errorf("mercadolivre read %d cards and no listing: card markup changed", cards.Length())
	}
	return listings, nil
}

func mlCanonicalURL(href string) string {
	if at := strings.Index(href, "#"); at >= 0 {
		return href[:at]
	}
	return href
}

func mlAttributes(card *goquery.Selection) string {
	var parts []string
	card.Find(mlAttributeSelector).Each(func(_ int, a *goquery.Selection) {
		if text := strings.TrimSpace(a.Text()); text != "" {
			parts = append(parts, text)
		}
	})
	return strings.Join(parts, " ")
}

func mlPrice(card *goquery.Selection) string {
	fraction := strings.TrimSpace(card.Find(mlFractionSelector).First().Text())
	if fraction == "" {
		return ""
	}
	if cents := strings.TrimSpace(card.Find(mlCentsSelector).First().Text()); cents != "" {
		return "R$ " + fraction + "," + cents
	}
	return "R$ " + fraction
}
