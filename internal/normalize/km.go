package normalize

import (
	"regexp"
	"strconv"
	"strings"
)

const maxPlausibleKm = 400000

var (
	kmThousands = regexp.MustCompile(`(?i)([\d.,]+)\s*mil\s*km`)
	kmPlain     = regexp.MustCompile(`(?i)([\d.,]+)\s*km`)
)

func ParseKm(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if m := kmThousands.FindStringSubmatch(s); m != nil {
		value, err := strconv.ParseFloat(decimalize(m[1]), 64)
		if err != nil {
			return 0, false
		}
		return plausibleKm(int(value * 1000))
	}
	if m := kmPlain.FindStringSubmatch(s); m != nil {
		digits := strings.NewReplacer(".", "", ",", "").Replace(m[1])
		value, err := strconv.Atoi(digits)
		if err != nil {
			return 0, false
		}
		return plausibleKm(value)
	}
	return 0, false
}

func plausibleKm(km int) (int, bool) {
	if km <= 0 || km > maxPlausibleKm {
		return 0, false
	}
	return km, true
}
