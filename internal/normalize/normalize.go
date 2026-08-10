package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

const mileageBucketSize = 5000

var fiscalYear = regexp.MustCompile(`\b(ipva|licenciad\w*|licenciamento|crlv|seguro|financiamento|document\w*|emplacad\w*)\s*(?:/|de)?\s*(19|20)\d{2}`)

func Normalize(raw model.RawListing) model.Listing {
	full := strings.TrimSpace(raw.Title + " " + raw.RawText)

	l := model.Listing{
		Source:     raw.Source,
		ExternalID: raw.ExternalID,
		URL:        raw.URL,
		Title:      raw.Title,
		RawText:    raw.RawText,
		ImageURL:   raw.ImageURL,
	}

	l.Bike, l.Variant = DetectBike(full)

	if cents, ok := ParsePrice(raw.PriceText); ok {
		l.PriceCents = &cents
	} else if cents, ok := ParsePrice(full); ok {
		l.PriceCents = &cents
	}

	if year, ok := ParseYear(raw.YearText); ok {
		l.Year = &year
	} else if year, ok := ParseYear(fiscalYear.ReplaceAllString(Fold(full), " ")); ok {
		l.Year = &year
	}

	if km, ok := ParseKm(raw.KmText); ok {
		l.Km = &km
	} else if km, ok := ParseKm(full); ok {
		l.Km = &km
	}

	l.City, l.State = ParseLocation(raw.LocationText)
	if l.City == "" {
		l.City, l.State = locationFromText(full)
	}

	if phone, ok := ParsePhone(full); ok {
		l.Phone = &phone
	}

	l.Fingerprint = Fingerprint(l)
	return l
}

func locationFromText(text string) (string, string) {
	folded := cityKey(Fold(text))
	bestCity, bestState, bestIndex := "", "", 0
	for city, state := range metroCities {
		index := wordIndex(folded, city)
		if index < 0 {
			continue
		}
		if bestCity == "" || len(city) > len(bestCity) ||
			(len(city) == len(bestCity) && index < bestIndex) {
			bestCity, bestState, bestIndex = city, state, index
		}
	}
	return bestCity, bestState
}

func wordIndex(text, term string) int {
	for from := 0; from <= len(text)-len(term); {
		offset := strings.Index(text[from:], term)
		if offset < 0 {
			return -1
		}
		start := from + offset
		if !wordChar(text, start-1) && !wordChar(text, start+len(term)) {
			return start
		}
		from = start + 1
	}
	return -1
}

func wordChar(text string, i int) bool {
	if i < 0 || i >= len(text) {
		return false
	}
	c := text[i]
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

func Fingerprint(l model.Listing) string {
	year := "?"
	if l.Year != nil {
		year = fmt.Sprint(*l.Year)
	}
	bucket := "?"
	if l.Km != nil {
		bucket = fmt.Sprint(*l.Km / mileageBucketSize)
	}
	seed := strings.Join([]string{l.Bike, year, bucket, l.City}, "|")
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8])
}
