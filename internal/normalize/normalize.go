package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

const mileageBucketSize = 5000

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
	} else if year, ok := ParseYear(full); ok {
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

	l.Fingerprint = Fingerprint(l)
	return l
}

func locationFromText(text string) (string, string) {
	folded := Fold(text)
	for city, state := range metroCities {
		if strings.Contains(folded, city) {
			return city, state
		}
	}
	return "", ""
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
