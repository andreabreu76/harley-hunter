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
	row, ok := t.rows[key(bike, variant, year)]
	if !ok && variant == model.VariantUnknown {
		row, ok = t.rows[key(bike, model.VariantBase, year)]
	}
	if !ok {
		return nil, false
	}
	row.Base = variant == model.VariantUnknown
	return &row, true
}

func key(bike, variant string, year int) string {
	return bike + "|" + variant + "|" + strconv.Itoa(year)
}
