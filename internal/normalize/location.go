package normalize

import (
	"regexp"
	"strings"
)

var segmentSplit = regexp.MustCompile(`[,/()]+|\s+-\s*|\s*-\s+`)

func ParseLocation(s string) (string, string) {
	if strings.TrimSpace(s) == "" {
		return "", ""
	}
	segments := splitSegments(Fold(s))
	if len(segments) == 0 {
		return "", ""
	}

	state, stateIndex := findState(segments)

	fallback := ""
	for _, seg := range segments {
		metroState, ok := metroCities[cityKey(seg)]
		if !ok {
			continue
		}
		if state != "" && metroState == state {
			return cityKey(seg), state
		}
		if fallback == "" {
			fallback = cityKey(seg)
		}
	}
	if fallback != "" {
		return fallback, state
	}

	for i, seg := range segments {
		if i != stateIndex {
			return seg, state
		}
	}
	return "", state
}

func splitSegments(folded string) []string {
	var out []string
	for _, seg := range segmentSplit.Split(folded, -1) {
		if seg = strings.TrimSpace(seg); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func findState(segments []string) (string, int) {
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if len(seg) == 2 && isBrazilianState(strings.ToUpper(seg)) {
			return strings.ToUpper(seg), i
		}
		if uf, ok := stateNames[seg]; ok {
			return uf, i
		}
	}
	return "", -1
}

func cityKey(s string) string {
	return strings.ReplaceAll(s, "-", " ")
}

func LocationTier(city, state string) string {
	if city != "" {
		if metroState, ok := metroCities[cityKey(city)]; ok {
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
