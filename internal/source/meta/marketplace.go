package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/source"
)

const (
	facebookHost       = "https://www.facebook.com"
	marketplaceChrome  = `a[href="/marketplace/create/"]`
	marketplaceCards   = `a[href^="/marketplace/item/"]`
	marketplaceTexts   = `span[dir="auto"]`
	marketplaceLogin   = "form#login_form"
	marketplaceItemURL = facebookHost + "/marketplace/item/"
)

var (
	marketplaceItemPath = regexp.MustCompile(`^/marketplace/item/(\d+)`)
	marketplacePrice    = regexp.MustCompile(`^R\$`)
	marketplaceCityUF   = regexp.MustCompile(`,\s*[A-Z]{2}$`)
)

type Marketplace struct {
	fetcher source.PageFetcher
	urls    []string
	delay   source.Delay
}

func NewMarketplace(fetcher source.PageFetcher, urls []string) *Marketplace {
	return &Marketplace{
		fetcher: fetcher,
		urls:    urls,
		delay:   randomDelay(instagramMinDelay, instagramMaxDelay),
	}
}

func (m *Marketplace) Name() string { return model.SourceMarketplace }

func (m *Marketplace) Fetch(ctx context.Context) ([]model.RawListing, error) {
	return source.FetchPages(ctx, m.fetcher, m.urls, m.delay, ParseMarketplace)
}

func ParseMarketplace(body io.Reader) ([]model.RawListing, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("reading html: %w", err)
	}

	if doc.Find(marketplaceChrome).Length() == 0 {
		if doc.Find(marketplaceLogin).Length() > 0 {
			return nil, errors.New("facebook answered with the login screen instead of marketplace results")
		}
		return nil, errors.New("marketplace results container not found: the page changed, or facebook served a checkpoint")
	}

	cards := doc.Find(marketplaceCards)
	var listings []model.RawListing
	seen := make(map[string]bool)
	cards.Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		match := marketplaceItemPath.FindStringSubmatch(href)
		if match == nil || seen[match[1]] {
			return
		}

		price, title, location := marketplaceFields(a)
		if title == "" {
			return
		}

		seen[match[1]] = true
		listings = append(listings, model.RawListing{
			Source:       model.SourceMarketplace,
			ExternalID:   match[1],
			URL:          marketplaceItemURL + match[1] + "/",
			Title:        title,
			PriceText:    price,
			LocationText: location,
			ImageURL:     a.Find("img").First().AttrOr("src", ""),
		})
	})

	if cards.Length() > 0 && len(listings) == 0 {
		return nil, fmt.Errorf("marketplace read %d cards and no listing: the card layout changed", cards.Length())
	}
	return listings, nil
}

func marketplaceFields(card *goquery.Selection) (price, title, location string) {
	var texts []string
	card.Find(marketplaceTexts).Each(func(_ int, span *goquery.Selection) {
		if text := strings.TrimSpace(span.Text()); text != "" {
			texts = append(texts, text)
		}
	})

	for _, text := range texts {
		switch {
		case price == "" && marketplacePrice.MatchString(text):
			price = text
		case location == "" && marketplaceCityUF.MatchString(text):
			location = text
		case title == "":
			title = text
		}
	}
	return price, title, location
}
