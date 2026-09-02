package fipe

import (
	"strconv"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

type Reference struct {
	Code       string
	Label      string
	Bike       string
	Variant    string
	Year       int
	PriceCents int64
	Month      string
	Base       bool
}

type Table struct {
	rows map[string]Reference
}

func NewTable(refs []Reference) *Table {
	rows := make(map[string]Reference, len(refs))
	for _, r := range refs {
		rows[key(r.Bike, r.Variant, r.Year)] = r
	}
	return &Table{rows: rows}
}

func (t *Table) Lookup(bike, variant string, year int) (*Reference, bool) {
	if t == nil || bike == "" || year == 0 {
		return nil, false
	}
	if row, ok := t.rows[key(bike, variant, year)]; ok {
		row.Base = variant == model.VariantUnknown
		return &row, true
	}
	if variant != model.VariantUnknown && variant != model.VariantBase {
		return nil, false
	}
	row, ok := t.cheapestOf(bike, year)
	if !ok {
		return nil, false
	}
	row.Base = true
	return &row, true
}

func (t *Table) cheapestOf(bike string, year int) (Reference, bool) {
	var cheapest Reference
	found := false
	for _, row := range t.rows {
		if row.Bike != bike || row.Year != year {
			continue
		}
		if !found || row.PriceCents < cheapest.PriceCents ||
			(row.PriceCents == cheapest.PriceCents && row.Code < cheapest.Code) {
			cheapest, found = row, true
		}
	}
	return cheapest, found
}

func key(bike, variant string, year int) string {
	return bike + "|" + variant + "|" + strconv.Itoa(year)
}
