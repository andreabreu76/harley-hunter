package normalize

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestNormalizeUsesStructuredFieldsFirst(t *testing.T) {
	raw := model.RawListing{
		Source:       "olx",
		ExternalID:   "123",
		Title:        "Harley Davidson Street Glide Special",
		PriceText:    "R$ 72.000",
		YearText:     "2015",
		KmText:       "31.000 km",
		LocationText: "Curitiba - PR",
	}
	l := Normalize(raw)

	if l.Bike != model.BikeStreetGlide || l.Variant != model.VariantSpecial {
		t.Errorf("bike/variant = %q/%q", l.Bike, l.Variant)
	}
	if l.Year == nil || *l.Year != 2015 {
		t.Errorf("Year = %v, want 2015", l.Year)
	}
	if l.PriceCents == nil || *l.PriceCents != 7200000 {
		t.Errorf("PriceCents = %v, want 7200000", l.PriceCents)
	}
	if l.Km == nil || *l.Km != 31000 {
		t.Errorf("Km = %v, want 31000", l.Km)
	}
	if l.City != "curitiba" || l.State != "PR" {
		t.Errorf("location = %q/%q", l.City, l.State)
	}
}

func TestNormalizeFallsBackToFreeText(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "abc",
		RawText:    "Vendo Road Glide Special 15/15, 42.000 km, R$ 74.900, Sao Paulo SP",
	}
	l := Normalize(raw)

	if l.Bike != model.BikeRoadGlide {
		t.Errorf("Bike = %q, want road_glide", l.Bike)
	}
	if l.Year == nil || *l.Year != 2015 {
		t.Errorf("Year = %v, want 2015", l.Year)
	}
	if l.PriceCents == nil || *l.PriceCents != 7490000 {
		t.Errorf("PriceCents = %v, want 7490000", l.PriceCents)
	}
}

func TestNormalizeLeavesMissingFieldsNil(t *testing.T) {
	raw := model.RawListing{
		Source:     "olx",
		ExternalID: "999",
		Title:      "Harley Street Glide",
		PriceText:  "a combinar",
	}
	l := Normalize(raw)

	if l.PriceCents != nil {
		t.Errorf("PriceCents = %v, want nil", l.PriceCents)
	}
	if l.Year != nil {
		t.Errorf("Year = %v, want nil", l.Year)
	}
}

func TestFingerprintIsStableAndDiscriminating(t *testing.T) {
	year := 2015
	km := 31200
	base := model.Listing{Bike: model.BikeStreetGlide, Year: &year, Km: &km, City: "curitiba"}

	otherKm := 33000
	sameBucket := base
	sameBucket.Km = &otherKm

	if Fingerprint(base) != Fingerprint(sameBucket) {
		t.Error("listings within the same mileage bucket should share a fingerprint")
	}

	farKm := 90000
	different := base
	different.Km = &farKm
	if Fingerprint(base) == Fingerprint(different) {
		t.Error("listings with very different mileage should not share a fingerprint")
	}
}
