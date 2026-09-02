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
	return DetectBikeIn(text, text)
}

func DetectBikeIn(title, text string) (string, string) {
	name := headline(title)
	if body := captionBody(text); body != "" {
		if bike, variant := detectBike(body, name); namesAFamily(bike) {
			return bike, variant
		}
	}
	return detectBike(text, name)
}

func headline(title string) string {
	if body := captionBody(title); body != "" {
		return body
	}
	return title
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
	case model.BikeSportster1200, model.BikeSportster883, model.BikeSportsterS:
		return true
	}
	return false
}

func detectBike(text, name string) (string, string) {
	t := Fold(text)
	compact := compactor.Replace(t)
	named := Fold(name)

	switch {
	case containsCode(compact, "xr1200"):
		return model.BikeSportsterUnknown, model.VariantUnknown
	case namesSportsterS(t, compact):
		return model.BikeSportsterS, model.VariantUnknown
	case names1200(t, compact):
		return model.BikeSportster1200, detectVariant(t, compact, named)
	case names883(t, compact):
		return model.BikeSportster883, model.VariantUnknown
	case namesSportster(t):
		return model.BikeSportsterUnknown, model.VariantUnknown
	default:
		return model.BikeOther, model.VariantUnknown
	}
}

var displacementCodes = []string{"xl1200cx", "xl1200ns", "xl1200c", "xl1200x", "xl1200n", "xl1200t", "xl1200v", "xl1200"}

func names1200(t, compact string) bool {
	for _, code := range displacementCodes {
		if containsCode(compact, code) {
			return true
		}
	}
	if containsAny(t, compact, "xl 1200", "xl1200", "forty eight", "fortyeight", "seventy two", "seventytwo", "iron 1200", "iron1200") {
		return true
	}
	if namesTheFortyEight(t) {
		return true
	}
	if containsWord(t, "roadster") && inSportsterContext(t) {
		return true
	}
	return strings.Contains(t, "1200") && inSportsterContext(t)
}

func names883(t, compact string) bool {
	for _, code := range []string{"xl883n", "xl883l", "xl883r", "xl883"} {
		if containsCode(compact, code) {
			return true
		}
	}
	return strings.Contains(t, "883") && inSportsterContext(t)
}

func namesSportsterS(t, compact string) bool {
	return containsWord(t, "sportster s") || containsCode(compact, "rh1250")
}

func namesSportster(t string) bool {
	if strings.Contains(t, "sportster") {
		return true
	}
	return isHarley(t) && containsWord(t, "iron", "nightster", "superlow", "roadster", "forty eight")
}

func namesTheFortyEight(t string) bool {
	return containsWord(t, "harley 48", "davidson 48", "hd 48", "sportster 48")
}

func inSportsterContext(t string) bool {
	return strings.Contains(t, "sportster") || isHarley(t)
}

func detectVariant(t, compact, named string) string {
	if v := variantIn(named, compactor.Replace(named)); v != model.VariantUnknown {
		return v
	}
	if v := variantIn(t, compact); v != model.VariantUnknown {
		return v
	}
	return model.VariantBase
}

func variantIn(t, compact string) string {
	switch {
	case containsCode(compact, "xl1200cx") || containsWord(t, "roadster"):
		return model.VariantRoadster
	case containsCode(compact, "xl1200ns") || containsWord(t, "iron"):
		return model.VariantIron
	case containsCode(compact, "xl1200x") || containsAny(t, compact, "forty eight", "fortyeight") || namesTheFortyEight(t):
		return model.VariantFortyEight
	case containsCode(compact, "xl1200c") || containsWord(t, "custom"):
		return model.VariantCustom
	default:
		return model.VariantUnknown
	}
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
