package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

const (
	olxFlightMarker = "self.__next_f.push("
	olxAdsMarker    = `"ads":[`
	olxRequestDelay = 2 * time.Second
)

type OLX struct {
	fetcher  PageFetcher
	baseURLs []string
	delay    time.Duration
}

func NewOLX(fetcher PageFetcher, baseURLs []string) *OLX {
	if fetcher == nil {
		fetcher = NewBrowserFetcher("")
	}
	return &OLX{fetcher: fetcher, baseURLs: baseURLs, delay: olxRequestDelay}
}

func (o *OLX) Name() string { return "olx" }

func (o *OLX) Fetch(ctx context.Context) ([]model.RawListing, error) {
	var all []model.RawListing
	for i, url := range o.baseURLs {
		if i > 0 {
			select {
			case <-ctx.Done():
				return all, ctx.Err()
			case <-time.After(o.delay):
			}
		}

		listings, err := o.fetchOne(ctx, url)
		if err != nil {
			return nil, err
		}
		all = append(all, listings...)
	}
	return all, nil
}

func (o *OLX) fetchOne(ctx context.Context, url string) ([]model.RawListing, error) {
	page, err := o.fetcher.FetchPage(ctx, url)
	if err != nil {
		return nil, err
	}

	listings, err := ParseOLX(strings.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", url, err)
	}
	return listings, nil
}

type olxAd struct {
	ListID          json.Number `json:"listId"`
	Subject         string      `json:"subject"`
	URL             string      `json:"url"`
	Price           string      `json:"price"`
	Location        string      `json:"location"`
	LocationDetails struct {
		Municipality string `json:"municipality"`
		UF           string `json:"uf"`
	} `json:"locationDetails"`
	Images []struct {
		Original string `json:"original"`
	} `json:"images"`
	Properties []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"properties"`
}

func ParseOLX(body io.Reader) ([]model.RawListing, error) {
	page, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading html: %w", err)
	}

	flight := olxFlightPayload(string(page))
	if flight == "" {
		return nil, errors.New("olx payload not found: page structure changed or request was blocked")
	}

	ads, ok := olxAds(flight)
	if !ok {
		return nil, errors.New("olx ads array not found: page structure changed")
	}

	listings := make([]model.RawListing, 0, len(ads))
	for _, ad := range ads {
		if ad.ListID.String() == "" || ad.URL == "" {
			continue
		}
		listings = append(listings, model.RawListing{
			Source:       "olx",
			ExternalID:   ad.ListID.String(),
			URL:          ad.URL,
			Title:        ad.Subject,
			RawText:      olxProperty(ad, "vehicle_model"),
			PriceText:    ad.Price,
			YearText:     olxProperty(ad, "regdate"),
			KmText:       olxProperty(ad, "mileage"),
			LocationText: olxLocation(ad),
			ImageURL:     olxImage(ad),
		})
	}
	return listings, nil
}

func olxFlightPayload(page string) string {
	var payload strings.Builder
	for offset := 0; ; {
		found := strings.Index(page[offset:], olxFlightMarker)
		if found < 0 {
			break
		}
		offset += found + len(olxFlightMarker)

		var chunk []json.RawMessage
		if err := json.NewDecoder(strings.NewReader(page[offset:])).Decode(&chunk); err != nil || len(chunk) < 2 {
			continue
		}
		var text string
		if err := json.Unmarshal(chunk[1], &text); err != nil {
			continue
		}
		payload.WriteString(text)
	}
	return payload.String()
}

func olxAds(flight string) ([]olxAd, bool) {
	var found bool
	for offset := 0; ; {
		at := strings.Index(flight[offset:], olxAdsMarker)
		if at < 0 {
			return nil, found
		}
		offset += at + len(olxAdsMarker) - 1

		var ads []olxAd
		if err := json.NewDecoder(strings.NewReader(flight[offset:])).Decode(&ads); err != nil {
			continue
		}
		found = true
		if len(ads) > 0 {
			return ads, true
		}
	}
}

func olxLocation(ad olxAd) string {
	city := strings.TrimSpace(ad.LocationDetails.Municipality)
	if city == "" {
		return strings.TrimSpace(ad.Location)
	}
	if uf := strings.TrimSpace(ad.LocationDetails.UF); uf != "" {
		return city + " - " + uf
	}
	return city
}

func olxImage(ad olxAd) string {
	if len(ad.Images) == 0 {
		return ""
	}
	return ad.Images[0].Original
}

func olxProperty(ad olxAd, name string) string {
	for _, p := range ad.Properties {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}
