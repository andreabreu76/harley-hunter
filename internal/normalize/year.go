package normalize

import (
	"regexp"
	"strconv"
	"time"
)

var (
	fourDigitYear = regexp.MustCompile(`\b(19[5-9]\d|20[0-4]\d)\b`)
	twoDigitPair  = regexp.MustCompile(`\b(\d{2})\s*/\s*(\d{2})\b`)
	fourDigitPair = regexp.MustCompile(`\b(19\d{2}|20\d{2})\s*/\s*(19\d{2}|20\d{2})\b`)
)

func ParseYear(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	maxYear := time.Now().Year() + 1

	if m := fourDigitPair.FindStringSubmatch(s); m != nil {
		year, _ := strconv.Atoi(m[2])
		return validYear(year, maxYear)
	}
	if m := twoDigitPair.FindStringSubmatch(s); m != nil {
		year, _ := strconv.Atoi(m[2])
		return validYear(2000+year, maxYear)
	}
	if m := fourDigitYear.FindString(s); m != "" {
		year, _ := strconv.Atoi(m)
		return validYear(year, maxYear)
	}
	return 0, false
}

func validYear(year, maxYear int) (int, bool) {
	if year < 1950 || year > maxYear {
		return 0, false
	}
	return year, true
}
