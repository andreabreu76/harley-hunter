package normalize

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	minPlausiblePriceCents = 500000
	maxPlausiblePriceCents = 50000000
)

var (
	unavailablePrice = regexp.MustCompile(`(?i)combinar|consulte|sob\s+consulta`)
	thousandsSuffix  = regexp.MustCompile(`(?i)([\d.,]+)\s*mil\s*([a-z]*)`)
	priceWithSymbol  = regexp.MustCompile(`(?i)r\$\s*([\d.,]+)`)
	bareNumber       = regexp.MustCompile(`^\s*([\d.,]+)\s*$`)
)

func ParsePrice(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || unavailablePrice.MatchString(s) {
		return 0, false
	}

	best := int64(0)
	for _, loc := range priceWithSymbol.FindAllStringSubmatchIndex(s, -1) {
		if precededByCeilingMarker(s, loc[0]) {
			continue
		}
		value, err := strconv.ParseFloat(decimalize(s[loc[2]:loc[3]]), 64)
		if err != nil {
			continue
		}
		if cents, ok := plausible(int64(value*100 + 0.5)); ok && cents > best {
			best = cents
		}
	}
	if best > 0 {
		return best, true
	}

	for _, m := range thousandsSuffix.FindAllStringSubmatch(s, -1) {
		if isNonPriceWord(m[2]) {
			continue
		}
		if value, err := strconv.ParseFloat(decimalize(m[1]), 64); err == nil {
			if cents, ok := plausible(int64(value*1000*100 + 0.5)); ok {
				return cents, true
			}
		}
	}

	if m := bareNumber.FindStringSubmatch(s); m != nil {
		if value, err := strconv.ParseFloat(decimalize(m[1]), 64); err == nil {
			if cents, ok := plausible(int64(value*100 + 0.5)); ok {
				return cents, true
			}
		}
	}

	return 0, false
}

var ceilingMarkers = []string{"ate", "até"}

func precededByCeilingMarker(s string, at int) bool {
	start := at - 8
	if start < 0 {
		start = 0
	}
	window := strings.TrimRight(strings.ToLower(s[start:at]), " ")
	for _, marker := range ceilingMarkers {
		if strings.HasSuffix(window, marker) {
			return true
		}
	}
	return false
}

var nonPricePrefixes = []string{
	"km", "quil", "curtid", "seguidor", "visualiza", "like", "view",
	"inscrit", "coment", "compartilh", "avalia",
}

func isNonPriceWord(s string) bool {
	s = strings.ToLower(s)
	for _, prefix := range nonPricePrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func decimalize(s string) string {
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		return strings.ReplaceAll(s, ",", ".")
	}
	if strings.Count(s, ".") >= 1 {
		parts := strings.Split(s, ".")
		last := parts[len(parts)-1]
		if len(last) == 3 {
			return strings.ReplaceAll(s, ".", "")
		}
	}
	return s
}

func plausible(cents int64) (int64, bool) {
	if cents < minPlausiblePriceCents || cents > maxPlausiblePriceCents {
		return 0, false
	}
	return cents, true
}
