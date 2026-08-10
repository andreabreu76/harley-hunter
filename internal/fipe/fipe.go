package fipe

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

type Quote struct {
	Code       string
	Model      string
	Year       int
	PriceCents int64
	Month      string
}

type quotePayload struct {
	Price     string `json:"price"`
	Model     string `json:"model"`
	ModelYear int    `json:"modelYear"`
	CodeFipe  string `json:"codeFipe"`
	Month     string `json:"referenceMonth"`
	Error     string `json:"error"`
}

func ParseQuote(body io.Reader) (Quote, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return Quote{}, fmt.Errorf("reading fipe response: %w", err)
	}

	var payload quotePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Quote{}, fmt.Errorf("fipe response is not the quote json: %w", err)
	}
	if payload.Error != "" {
		return Quote{}, fmt.Errorf("fipe has no quote: %s", payload.Error)
	}
	if payload.CodeFipe == "" || payload.ModelYear == 0 {
		return Quote{}, errors.New("fipe quote is missing its code or year")
	}

	cents, err := parseCents(payload.Price)
	if err != nil {
		return Quote{}, err
	}
	return Quote{
		Code:       payload.CodeFipe,
		Model:      strings.TrimSpace(payload.Model),
		Year:       payload.ModelYear,
		PriceCents: cents,
		Month:      strings.TrimSpace(payload.Month),
	}, nil
}

func parseCents(price string) (int64, error) {
	digits := strings.NewReplacer("R$", "", " ", "", " ", "", ".", "").Replace(price)
	digits = strings.ReplaceAll(digits, ",", ".")
	value, err := strconv.ParseFloat(strings.TrimSpace(digits), 64)
	if err != nil {
		return 0, fmt.Errorf("fipe price %q is not a number: %w", price, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("fipe price %q is not positive", price)
	}
	return int64(value*100 + 0.5), nil
}

type modelRef struct {
	Bike      string
	Variant   string
	ModelCode string
	Label     string
}

var models = []modelRef{
	{model.BikeStreetGlide, model.VariantBase, "5760", "FLHX"},
	{model.BikeStreetGlide, model.VariantSpecial, "7057", "FLHXS"},
	{model.BikeStreetGlide, model.VariantCVO, "7053", "FLHXSE"},
	{model.BikeRoadGlide, model.VariantBase, "11056", "FLTRX"},
	{model.BikeRoadGlide, model.VariantSpecial, "8139", "FLTRXS"},
	{model.BikeRoadGlide, model.VariantCVO, "8135", "FLTRXSE"},
	{model.BikeElectraGlide, model.VariantUnknown, "5761", "FLHTK"},
	{model.BikeUltra, model.VariantUnknown, "5761", "FLHTK"},
}
