package normalize

import (
	"strings"
	"unicode"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"golang.org/x/text/unicode/norm"
)

var compactor = strings.NewReplacer(" ", "", "-", "", ".", "", "/", "")

func Fold(s string) string {
	decomposed := norm.NFD.String(strings.ToLower(s))
	var b strings.Builder
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func DetectBike(text string) (string, string) {
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
	if containsAny(t, compact, "cvo", cvoCode) {
		return model.VariantCVO
	}
	if containsAny(t, compact, "special", "especial", specialCode) {
		return model.VariantSpecial
	}
	return model.VariantBase
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
