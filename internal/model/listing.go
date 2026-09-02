package model

import "time"

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
	SourceMobiauto     = "mobiauto"
	SourceInstagram    = "instagram"
	SourceMarketplace  = "marketplace"
)

const (
	AxisModel    = "model"
	AxisYear     = "year"
	AxisPrice    = "price"
	AxisLocation = "location"
)

const (
	BikeSportster1200    = "sportster_1200"
	BikeSportster883     = "sportster_883"
	BikeSportsterS       = "sportster_s"
	BikeSportsterUnknown = "sportster_unknown"
	BikeOther            = "other"
)

const (
	VariantCustom     = "custom"
	VariantIron       = "iron"
	VariantFortyEight = "forty_eight"
	VariantRoadster   = "roadster"
	VariantBase       = "base"
	VariantUnknown    = "unknown"
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
	PublishedAt  *time.Time
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
	Phone         *string
	PublishedAt   *time.Time
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
