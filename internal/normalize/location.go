package normalize

import (
	"regexp"
	"strings"
)

var stateSuffix = regexp.MustCompile(`(?i)[\s,/\-]+([A-Z]{2})\s*$`)

func ParseLocation(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	state := ""
	if m := stateSuffix.FindStringSubmatch(s); m != nil {
		candidate := strings.ToUpper(m[1])
		if isBrazilianState(candidate) {
			state = candidate
			s = s[:len(s)-len(m[0])]
		}
	}
	city := Fold(strings.Trim(s, " ,-/"))
	return city, state
}

func LocationTier(city, state string) string {
	if city != "" {
		if metroState, ok := metroCities[city]; ok {
			if state == "" || state == metroState {
				return "metro"
			}
		}
	}
	if targetStates[state] {
		return "state"
	}
	return "outside"
}

func isBrazilianState(s string) bool {
	switch s {
	case "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS",
		"MG", "PA", "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC",
		"SP", "SE", "TO":
		return true
	}
	return false
}
