package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
)

const (
	webmotorsRequestDelay = 4 * time.Second
	webmotorsListingBase  = "https://www.webmotors.com.br/comprar/"
	webmotorsImageBase    = "https://image.webmotors.com.br/_fotos/anunciousados/gigante/"
)

var (
	webmotorsSlugNoise    = regexp.MustCompile(`[^a-z0-9]+`)
	webmotorsStateInParen = regexp.MustCompile(`\(([A-Za-z]{2})\)\s*$`)
)

type Webmotors struct {
	fetcher  PageFetcher
	baseURLs []string
	delay    time.Duration
}

func NewWebmotors(fetcher PageFetcher, baseURLs []string) *Webmotors {
	if fetcher == nil {
		fetcher = NewBrowserFetcher("")
	}
	return &Webmotors{fetcher: fetcher, baseURLs: baseURLs, delay: webmotorsRequestDelay}
}

func (w *Webmotors) Name() string { return model.SourceWebmotors }

func (w *Webmotors) Fetch(ctx context.Context) ([]model.RawListing, error) {
	var all []model.RawListing
	for i, url := range w.baseURLs {
		if err := ctx.Err(); err != nil {
			return all, err
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return all, ctx.Err()
			case <-time.After(w.delay):
			}
		}

		listings, err := w.fetchOne(ctx, url)
		if err != nil {
			return all, err
		}
		all = append(all, listings...)
	}
	return all, nil
}

func (w *Webmotors) fetchOne(ctx context.Context, url string) ([]model.RawListing, error) {
	page, err := w.fetcher.FetchPage(ctx, url)
	if err != nil {
		return nil, err
	}

	listings, err := ParseWebmotors(strings.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", url, err)
	}
	return listings, nil
}

type webmotorsResponse struct {
	SearchResults *[]webmotorsResult `json:"SearchResults"`
}

type webmotorsResult struct {
	UniqueID      int64  `json:"UniqueId"`
	PhotoPath     string `json:"PhotoPath"`
	LongComment   string `json:"LongComment"`
	Specification struct {
		Title string `json:"Title"`
		Make  struct {
			Value string `json:"Value"`
		} `json:"Make"`
		Model struct {
			Value string `json:"Value"`
		} `json:"Model"`
		YearFabrication string  `json:"YearFabrication"`
		YearModel       float64 `json:"YearModel"`
		Odometer        float64 `json:"Odometer"`
		CubicCentimeter float64 `json:"CubicCentimeter"`
	} `json:"Specification"`
	Seller struct {
		City  string `json:"City"`
		State string `json:"State"`
	} `json:"Seller"`
	Prices struct {
		Price float64 `json:"Price"`
	} `json:"Prices"`
}

func ParseWebmotors(body io.Reader) ([]model.RawListing, error) {
	payload, err := webmotorsPayload(body)
	if err != nil {
		return nil, err
	}

	var response webmotorsResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, fmt.Errorf("webmotors response is not the search json: %w", err)
	}
	if response.SearchResults == nil {
		return nil, errors.New("webmotors search results not found: response structure changed or request was blocked")
	}

	results := *response.SearchResults
	listings := make([]model.RawListing, 0, len(results))
	for _, r := range results {
		title := strings.TrimSpace(r.Specification.Title)
		url := webmotorsListingURL(r)
		if url == "" || title == "" {
			continue
		}
		listings = append(listings, model.RawListing{
			Source:       model.SourceWebmotors,
			ExternalID:   strconv.FormatInt(r.UniqueID, 10),
			URL:          url,
			Title:        title,
			RawText:      strings.TrimSpace(r.LongComment),
			PriceText:    webmotorsPrice(r.Prices.Price),
			YearText:     webmotorsYears(r),
			KmText:       webmotorsKm(r.Specification.Odometer),
			LocationText: webmotorsLocation(r),
			ImageURL:     webmotorsImage(r.PhotoPath),
		})
	}

	if len(results) > 0 && len(listings) == 0 {
		return nil, fmt.Errorf("webmotors read %d results and no listing: result payload changed", len(results))
	}
	return listings, nil
}

func webmotorsPayload(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed, nil
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("reading html: %w", err)
	}
	text := strings.TrimSpace(doc.Find("pre").First().Text())
	if text == "" {
		return nil, errors.New("webmotors json body not found: response structure changed or request was blocked")
	}
	return []byte(text), nil
}

func webmotorsListingURL(r webmotorsResult) string {
	brand := webmotorsSlug(r.Specification.Make.Value)
	name := webmotorsSlug(r.Specification.Model.Value)
	years := webmotorsYearPath(r)
	if r.UniqueID == 0 || brand == "" || name == "" || years == "" || r.Specification.CubicCentimeter <= 0 {
		return ""
	}
	displacement := strconv.FormatInt(int64(r.Specification.CubicCentimeter), 10) + "cc"
	return webmotorsListingBase + strings.Join([]string{
		brand, name, displacement, years, strconv.FormatInt(r.UniqueID, 10),
	}, "/")
}

func webmotorsSlug(s string) string {
	folded := normalize.Fold(s)
	return strings.Trim(webmotorsSlugNoise.ReplaceAllString(folded, "-"), "-")
}

func webmotorsYearPath(r webmotorsResult) string {
	fabrication, listed := webmotorsYearPair(r)
	if fabrication == "" {
		return ""
	}
	if listed == "" || listed == fabrication {
		return fabrication
	}
	return fabrication + "-" + listed
}

func webmotorsYears(r webmotorsResult) string {
	fabrication, listed := webmotorsYearPair(r)
	if fabrication == "" {
		return listed
	}
	if listed == "" {
		return fabrication
	}
	return fabrication + "/" + listed
}

func webmotorsYearPair(r webmotorsResult) (string, string) {
	fabrication := strings.TrimSpace(r.Specification.YearFabrication)
	listed := ""
	if r.Specification.YearModel > 0 {
		listed = strconv.FormatInt(int64(r.Specification.YearModel), 10)
	}
	return fabrication, listed
}

func webmotorsPrice(price float64) string {
	if price <= 0 {
		return ""
	}
	return "R$ " + strconv.FormatFloat(price, 'f', -1, 64)
}

func webmotorsKm(odometer float64) string {
	if odometer < 0 {
		return ""
	}
	return strconv.FormatFloat(odometer, 'f', -1, 64) + " km"
}

func webmotorsLocation(r webmotorsResult) string {
	city := strings.TrimSpace(r.Seller.City)
	state := strings.TrimSpace(r.Seller.State)
	if m := webmotorsStateInParen.FindStringSubmatch(state); m != nil {
		state = strings.ToUpper(m[1])
	}
	if city == "" {
		return state
	}
	if state == "" {
		return city
	}
	return city + " - " + state
}

func webmotorsImage(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, "/"))
	if path == "" {
		return ""
	}
	return webmotorsImageBase + path
}
