package fipe

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(body)
}

func TestParseQuoteReadsTheOwnerAnchor(t *testing.T) {
	q, err := ParseQuote(strings.NewReader(readFixture(t, "value-5760-2014.json")))
	if err != nil {
		t.Fatalf("ParseQuote: %v", err)
	}
	if q.Code != "810059-4" {
		t.Errorf("Code = %q, want 810059-4: the Street Glide FLHX the owner watches", q.Code)
	}
	if q.Model != "STREET GLIDE FLHX" {
		t.Errorf("Model = %q", q.Model)
	}
	if q.Year != 2014 {
		t.Errorf("Year = %d, want 2014", q.Year)
	}
	if q.PriceCents != 6920700 {
		t.Errorf("PriceCents = %d, want 6920700 for R$ 69.207,00", q.PriceCents)
	}
	if q.Month != "agosto de 2026" {
		t.Errorf("Month = %q", q.Month)
	}
}

func TestParseQuoteReadsEveryCommittedFixture(t *testing.T) {
	cases := []struct {
		file  string
		code  string
		cents int64
	}{
		{"value-5760-2013.json", "810059-4", 6751900},
		{"value-7057-2015.json", "810071-3", 7464500},
		{"value-7057-2016.json", "810071-3", 7735700},
		{"value-7053-2015.json", "810070-5", 8406200},
		{"value-5761-2013.json", "810060-8", 6554800},
		{"value-5761-2016.json", "810060-8", 7482900},
	}
	for _, c := range cases {
		q, err := ParseQuote(strings.NewReader(readFixture(t, c.file)))
		if err != nil {
			t.Errorf("%s: ParseQuote: %v", c.file, err)
			continue
		}
		if q.Code != c.code || q.PriceCents != c.cents {
			t.Errorf("%s: got %s/%d, want %s/%d", c.file, q.Code, q.PriceCents, c.code, c.cents)
		}
	}
}

func TestParseQuoteRejectsTheNotFoundBody(t *testing.T) {
	if _, err := ParseQuote(strings.NewReader(readFixture(t, "value-notfound.json"))); err == nil {
		t.Error("ParseQuote accepted the not-found body, want an error so nothing is stored")
	}
}

func TestModelTableNamesMatchWhatTheAPIPublishes(t *testing.T) {
	var published []struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(readFixture(t, "models.json")), &published); err != nil {
		t.Fatalf("reading model list: %v", err)
	}
	byCode := make(map[string]string, len(published))
	for _, m := range published {
		byCode[m.Code] = m.Name
	}
	bikesOfCode := make(map[string]map[string]bool, len(models))
	for _, ref := range models {
		if bikesOfCode[ref.ModelCode] == nil {
			bikesOfCode[ref.ModelCode] = make(map[string]bool)
		}
		bikesOfCode[ref.ModelCode][ref.Bike] = true
	}

	for _, ref := range models {
		name, ok := byCode[ref.ModelCode]
		if !ok {
			t.Errorf("%s/%s: model code %s is not in the brand list", ref.Bike, ref.Variant, ref.ModelCode)
			continue
		}
		if !strings.Contains(normalize.Fold(name), strings.ToLower(ref.Label)) {
			t.Errorf("%s/%s: code %s is %q, which does not carry the label %q",
				ref.Bike, ref.Variant, ref.ModelCode, name, ref.Label)
		}
		bike, variant := normalize.DetectBike(name)
		if !bikesOfCode[ref.ModelCode][bike] {
			t.Errorf("%s/%s: the api name %q classifies as %q, which no entry maps to code %s",
				ref.Bike, ref.Variant, name, bike, ref.ModelCode)
		}
		if ref.Variant != model.VariantUnknown && variant != ref.Variant {
			t.Errorf("%s/%s: the api name %q classifies as variant %q", ref.Bike, ref.Variant, name, variant)
		}
	}
}

func TestSharedModelCodeIsOnlyForTheSameMotorcycle(t *testing.T) {
	byCode := make(map[string][]string)
	for _, ref := range models {
		byCode[ref.ModelCode] = append(byCode[ref.ModelCode], ref.Bike+"/"+ref.Variant)
	}
	shared := byCode["5761"]
	if len(shared) != 2 {
		t.Fatalf("code 5761 maps %v, want exactly electra glide and ultra", shared)
	}
	for _, ref := range models {
		if ref.ModelCode == "5761" && ref.Variant != model.VariantUnknown {
			t.Errorf("%s maps to 5761 with variant %q: the shared code is only honest while the trim is unknown",
				ref.Bike, ref.Variant)
		}
	}
}

func TestModelTableHasNoDuplicateCombos(t *testing.T) {
	seen := make(map[string]bool, len(models))
	for _, ref := range models {
		key := ref.Bike + "|" + ref.Variant
		if seen[key] {
			t.Errorf("combo %s appears twice: the lookup would be ambiguous", key)
		}
		seen[key] = true
	}
}
