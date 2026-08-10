package normalize

import (
	"strings"
	"unicode"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"golang.org/x/text/unicode/norm"
)

var compactor = strings.NewReplacer(" ", "", "-", "", ".", "", "/", "")

func Fold(s string) string {
	decomposed := norm.NFKD.String(s)
	var b strings.Builder
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(strings.ToLower(b.String())), " ")
}

func DetectBike(text string) (string, string) {
	if body := captionBody(text); body != "" {
		if bike, variant := detectBike(body); namesAFamily(bike) {
			return bike, variant
		}
	}
	return detectBike(text)
}

func captionBody(text string) string {
	at := strings.IndexByte(text, '#')
	if at < 0 {
		return ""
	}
	return strings.TrimSpace(text[:at])
}

func namesAFamily(bike string) bool {
	switch bike {
	case model.BikeStreetGlide, model.BikeRoadGlide, model.BikeElectraGlide, model.BikeUltra:
		return true
	}
	return false
}

func detectBike(text string) (string, string) {
	t := Fold(text)
	compact := compactor.Replace(t)

	switch {
	case containsAny(t, compact, "electra glide", "electraglide", "flht"):
		return model.BikeElectraGlide, model.VariantUnknown
	case containsAny(t, compact, "road glide", "roadglide", "fltrx"):
		return model.BikeRoadGlide, detectVariant(t, compact, "fltrxse", "fltrxs")
	case containsAny(t, compact, "street glide", "streetglide", "stglide", "flhx"):
		return model.BikeStreetGlide, detectVariant(t, compact, "flhxse", "flhxs")
	case containsAny(t, compact, "ultra limited", "ultraclassic", "ultra classic"):
		return model.BikeUltra, model.VariantUnknown
	case isHarley(t) && containsAny(t, compact, "touring", "1690", "1745", "rushmore"):
		return model.BikeTouringUnknown, model.VariantUnknown
	default:
		return model.BikeOther, model.VariantUnknown
	}
}

func detectVariant(t, compact, cvoCode, specialCode string) string {
	if strings.Contains(t, "cvo") || containsCode(compact, cvoCode) {
		return model.VariantCVO
	}
	if containsWord(t, "special", "especial") || containsCode(compact, specialCode) {
		return model.VariantSpecial
	}
	return model.VariantBase
}

func containsWord(text string, words ...string) bool {
	for _, w := range words {
		if wordIndex(text, w) >= 0 {
			return true
		}
	}
	return false
}

func containsCode(compact, code string) bool {
	for from := 0; from <= len(compact)-len(code); {
		offset := strings.Index(compact[from:], code)
		if offset < 0 {
			return false
		}
		end := from + offset + len(code)
		if end >= len(compact) || compact[end] < 'a' || compact[end] > 'z' {
			return true
		}
		from = from + offset + 1
	}
	return false
}

func isHarley(t string) bool {
	return strings.Contains(t, "harley") || strings.Contains(t, "hd ")
}

func containsAny(t, compact string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(t, n) || strings.Contains(compact, strings.ReplaceAll(n, " ", "")) {
			return true
		}
	}
	return false
}
