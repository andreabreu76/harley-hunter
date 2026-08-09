package match

import (
	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
)

func Evaluate(l model.Listing, c config.MatchCriteria) (model.Verdict, map[string]model.Verdict) {
	axes := map[string]model.Verdict{
		model.AxisModel:    evaluateBike(l.Bike),
		model.AxisYear:     evaluateYear(l.Year, c),
		model.AxisPrice:    evaluatePrice(l.PriceCents, c),
		model.AxisLocation: evaluateLocation(l.City, l.State),
	}
	return model.CombineVerdicts(axes), axes
}

func evaluateBike(bike string) model.Verdict {
	switch bike {
	case model.BikeStreetGlide, model.BikeRoadGlide:
		return model.VerdictMatch
	case model.BikeTouringUnknown:
		return model.VerdictMaybe
	default:
		return model.VerdictReject
	}
}

func evaluateYear(year *int, c config.MatchCriteria) model.Verdict {
	if year == nil {
		return model.VerdictMaybe
	}
	if contains(c.Years, *year) {
		return model.VerdictMatch
	}
	if contains(c.MaybeYears, *year) {
		return model.VerdictMaybe
	}
	return model.VerdictReject
}

func evaluatePrice(cents *int64, c config.MatchCriteria) model.Verdict {
	if cents == nil {
		return model.VerdictMaybe
	}
	if *cents <= c.MaxPriceCents {
		return model.VerdictMatch
	}
	if *cents <= c.MaybeMaxPriceCents {
		return model.VerdictMaybe
	}
	return model.VerdictReject
}

func evaluateLocation(city, state string) model.Verdict {
	switch normalize.LocationTier(city, state) {
	case "metro":
		return model.VerdictMatch
	case "state":
		return model.VerdictMaybe
	default:
		return model.VerdictReject
	}
}

func contains(list []int, v int) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
