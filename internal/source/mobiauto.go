package source

import (
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
)

const (
	mobiautoRequestDelay = 2 * time.Second
	mobiautoPayloadID    = "script#__NEXT_DATA__"
	mobiautoImageBase    = "https://image1.mobiauto.com.br/images/api/images/v1.0/"
	mobiautoImageSuffix  = "/transform/fl_progressive,f_webp,q_70,w_640"
)

var mobiautoDetailPath = regexp.MustCompile(`/detalhes/(\d+)`)

type Mobiauto struct {
	fetcher  PageFetcher
	baseURLs []string
	delay    time.Duration
}

func NewMobiauto(fetcher PageFetcher, baseURLs []string) *Mobiauto {
	if fetcher == nil {
		fetcher = NewHTTPFetcher()
	}
	return &Mobiauto{fetcher: fetcher, baseURLs: baseURLs, delay: mobiautoRequestDelay}
}

func (m *Mobiauto) Name() string { return model.SourceMobiauto }

func (m *Mobiauto) Fetch(ctx context.Context) ([]model.RawListing, error) {
	return fetchPages(ctx, m.fetcher, m.baseURLs, m.delay, ParseMobiauto)
}

type mobiautoPayload struct {
	Props struct {
		PageProps struct {
			Deals *struct {
				NumResults int               `json:"numResults"`
				Results    *[]mobiautoResult `json:"results"`
			} `json:"deals"`
		} `json:"pageProps"`
	} `json:"props"`
}

type mobiautoResult struct {
	ID     int64 `json:"id"`
	Price  int64 `json:"price"`
	Km     *int  `json:"km"`
	Images []struct {
		ImageID int64 `json:"imageId"`
	} `json:"images"`
	Trim struct {
		Name string `json:"name"`
		Make struct {
			Name string `json:"name"`
		} `json:"make"`
		Model struct {
			Name string `json:"name"`
			Year int    `json:"year"`
		} `json:"model"`
		Bodystyle struct {
			Name string `json:"name"`
		} `json:"bodystyle"`
		ProductionYear int `json:"productionYear"`
	} `json:"trim"`
	Dealer struct {
		Location struct {
			City  string `json:"city"`
			State string `json:"state"`
		} `json:"location"`
	} `json:"dealer"`
}

func ParseMobiauto(body io.Reader) ([]model.RawListing, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("reading html: %w", err)
	}

	raw := strings.TrimSpace(doc.Find(mobiautoPayloadID).First().Text())
	if raw == "" {
		return nil, errors.New("mobiauto payload not found: page structure changed or request was blocked")
	}

	var payload mobiautoPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("mobiauto payload is not the search json: %w", err)
	}

	deals := payload.Props.PageProps.Deals
	if deals == nil || deals.Results == nil {
		return nil, errors.New("mobiauto search results not found: payload structure changed")
	}

	urls := mobiautoDetailURLs(doc)
	results := *deals.Results
	listings := make([]model.RawListing, 0, len(results))
	for _, r := range results {
		id := strconv.FormatInt(r.ID, 10)
		url := urls[id]
		title := mobiautoTitle(r)
		if r.ID == 0 || url == "" || title == "" {
			continue
		}
		listings = append(listings, model.RawListing{
			Source:       model.SourceMobiauto,
			ExternalID:   id,
			URL:          url,
			Title:        title,
			RawText:      mobiautoDetails(r),
			PriceText:    mobiautoPrice(r.Price),
			YearText:     mobiautoYears(r),
			KmText:       mobiautoKm(r.Km),
			LocationText: mobiautoLocation(r),
			ImageURL:     mobiautoImage(r),
		})
	}

	if len(results) > 0 && len(listings) == 0 {
		return nil, fmt.Errorf("mobiauto read %d results and no listing: result payload changed", len(results))
	}
	if deals.NumResults > len(results) {
		return listings, fmt.Errorf("%w: mobiauto search has %d results and only %d fit in the page that was read",
			ErrPartialPage, deals.NumResults, len(results))
	}
	return listings, nil
}

func mobiautoDetailURLs(doc *goquery.Document) map[string]string {
	urls := make(map[string]string)
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		match := mobiautoDetailPath.FindStringSubmatch(href)
		if match == nil {
			return
		}
		if _, taken := urls[match[1]]; taken {
			return
		}
		urls[match[1]] = mobiautoCanonicalURL(href)
	})
	return urls
}

func mobiautoCanonicalURL(href string) string {
	if at := strings.IndexAny(href, "?#"); at >= 0 {
		href = href[:at]
	}
	return href
}

func mobiautoTitle(r mobiautoResult) string {
	parts := []string{r.Trim.Make.Name, r.Trim.Model.Name}
	if trim := strings.TrimSpace(r.Trim.Name); trim != "" && !strings.EqualFold(trim, strings.TrimSpace(r.Trim.Model.Name)) {
		parts = append(parts, trim)
	}
	return joinNonEmpty(parts)
}

func mobiautoDetails(r mobiautoResult) string {
	return joinNonEmpty([]string{r.Trim.Name, r.Trim.Bodystyle.Name})
}

func joinNonEmpty(parts []string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " ")
}

func mobiautoPrice(price int64) string {
	if price <= 0 {
		return ""
	}
	return "R$ " + strconv.FormatInt(price, 10)
}

func mobiautoYears(r mobiautoResult) string {
	production := r.Trim.ProductionYear
	listed := r.Trim.Model.Year
	switch {
	case production <= 0 && listed <= 0:
		return ""
	case production <= 0:
		return strconv.Itoa(listed)
	case listed <= 0:
		return strconv.Itoa(production)
	default:
		return strconv.Itoa(production) + "/" + strconv.Itoa(listed)
	}
}

func mobiautoKm(km *int) string {
	if km == nil {
		return ""
	}
	return strconv.Itoa(*km) + " km"
}

func mobiautoLocation(r mobiautoResult) string {
	city := strings.TrimSpace(r.Dealer.Location.City)
	state := strings.ToUpper(strings.TrimSpace(r.Dealer.Location.State))
	if city == "" {
		return state
	}
	if state == "" {
		return city
	}
	return city + " - " + state
}

func mobiautoImage(r mobiautoResult) string {
	if len(r.Images) == 0 || r.Images[0].ImageID == 0 {
		return ""
	}
	return mobiautoImageBase + strconv.FormatInt(r.Images[0].ImageID, 10) + mobiautoImageSuffix
}
