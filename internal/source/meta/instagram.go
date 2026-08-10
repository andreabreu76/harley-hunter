package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
	"github.com/andreabreu76/harley-hunter/internal/source"
)

const (
	instagramHost      = "https://www.instagram.com"
	instagramGrid      = `main[role="main"]`
	instagramLoginForm = "form#login_form"
	instagramMaxPosts  = 12
	instagramMinDelay  = 8 * time.Second
	instagramMaxDelay  = 20 * time.Second
)

var (
	instagramPostPath = regexp.MustCompile(`^/(?:p|reel)/([^/?#]+)`)
	priceMark         = regexp.MustCompile(`(?i)r\$\s*\d`)
	saleTerms         = []string{"vendo", "vende-se", "a venda", "disponivel", "aceito troca", "aceito proposta"}
	soldTerms         = regexp.MustCompile(`\bvendid[oa]s?\b`)
)

type Instagram struct {
	fetcher source.PageFetcher
	urls    []string
	delay   source.Delay
}

func NewInstagram(fetcher source.PageFetcher, urls []string) *Instagram {
	return &Instagram{
		fetcher: fetcher,
		urls:    urls,
		delay:   randomDelay(instagramMinDelay, instagramMaxDelay),
	}
}

func (i *Instagram) Name() string { return model.SourceInstagram }

func (i *Instagram) Fetch(ctx context.Context) ([]model.RawListing, error) {
	return source.FetchPages(ctx, i.fetcher, i.urls, i.delay, ParseInstagram)
}

func randomDelay(min, max time.Duration) source.Delay {
	return func() time.Duration {
		return min + time.Duration(rand.Int63n(int64(max-min+1)))
	}
}

func ParseInstagram(body io.Reader) ([]model.RawListing, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("reading html: %w", err)
	}

	if doc.Find(instagramLoginForm).Length() > 0 {
		return nil, errors.New("instagram answered with the login screen: the browser profile is not signed in")
	}

	grid := doc.Find(instagramGrid).First()
	if grid.Length() == 0 {
		return nil, errors.New("instagram grid container not found: the page changed, or instagram served a checkpoint")
	}

	var listings []model.RawListing
	seen := make(map[string]bool)
	grid.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		shortcode := instagramShortcode(href)
		if shortcode == "" || seen[shortcode] {
			return true
		}

		img := a.Find("img").First()
		caption := strings.TrimSpace(img.AttrOr("alt", ""))
		if caption == "" || !hasSaleSignal(caption) || isSold(caption) {
			return true
		}

		seen[shortcode] = true
		listings = append(listings, model.RawListing{
			Source:     model.SourceInstagram,
			ExternalID: shortcode,
			URL:        instagramHost + href,
			RawText:    caption,
			ImageURL:   img.AttrOr("src", ""),
		})
		return len(listings) < instagramMaxPosts
	})

	return listings, nil
}

func instagramShortcode(href string) string {
	match := instagramPostPath.FindStringSubmatch(href)
	if match == nil {
		return ""
	}
	return match[1]
}

func isSold(caption string) bool {
	return soldTerms.MatchString(normalize.Fold(caption))
}

func hasSaleSignal(caption string) bool {
	if priceMark.MatchString(caption) {
		return true
	}
	folded := normalize.Fold(caption)
	for _, term := range saleTerms {
		if strings.Contains(folded, term) {
			return true
		}
	}
	return false
}
