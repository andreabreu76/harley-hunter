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
	q, err := ParseQuote(strings.NewReader(readFixture(t, "value-6719-2016.json")))
	if err != nil {
		t.Fatalf("ParseQuote: %v", err)
	}
	if q.Code != "810066-7" {
		t.Errorf("Code = %q, want 810066-7: the cheapest Sportster 1200 the owner can still finance", q.Code)
	}
	if q.Model != "XL 1200X FORTY EIGHT SPORTSTER" {
		t.Errorf("Model = %q", q.Model)
	}
	if q.Year != 2016 {
		t.Errorf("Year = %d, want 2016", q.Year)
	}
	if q.PriceCents != 4698800 {
		t.Errorf("PriceCents = %d, want 4698800 for R$ 46.988,00", q.PriceCents)
	}
	if q.Month == "" {
		t.Error("Month is empty, want the reference month fipe published")
	}
}

func TestParseQuoteReadsEveryCommittedFixture(t *testing.T) {
	cases := []struct {
		file  string
		code  string
		cents int64
	}{
		{"value-6719-2017.json", "810066-7", 4816300},
		{"value-6719-2019.json", "810066-7", 5452200},
		{"value-6820-2016.json", "810067-5", 4712700},
		{"value-6820-2017.json", "810067-5", 4830700},
		{"value-7863-2017.json", "810075-6", 4862800},
		{"value-7863-2018.json", "810075-6", 4987800},
		{"value-8556-2019.json", "810099-3", 4963500},
		{"value-8556-2020.json", "810099-3", 5584700},
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

func TestNoModelCodeIsSharedByTwoTrims(t *testing.T) {
	byCode := make(map[string][]string)
	for _, ref := range models {
		byCode[ref.ModelCode] = append(byCode[ref.ModelCode], ref.Bike+"/"+ref.Variant)
	}
	for code, combos := range byCode {
		if len(combos) != 1 {
			t.Errorf("code %s maps %v, want one combo: two trims behind one quote would price the wrong bike", code, combos)
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
