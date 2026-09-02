package match

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/model"
)

func criteria() config.MatchCriteria {
	return config.MatchCriteria{
		Years:              []int{2016},
		MaxPriceCents:      4500000,
		MaybeMaxPriceCents: 5500000,
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
		{"sportster 1200 in target", listing(model.BikeSportster1200, 2016, 4300000, "rio de janeiro", "RJ"), model.VerdictMatch},
		{"the target year at the budget ceiling", listing(model.BikeSportster1200, 2016, 4490000, "niteroi", "RJ"), model.VerdictMatch},
		{"metro area counts", listing(model.BikeSportster1200, 2016, 4200000, "marica", "RJ"), model.VerdictMatch},
		{"883 within budget is maybe", listing(model.BikeSportster883, 2016, 4000000, "rio de janeiro", "RJ"), model.VerdictMaybe},
		{"sportster s within budget is maybe", listing(model.BikeSportsterS, 2016, 4400000, "rio de janeiro", "RJ"), model.VerdictMaybe},
		{"unknown sportster is maybe", listing(model.BikeSportsterUnknown, 2016, 4000000, "rio de janeiro", "RJ"), model.VerdictMaybe},
		{"other brand rejected", listing(model.BikeOther, 2016, 4000000, "rio de janeiro", "RJ"), model.VerdictReject},
		{"a year outside the target is rejected", listing(model.BikeSportster1200, 2017, 4000000, "rio de janeiro", "RJ"), model.VerdictReject},
		{"missing year is maybe", listing(model.BikeSportster1200, 0, 4000000, "rio de janeiro", "RJ"), model.VerdictMaybe},
		{"missing price is maybe", listing(model.BikeSportster1200, 2016, 0, "rio de janeiro", "RJ"), model.VerdictMaybe},
		{"slightly over budget is maybe", listing(model.BikeSportster1200, 2016, 5000000, "rio de janeiro", "RJ"), model.VerdictMaybe},
		{"far over budget rejected", listing(model.BikeSportster1200, 2016, 6500000, "rio de janeiro", "RJ"), model.VerdictReject},
		{"same state is maybe", listing(model.BikeSportster1200, 2016, 4000000, "volta redonda", "RJ"), model.VerdictMaybe},
		{"other state rejected", listing(model.BikeSportster1200, 2016, 4000000, "sao paulo", "SP"), model.VerdictReject},
		{"old year rejected", listing(model.BikeSportster1200, 2009, 4000000, "rio de janeiro", "RJ"), model.VerdictReject},
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
		{"price exactly at budget still matches", listing(model.BikeSportster1200, 2016, 4500000, "rio de janeiro", "RJ"), model.VerdictMatch, model.AxisPrice, model.VerdictMatch},
		{"one cent over budget is maybe", listing(model.BikeSportster1200, 2016, 4500001, "rio de janeiro", "RJ"), model.VerdictMaybe, model.AxisPrice, model.VerdictMaybe},
		{"price exactly at maybe ceiling is maybe", listing(model.BikeSportster1200, 2016, 5500000, "rio de janeiro", "RJ"), model.VerdictMaybe, model.AxisPrice, model.VerdictMaybe},
		{"one cent over maybe ceiling is rejected", listing(model.BikeSportster1200, 2016, 5500001, "rio de janeiro", "RJ"), model.VerdictReject, model.AxisPrice, model.VerdictReject},
		{"the year right below the cut is rejected", listing(model.BikeSportster1200, 2015, 4000000, "rio de janeiro", "RJ"), model.VerdictReject, model.AxisYear, model.VerdictReject},
		{"year and price both absent is maybe", listing(model.BikeSportster1200, 0, 0, "rio de janeiro", "RJ"), model.VerdictMaybe, model.AxisYear, model.VerdictMaybe},
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

func TestOnlyTheTargetYearMatches(t *testing.T) {
	if _, axes := Evaluate(listing(model.BikeSportster1200, 2016, 4000000, "rio de janeiro", "RJ"), criteria()); axes[model.AxisYear] != model.VerdictMatch {
		t.Errorf("2016 axis = %q, want match", axes[model.AxisYear])
	}
	for _, year := range []int{2014, 2015, 2017, 2019, 2026} {
		_, axes := Evaluate(listing(model.BikeSportster1200, year, 4000000, "rio de janeiro", "RJ"), criteria())
		if axes[model.AxisYear] != model.VerdictReject {
			t.Errorf("year %d axis = %q, want reject: the target is the 2016 model year alone", year, axes[model.AxisYear])
		}
	}
}

func TestSaoPauloAndParanaAreOutsideTheTarget(t *testing.T) {
	for _, place := range []struct{ city, state string }{
		{"sao paulo", "SP"}, {"guarulhos", "SP"}, {"curitiba", "PR"},
	} {
		verdict, axes := Evaluate(listing(model.BikeSportster1200, 2016, 4000000, place.city, place.state), criteria())
		if axes[model.AxisLocation] != model.VerdictReject {
			t.Errorf("%s/%s location axis = %q, want reject: the hunt is Rio only", place.city, place.state, axes[model.AxisLocation])
		}
		if verdict != model.VerdictReject {
			t.Errorf("%s/%s = %q, want reject", place.city, place.state, verdict)
		}
	}
}

func TestEvaluateNeverRejectsOnAbsentValuesAlone(t *testing.T) {
	l := listing(model.BikeSportster1200, 0, 0, "rio de janeiro", "RJ")
	got, axes := Evaluate(l, criteria())
	if got == model.VerdictReject {
		t.Fatalf("absent year and price must not reject, got %q (axes: %v)", got, axes)
	}
	if axes[model.AxisYear] != model.VerdictMaybe || axes[model.AxisPrice] != model.VerdictMaybe {
		t.Errorf("absent values must yield maybe, got year=%q price=%q", axes[model.AxisYear], axes[model.AxisPrice])
	}
}

func TestSportsterSiblingsNeverReachMatch(t *testing.T) {
	for _, bike := range []string{model.BikeSportster883, model.BikeSportsterS, model.BikeSportsterUnknown} {
		verdict, axes := Evaluate(listing(bike, 2016, 4000000, "rio de janeiro", "RJ"), criteria())
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

func TestOnlyTheSportster1200CanMatch(t *testing.T) {
	if got, _ := Evaluate(listing(model.BikeSportster1200, 2016, 4000000, "rio de janeiro", "RJ"), criteria()); got != model.VerdictMatch {
		t.Errorf("sportster_1200 = %q, want match", got)
	}
	if got, _ := Evaluate(listing(model.BikeOther, 2016, 4000000, "rio de janeiro", "RJ"), criteria()); got != model.VerdictReject {
		t.Errorf("other = %q, want reject: the radar is not open to every Harley", got)
	}
}

func TestAbsentLocationIsMaybeLikeEveryOtherAbsentAxis(t *testing.T) {
	l := listing(model.BikeSportster1200, 2016, 4480000, "", "")

	verdict, axes := Evaluate(l, criteria())
	if axes[model.AxisLocation] != model.VerdictMaybe {
		t.Errorf("location axis = %q, want maybe: an ad that never wrote a city is unknown, not elsewhere",
			axes[model.AxisLocation])
	}
	if verdict != model.VerdictMaybe {
		t.Errorf("Evaluate = %q, want maybe (axes: %v)", verdict, axes)
	}
}

func TestTheInstagramFortyEightReachesTalvez(t *testing.T) {
	year := 2016
	cents := int64(4480000)
	km := 12400
	l := model.Listing{
		Source: "instagram", ExternalID: "DVgc6lPkQVb",
		Title: "HD Forty Eight 2016/2016 12.400km 1.200cc",
		Bike:  model.BikeSportster1200, Variant: model.VariantFortyEight,
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
	verdict, axes := Evaluate(listing(model.BikeSportster1200, 2016, 4000000, "", ""), criteria())
	if verdict == model.VerdictMatch {
		t.Errorf("Evaluate = match with no location at all (axes: %v), want maybe", axes)
	}
}

func TestAKnownLocationOutsideTheTargetIsStillRejected(t *testing.T) {
	verdict, axes := Evaluate(listing(model.BikeSportster1200, 2016, 4000000, "belo horizonte", "MG"), criteria())
	if axes[model.AxisLocation] != model.VerdictReject {
		t.Errorf("location axis = %q, want reject: a city that was written and is far away is not unknown",
			axes[model.AxisLocation])
	}
	if verdict != model.VerdictReject {
		t.Errorf("Evaluate = %q, want reject", verdict)
	}
}

func TestAStateOnlyLocationIsUnaffected(t *testing.T) {
	_, axes := Evaluate(listing(model.BikeSportster1200, 2016, 4000000, "", "RJ"), criteria())
	if axes[model.AxisLocation] != model.VerdictMaybe {
		t.Errorf("location axis = %q, want maybe for the target state without a city", axes[model.AxisLocation])
	}
}
