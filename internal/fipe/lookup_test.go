package fipe

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func sampleRefs() []Reference {
	return []Reference{
		{Code: "810059-4", Label: "FLHX", Bike: model.BikeStreetGlide, Variant: model.VariantBase, Year: 2014, PriceCents: 6920700, Month: "agosto de 2026"},
		{Code: "810071-3", Label: "FLHXS", Bike: model.BikeStreetGlide, Variant: model.VariantSpecial, Year: 2015, PriceCents: 7464500, Month: "agosto de 2026"},
		{Code: "810060-8", Label: "FLHTK", Bike: model.BikeElectraGlide, Variant: model.VariantUnknown, Year: 2014, PriceCents: 6822400, Month: "agosto de 2026"},
	}
}

func TestLookupReturnsTheExactCombo(t *testing.T) {
	table := NewTable(sampleRefs())

	ref, ok := table.Lookup(model.BikeStreetGlide, model.VariantBase, 2014)
	if !ok {
		t.Fatal("Lookup found nothing for the combo that exists")
	}
	if ref.Code != "810059-4" || ref.PriceCents != 6920700 {
		t.Errorf("ref = %+v, want the FLHX 2014 row", ref)
	}
	if ref.Base {
		t.Error("Base = true for a confident variant, want false")
	}
}

func TestLookupLabelsTheBaseFallbackWhenTheVariantIsUnknown(t *testing.T) {
	table := NewTable(sampleRefs())

	ref, ok := table.Lookup(model.BikeStreetGlide, model.VariantUnknown, 2014)
	if !ok {
		t.Fatal("Lookup found nothing, want the base model as a labelled fallback")
	}
	if ref.Label != "FLHX" {
		t.Errorf("Label = %q, want the base model", ref.Label)
	}
	if !ref.Base {
		t.Error("Base = false, want true so the card can say the trim is a guess")
	}
}

func TestLookupOfAFamilyWithoutTrimIsAlwaysBase(t *testing.T) {
	table := NewTable(sampleRefs())

	ref, ok := table.Lookup(model.BikeElectraGlide, model.VariantUnknown, 2014)
	if !ok {
		t.Fatal("Lookup found nothing for the electra glide row")
	}
	if !ref.Base {
		t.Error("Base = false: the electra glide trim is never known, so the reference is a family reference")
	}
}

func TestLookupNeverGuessesAcrossYears(t *testing.T) {
	table := NewTable(sampleRefs())

	if ref, ok := table.Lookup(model.BikeStreetGlide, model.VariantBase, 2015); ok {
		t.Errorf("Lookup returned %+v for a year fipe does not publish, want nothing", ref)
	}
}

func TestLookupNeverSubstitutesAnotherTrim(t *testing.T) {
	table := NewTable(sampleRefs())

	if ref, ok := table.Lookup(model.BikeStreetGlide, model.VariantCVO, 2014); ok {
		t.Errorf("Lookup returned %+v for a cvo with no cvo row, want nothing: a wrong reference is worse than none", ref)
	}
}

func TestLookupOfAnUnmappedBikeFindsNothing(t *testing.T) {
	table := NewTable(sampleRefs())

	for _, bike := range []string{model.BikeTouringUnknown, model.BikeOther} {
		if ref, ok := table.Lookup(bike, model.VariantUnknown, 2014); ok {
			t.Errorf("%s returned %+v, want nothing", bike, ref)
		}
	}
}

func TestLookupOnAnEmptyTableIsQuiet(t *testing.T) {
	if _, ok := NewTable(nil).Lookup(model.BikeStreetGlide, model.VariantBase, 2014); ok {
		t.Error("an empty table answered a lookup")
	}
}
