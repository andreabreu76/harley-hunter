package model

type Verdict string

const (
	VerdictMatch  Verdict = "match"
	VerdictMaybe  Verdict = "maybe"
	VerdictReject Verdict = "reject"
)

const (
	SourceOLX          = "olx"
	SourceMercadoLivre = "mercadolivre"
	SourceWebmotors    = "webmotors"
)

const (
	AxisModel    = "model"
	AxisYear     = "year"
	AxisPrice    = "price"
	AxisLocation = "location"
)

const (
	BikeStreetGlide    = "street_glide"
	BikeRoadGlide      = "road_glide"
	BikeElectraGlide   = "electra_glide"
	BikeUltra          = "ultra"
	BikeTouringUnknown = "touring_unknown"
	BikeOther          = "other"
)

const (
	VariantBase    = "base"
	VariantSpecial = "special"
	VariantCVO     = "cvo"
	VariantUnknown = "unknown"
)

type RawListing struct {
	Source       string
	ExternalID   string
	URL          string
	Title        string
	RawText      string
	PriceText    string
	YearText     string
	KmText       string
	LocationText string
	ImageURL     string
}

type Listing struct {
	Source        string
	ExternalID    string
	URL           string
	Title         string
	RawText       string
	Bike          string
	Variant       string
	Year          *int
	PriceCents    *int64
	Km            *int
	City          string
	State         string
	ImageURL      string
	Verdict       Verdict
	VerdictReason map[string]Verdict
	Fingerprint   string
}

func CombineVerdicts(axes map[string]Verdict) Verdict {
	if len(axes) == 0 {
		return VerdictReject
	}
	result := VerdictMatch
	for _, v := range axes {
		if v == VerdictReject {
			return VerdictReject
		}
		if v == VerdictMaybe {
			result = VerdictMaybe
		}
	}
	return result
}
