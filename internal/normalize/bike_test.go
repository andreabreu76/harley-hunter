package normalize

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestDetectBike(t *testing.T) {
	cases := []struct {
		in          string
		wantBike    string
		wantVariant string
	}{
		{"Harley Davidson Street Glide 2015", model.BikeStreetGlide, model.VariantBase},
		{"HD STREET GLIDE ESPECIAL 15/15 IMPECAVEL", model.BikeStreetGlide, model.VariantSpecial},
		{"Street Glide Special 2014", model.BikeStreetGlide, model.VariantSpecial},
		{"streetglide 2015", model.BikeStreetGlide, model.VariantBase},
		{"Harley FLHXS 2015", model.BikeStreetGlide, model.VariantSpecial},
		{"CVO Street Glide 2015", model.BikeStreetGlide, model.VariantCVO},
		{"Road Glide Special 2015 - valor a combinar", model.BikeRoadGlide, model.VariantSpecial},
		{"roadglide 2015", model.BikeRoadGlide, model.VariantBase},
		{"Harley FLTRXSE 2015", model.BikeRoadGlide, model.VariantCVO},
		{"Electra Glide Ultra Limited 2015", model.BikeElectraGlide, model.VariantUnknown},
		{"Harley Davidson Ultra Limited 2014", model.BikeUltra, model.VariantUnknown},
		{"Harley Davidson Touring 1690 2015", model.BikeTouringUnknown, model.VariantUnknown},
		{"Honda Gold Wing 2015", model.BikeOther, model.VariantUnknown},
		{"Harley Davidson Iron 883", model.BikeOther, model.VariantUnknown},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			bike, variant := DetectBike(c.in)
			if bike != c.wantBike {
				t.Errorf("DetectBike(%q) bike = %q, want %q", c.in, bike, c.wantBike)
			}
			if variant != c.wantVariant {
				t.Errorf("DetectBike(%q) variant = %q, want %q", c.in, variant, c.wantVariant)
			}
		})
	}
}

func TestFoldRemovesAccents(t *testing.T) {
	if got := Fold("SÃO JOSÉ DOS PINHAIS"); got != "sao jose dos pinhais" {
		t.Errorf("Fold = %q, want %q", got, "sao jose dos pinhais")
	}
}
