package fipe

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func sampleRefs() []Reference {
	return []Reference{
		{Code: "810066-7", Label: "XL1200X", Bike: model.BikeSportster1200, Variant: model.VariantFortyEight, Year: 2016, PriceCents: 4698800, Month: "setembro de 2026"},
		{Code: "810067-5", Label: "XL1200C", Bike: model.BikeSportster1200, Variant: model.VariantCustom, Year: 2016, PriceCents: 4712700, Month: "setembro de 2026"},
		{Code: "810099-3", Label: "XL1200NS", Bike: model.BikeSportster1200, Variant: model.VariantIron, Year: 2019, PriceCents: 4963500, Month: "setembro de 2026"},
	}
}

func TestLookupReturnsTheExactCombo(t *testing.T) {
	table := NewTable(sampleRefs())

	ref, ok := table.Lookup(model.BikeSportster1200, model.VariantFortyEight, 2016)
	if !ok {
		t.Fatal("Lookup found nothing for the combo that exists")
	}
	if ref.Code != "810066-7" || ref.PriceCents != 4698800 {
		t.Errorf("ref = %+v, want the XL1200X 2016 row", ref)
	}
	if ref.Base {
		t.Error("Base = true for a confident variant, want false")
	}
}

func TestLookupFallsBackToTheCheapestTrimOfTheYear(t *testing.T) {
	table := NewTable(sampleRefs())

	for _, variant := range []string{model.VariantUnknown, model.VariantBase} {
		ref, ok := table.Lookup(model.BikeSportster1200, variant, 2016)
		if !ok {
			t.Fatalf("%s: Lookup found nothing, want the cheapest trim as a labelled fallback", variant)
		}
		if ref.Code != "810066-7" {
			t.Errorf("%s: ref = %+v, want the cheapest 2016 row so the estimate never flatters the ad", variant, ref)
		}
		if !ref.Base {
			t.Errorf("%s: Base = false, want true so the card can say the trim is a guess", variant)
		}
	}
}

func TestTheFallbackIsTheSameRowEveryTime(t *testing.T) {
	table := NewTable(sampleRefs())

	first, ok := table.Lookup(model.BikeSportster1200, model.VariantUnknown, 2016)
	if !ok {
		t.Fatal("Lookup found nothing")
	}
	for i := 0; i < 50; i++ {
		again, ok := table.Lookup(model.BikeSportster1200, model.VariantUnknown, 2016)
		if !ok || again.Code != first.Code {
			t.Fatalf("run %d gave %v, first gave %s: map order must not reach the card", i, again, first.Code)
		}
	}
}

func TestLookupNeverGuessesAcrossYears(t *testing.T) {
	table := NewTable(sampleRefs())

	if ref, ok := table.Lookup(model.BikeSportster1200, model.VariantUnknown, 2021); ok {
		t.Errorf("Lookup returned %+v for a year fipe does not publish, want nothing", ref)
	}
}

func TestLookupNeverSubstitutesAnotherTrim(t *testing.T) {
	table := NewTable(sampleRefs())

	if ref, ok := table.Lookup(model.BikeSportster1200, model.VariantRoadster, 2016); ok {
		t.Errorf("Lookup returned %+v for a roadster with no roadster row, want nothing: a wrong reference is worse than none", ref)
	}
}

func TestLookupOfAnUnmappedBikeFindsNothing(t *testing.T) {
	table := NewTable(sampleRefs())

	for _, bike := range []string{model.BikeSportster883, model.BikeSportsterS, model.BikeSportsterUnknown, model.BikeOther} {
		if ref, ok := table.Lookup(bike, model.VariantUnknown, 2016); ok {
			t.Errorf("%s returned %+v, want nothing", bike, ref)
		}
	}
}

func TestLookupOnAnEmptyTableIsQuiet(t *testing.T) {
	if _, ok := NewTable(nil).Lookup(model.BikeSportster1200, model.VariantFortyEight, 2016); ok {
		t.Error("an empty table answered a lookup")
	}
}
