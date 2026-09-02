package normalize

import (
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestNormalizeUsesStructuredFieldsFirst(t *testing.T) {
	raw := model.RawListing{
		Source:       "olx",
		ExternalID:   "123",
		Title:        "Harley Davidson Sportster 1200 Custom",
		PriceText:    "R$ 72.000",
		YearText:     "2015",
		KmText:       "31.000 km",
		LocationText: "Curitiba - PR",
	}
	l := Normalize(raw)

	if l.Bike != model.BikeSportster1200 || l.Variant != model.VariantCustom {
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
		RawText:    "Vendo Iron 1200 19/19, 42.000 km, R$ 44.900, Sao Paulo SP",
	}
	l := Normalize(raw)

	if l.Bike != model.BikeSportster1200 {
		t.Errorf("Bike = %q, want sportster_1200", l.Bike)
	}
	if l.Year == nil || *l.Year != 2019 {
		t.Errorf("Year = %v, want 2019", l.Year)
	}
	if l.PriceCents == nil || *l.PriceCents != 4490000 {
		t.Errorf("PriceCents = %v, want 4490000", l.PriceCents)
	}
}

func TestNormalizeLeavesMissingFieldsNil(t *testing.T) {
	raw := model.RawListing{
		Source:     "olx",
		ExternalID: "999",
		Title:      "Harley Sportster 1200",
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

func TestNormalizeResolvesCityDeterministically(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "det1",
		RawText:    "Sportster 1200 2015 em Sao Jose dos Pinhais, aceito troca",
	}
	first := Normalize(raw)
	for i := 0; i < 50; i++ {
		if got := Normalize(raw); got.City != first.City || got.Fingerprint != first.Fingerprint {
			t.Fatalf("run %d gave %q/%s, first gave %q/%s", i, got.City, got.Fingerprint, first.City, first.Fingerprint)
		}
	}
	if first.City != "sao jose dos pinhais" {
		t.Errorf("City = %q, want sao jose dos pinhais", first.City)
	}
}

func TestNormalizeCityMatchesWholeWordsOnly(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "word1",
		RawText:    "Sportster 1200 2015, mais imagens no WhatsApp, Rio de Janeiro RJ",
	}
	if l := Normalize(raw); l.City != "rio de janeiro" {
		t.Errorf("City = %q, want rio de janeiro", l.City)
	}
}

func TestNormalizePrefersLongestCityMatch(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "long1",
		RawText:    "Sportster 1200 2015, moto na Lapa, Sao Paulo capital",
	}
	l := Normalize(raw)
	if l.City != "sao paulo" || l.State != "SP" {
		t.Errorf("location = %q/%q, want sao paulo/SP", l.City, l.State)
	}
}

func TestNormalizeIgnoresFiscalYears(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "fiscal1",
		RawText:    "IPVA 2026 pago. Vendo Iron 1200 2015, Curitiba - PR",
	}
	l := Normalize(raw)
	if l.Year == nil || *l.Year != 2015 {
		t.Errorf("Year = %v, want 2015", l.Year)
	}
}

func TestNormalizeReadsPriceAfterMileage(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "price1",
		RawText:    "Sportster 1200 2016, 42 mil km, valor 44 mil, Curitiba - PR",
	}
	l := Normalize(raw)
	if l.PriceCents == nil || *l.PriceCents != 4400000 {
		t.Errorf("PriceCents = %v, want 4400000", l.PriceCents)
	}
}

func TestNormalizeFindsCityWrittenWithAttachedState(t *testing.T) {
	cases := []struct {
		text string
		city string
	}{
		{"Sportster 1200 2015, moto em Curitiba-PR, aceito troca", "curitiba"},
		{"Iron 1200 2015 (Guarulhos-SP) impecavel", "guarulhos"},
		{"Sportster 1200 2014, Embu-Guacu SP", "embu guacu"},
		{"Sportster 1200 2015, entrega em Sao-Jose-dos-Pinhais", "sao jose dos pinhais"},
	}
	for _, c := range cases {
		t.Run(c.city, func(t *testing.T) {
			l := Normalize(model.RawListing{Source: "instagram", ExternalID: c.city, RawText: c.text})
			if l.City != c.city {
				t.Errorf("City = %q, want %q", l.City, c.city)
			}
		})
	}
}

func TestNormalizeIgnoresMoreFiscalYearShapes(t *testing.T) {
	cases := []string{
		"IPVA/2026 pago. Sportster 1200 2015, Curitiba - PR",
		"Documento 2026 ok. Iron 1200 2015, Curitiba - PR",
		"Emplacada 2026. Sportster 1200 2015, Curitiba - PR",
		"Documentação 2026 em dia. Sportster 1200 2015, Curitiba - PR",
		"Documentos 2026 ok. Iron 1200 2015, Curitiba - PR",
		"IPVA 2026 PAGO. VENDO SPORTSTER 1200 2015, CURITIBA-PR",
	}
	for _, text := range cases {
		t.Run(string([]rune(text)[:12]), func(t *testing.T) {
			l := Normalize(model.RawListing{Source: "instagram", ExternalID: string([]rune(text)[:8]), RawText: text})
			if l.Year == nil || *l.Year != 2015 {
				t.Errorf("Year = %v, want 2015", l.Year)
			}
		})
	}
}

func TestFingerprintIsStableAndDiscriminating(t *testing.T) {
	year := 2015
	km := 31200
	base := model.Listing{Bike: model.BikeSportster1200, Year: &year, Km: &km, City: "curitiba"}

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

func TestNormalizeReadsTheSellerPhoneFromTheAdText(t *testing.T) {
	raw := model.RawListing{
		Source:     "webmotors",
		ExternalID: "2977981",
		Title:      "HARLEY-DAVIDSON SPORTSTER 1200",
		RawText:    "Interessados chame nesse contato: 11 98241-3574 Wilson",
	}
	l := Normalize(raw)

	if l.Phone == nil || *l.Phone != "11982413574" {
		t.Errorf("Phone = %v, want 11982413574", l.Phone)
	}
}

func TestNormalizeLeavesThePhoneNilWhenTheAdHasNone(t *testing.T) {
	raw := model.RawListing{
		Source:     "webmotors",
		ExternalID: "3020437",
		Title:      "HARLEY-DAVIDSON SPORTSTER 1200",
		RawText:    "Impecável. Simplesmente sem detalhes. 43.500 km por R$ 44.900",
	}
	l := Normalize(raw)

	if l.Phone != nil {
		t.Errorf("Phone = %q, want nil", *l.Phone)
	}
}

func TestNormalizeCarriesThePublishedDateInUTC(t *testing.T) {
	brt := time.FixedZone("BRT", -3*3600)
	published := time.Date(2026, 8, 4, 9, 49, 53, 0, brt)
	raw := model.RawListing{
		Source:      "olx",
		ExternalID:  "1499153946",
		Title:       "Harley Sportster 1200",
		PublishedAt: &published,
	}
	l := Normalize(raw)

	if l.PublishedAt == nil {
		t.Fatal("PublishedAt is nil, want the date the source published")
	}
	if !l.PublishedAt.Equal(published) {
		t.Errorf("PublishedAt = %s, want %s", l.PublishedAt, published)
	}
	if l.PublishedAt.Location() != time.UTC {
		t.Errorf("PublishedAt zone = %s, want UTC", l.PublishedAt.Location())
	}
}

func TestNormalizeLeavesThePublishedDateNilWhenTheSourceOmitsIt(t *testing.T) {
	l := Normalize(model.RawListing{Source: "mobiauto", ExternalID: "31389122", Title: "Harley Sportster 1200"})
	if l.PublishedAt != nil {
		t.Errorf("PublishedAt = %v, want nil", l.PublishedAt)
	}
}

func TestNormalizeReadsAStyledUnicodeCaption(t *testing.T) {
	l := Normalize(model.RawListing{
		Source:     "instagram",
		ExternalID: "styled",
		RawText:    "𝐈𝐫𝐨𝐧 𝟏𝟐𝟎𝟎 𝟐𝟎𝟏𝟗 em 𝐂𝐮𝐫𝐢𝐭𝐢𝐛𝐚",
	})

	if l.Bike != model.BikeSportster1200 || l.Variant != model.VariantIron {
		t.Errorf("bike/variant = %q/%q, want sportster_1200/iron", l.Bike, l.Variant)
	}
	if l.Year == nil || *l.Year != 2019 {
		t.Errorf("Year = %v, want 2019 read through the folded text", l.Year)
	}
	if l.City != "curitiba" {
		t.Errorf("City = %q, want curitiba", l.City)
	}
}
