package normalize

import (
	"regexp"
	"strings"
)

var phoneCandidate = regexp.MustCompile(`((?:\+?55[\s.\-]?)?)\(?(\d{2})\)?[\s.\-]?(9?)[\s.\-]?(\d{4,5})[\s.\-]?(\d{4})`)

func ParsePhone(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	for _, loc := range phoneCandidate.FindAllStringSubmatchIndex(s, -1) {
		if digitAt(s, loc[0]-1) || digitAt(s, loc[1]) {
			continue
		}
		area := s[loc[4]:loc[5]]
		subscriber := s[loc[6]:loc[7]] + s[loc[8]:loc[9]] + s[loc[10]:loc[11]]
		national := s[loc[3]:loc[1]]
		if plausiblePhone(area, subscriber, national) {
			return area + subscriber, true
		}
	}
	return "", false
}

func plausiblePhone(area, subscriber, national string) bool {
	if area < "11" || area > "99" {
		return false
	}
	mobile := len(subscriber) == 9 && subscriber[0] == '9'
	landline := len(subscriber) == 8 && subscriber[0] >= '2' && subscriber[0] <= '9'
	if !mobile && !landline {
		return false
	}
	if !mobile && strings.IndexFunc(national, notDigit) < 0 {
		return false
	}
	return true
}

func notDigit(r rune) bool {
	return r < '0' || r > '9'
}

func digitAt(s string, i int) bool {
	return i >= 0 && i < len(s) && s[i] >= '0' && s[i] <= '9'
}
