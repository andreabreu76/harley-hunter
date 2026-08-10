package match

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/model"
)

func criteria() config.MatchCriteria {
	return config.MatchCriteria{
		Years:              []int{2014, 2015},
		MaybeYears:         []int{2013, 2016},
		MaxPriceCents:      7500000,
		MaybeMaxPriceCents: 8500000,
	}
}

func listing(bike string, year int, cents int64, city, state string) model.Listing {
	l := model.Listing{Bike: bike, City: city, State: state}
	if year != 0 {
		l.Year = &year
	}
	if cents != 0 {
		l.PriceCents = &cents
	}
	return l
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name string
		in   model.Listing
		want model.Verdict
	}{
		{"street glide in target", listing(model.BikeStreetGlide, 2015, 7200000, "curitiba", "PR"), model.VerdictMatch},
		{"road glide in target", listing(model.BikeRoadGlide, 2015, 7490000, "sao paulo", "SP"), model.VerdictMatch},
		{"metro area counts", listing(model.BikeStreetGlide, 2014, 7000000, "niteroi", "RJ"), model.VerdictMatch},
		{"electra glide within budget is maybe", listing(model.BikeElectraGlide, 2015, 7000000, "curitiba", "PR"), model.VerdictMaybe},
		{"ultra within budget is maybe", listing(model.BikeUltra, 2015, 7000000, "curitiba", "PR"), model.VerdictMaybe},
		{"other brand rejected", listing(model.BikeOther, 2015, 7000000, "curitiba", "PR"), model.VerdictReject},
		{"adjacent year is maybe", listing(model.BikeStreetGlide, 2016, 7000000, "curitiba", "PR"), model.VerdictMaybe},
		{"missing year is maybe", listing(model.BikeStreetGlide, 0, 7000000, "curitiba", "PR"), model.VerdictMaybe},
		{"missing price is maybe", listing(model.BikeStreetGlide, 2015, 0, "curitiba", "PR"), model.VerdictMaybe},
		{"slightly over budget is maybe", listing(model.BikeStreetGlide, 2015, 8000000, "curitiba", "PR"), model.VerdictMaybe},
		{"far over budget rejected", listing(model.BikeStreetGlide, 2015, 9500000, "curitiba", "PR"), model.VerdictReject},
		{"same state is maybe", listing(model.BikeStreetGlide, 2015, 7000000, "campinas", "SP"), model.VerdictMaybe},
		{"other state rejected", listing(model.BikeStreetGlide, 2015, 7000000, "belo horizonte", "MG"), model.VerdictReject},
		{"unknown touring is maybe", listing(model.BikeTouringUnknown, 2015, 7000000, "curitiba", "PR"), model.VerdictMaybe},
		{"old year rejected", listing(model.BikeStreetGlide, 2009, 7000000, "curitiba", "PR"), model.VerdictReject},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, axes := Evaluate(c.in, criteria())
			if got != c.want {
				t.Errorf("Evaluate = %q, want %q (axes: %v)", got, c.want, axes)
			}
			if len(axes) != 4 {
				t.Errorf("expected 4 axes, got %d", len(axes))
			}
		})
	}
}

func TestEvaluateBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		in       model.Listing
		want     model.Verdict
		axis     string
		axisWant model.Verdict
	}{
		{"price exactly at budget still matches", listing(model.BikeStreetGlide, 2015, 7500000, "curitiba", "PR"), model.VerdictMatch, model.AxisPrice, model.VerdictMatch},
		{"one cent over budget is maybe", listing(model.BikeStreetGlide, 2015, 7500001, "curitiba", "PR"), model.VerdictMaybe, model.AxisPrice, model.VerdictMaybe},
		{"price exactly at maybe ceiling is maybe", listing(model.BikeStreetGlide, 2015, 8500000, "curitiba", "PR"), model.VerdictMaybe, model.AxisPrice, model.VerdictMaybe},
		{"one cent over maybe ceiling is rejected", listing(model.BikeStreetGlide, 2015, 8500001, "curitiba", "PR"), model.VerdictReject, model.AxisPrice, model.VerdictReject},
		{"oldest maybe year is maybe", listing(model.BikeStreetGlide, 2013, 7000000, "curitiba", "PR"), model.VerdictMaybe, model.AxisYear, model.VerdictMaybe},
		{"year and price both absent is maybe", listing(model.BikeStreetGlide, 0, 0, "curitiba", "PR"), model.VerdictMaybe, model.AxisYear, model.VerdictMaybe},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, axes := Evaluate(c.in, criteria())
			if got != c.want {
				t.Errorf("Evaluate = %q, want %q (axes: %v)", got, c.want, axes)
			}
			if axes[c.axis] != c.axisWant {
				t.Errorf("axis %q = %q, want %q", c.axis, axes[c.axis], c.axisWant)
			}
		})
	}
}

func TestEvaluateNeverRejectsOnAbsentValuesAlone(t *testing.T) {
	l := listing(model.BikeStreetGlide, 0, 0, "curitiba", "PR")
	got, axes := Evaluate(l, criteria())
	if got == model.VerdictReject {
		t.Fatalf("absent year and price must not reject, got %q (axes: %v)", got, axes)
	}
	if axes[model.AxisYear] != model.VerdictMaybe || axes[model.AxisPrice] != model.VerdictMaybe {
		t.Errorf("absent values must yield maybe, got year=%q price=%q", axes[model.AxisYear], axes[model.AxisPrice])
	}
}

func TestTouringSiblingsNeverReachMatch(t *testing.T) {
	for _, bike := range []string{model.BikeElectraGlide, model.BikeUltra, model.BikeTouringUnknown} {
		verdict, axes := Evaluate(listing(bike, 2015, 7000000, "curitiba", "PR"), criteria())
		if verdict != model.VerdictMaybe {
			t.Errorf("%s = %q, want maybe with every other axis matching", bike, verdict)
		}
		if axes[model.AxisModel] != model.VerdictMaybe {
			t.Errorf("%s model axis = %q, want maybe", bike, axes[model.AxisModel])
		}
		for _, axis := range []string{model.AxisYear, model.AxisPrice, model.AxisLocation} {
			if axes[axis] != model.VerdictMatch {
				t.Fatalf("%s axis %s = %q, want match so the case proves the model axis is the cap", bike, axis, axes[axis])
			}
		}
	}
}

func TestOnlyStreetAndRoadGlideCanMatch(t *testing.T) {
	for _, bike := range []string{model.BikeStreetGlide, model.BikeRoadGlide} {
		if got, _ := Evaluate(listing(bike, 2015, 7000000, "curitiba", "PR"), criteria()); got != model.VerdictMatch {
			t.Errorf("%s = %q, want match", bike, got)
		}
	}
	if got, _ := Evaluate(listing(model.BikeOther, 2015, 7000000, "curitiba", "PR"), criteria()); got != model.VerdictReject {
		t.Errorf("other = %q, want reject: the radar is not open to every Harley", got)
	}
}

func TestAbsentLocationIsMaybeLikeEveryOtherAbsentAxis(t *testing.T) {
	l := listing(model.BikeStreetGlide, 2014, 7480000, "", "")

	verdict, axes := Evaluate(l, criteria())
	if axes[model.AxisLocation] != model.VerdictMaybe {
		t.Errorf("location axis = %q, want maybe: an ad that never wrote a city is unknown, not elsewhere",
			axes[model.AxisLocation])
	}
	if verdict != model.VerdictMaybe {
		t.Errorf("Evaluate = %q, want maybe (axes: %v)", verdict, axes)
	}
}

func TestTheInstagramStreetGlideReachesTalvez(t *testing.T) {
	year := 2014
	cents := int64(7480000)
	km := 57400
	l := model.Listing{
		Source: "instagram", ExternalID: "DVgc6lPkQVb",
		Title: "HD Street Glide 2014/2014 57.400km 1.680cc Motor 103",
		Bike:  model.BikeStreetGlide, Variant: model.VariantBase,
		Year: &year, PriceCents: &cents, Km: &km,
	}

	verdict, axes := Evaluate(l, criteria())
	for _, axis := range []string{model.AxisModel, model.AxisYear, model.AxisPrice} {
		if axes[axis] != model.VerdictMatch {
			t.Fatalf("axis %s = %q, want match: the case only means something when the other three match", axis, axes[axis])
		}
	}
	if verdict != model.VerdictMaybe {
		t.Errorf("Evaluate = %q, want maybe: this is the bike the project exists to find", verdict)
	}
}

func TestAbsentLocationStillCannotReachMatch(t *testing.T) {
	verdict, axes := Evaluate(listing(model.BikeStreetGlide, 2015, 7000000, "", ""), criteria())
	if verdict == model.VerdictMatch {
		t.Errorf("Evaluate = match with no location at all (axes: %v), want maybe", axes)
	}
}

func TestAKnownLocationOutsideTheTargetIsStillRejected(t *testing.T) {
	verdict, axes := Evaluate(listing(model.BikeStreetGlide, 2015, 7000000, "belo horizonte", "MG"), criteria())
	if axes[model.AxisLocation] != model.VerdictReject {
		t.Errorf("location axis = %q, want reject: a city that was written and is far away is not unknown",
			axes[model.AxisLocation])
	}
	if verdict != model.VerdictReject {
		t.Errorf("Evaluate = %q, want reject", verdict)
	}
}

func TestAStateOnlyLocationIsUnaffected(t *testing.T) {
	_, axes := Evaluate(listing(model.BikeStreetGlide, 2015, 7000000, "", "SP"), criteria())
	if axes[model.AxisLocation] != model.VerdictMaybe {
		t.Errorf("location axis = %q, want maybe for a target state without a city", axes[model.AxisLocation])
	}
}
