package notify

import (
	"strings"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/store"
)

func TestFormatAlertIncludesEssentials(t *testing.T) {
	year := 2015
	cents := int64(7200000)
	row := store.Row{
		Title:      "Harley Davidson Street Glide Special",
		Year:       &year,
		PriceCents: &cents,
		City:       "curitiba",
		State:      "PR",
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	for _, want := range []string{"Street Glide", "2015", "72.000", "curitiba"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q is missing %q", msg, want)
		}
	}
	if strings.Contains(msg, "https://olx.com.br/abc") {
		t.Errorf("message %q still carries the url; the transport opens it on click now", msg)
	}
	if len(msg) > 160 {
		t.Errorf("message is %d chars, well past what a notification banner shows before truncating", len(msg))
	}
}

func TestFormatAlertDoesNotRepeatAYearAlreadyInTheTitle(t *testing.T) {
	year := 2014
	cents := int64(7200000)
	row := store.Row{
		Title:      "Harley-Davidson Street Glide 2014",
		Year:       &year,
		PriceCents: &cents,
		City:       "curitiba",
		State:      "PR",
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	if strings.Contains(msg, "2014 2014") {
		t.Errorf("message %q repeats the year already present in the title", msg)
	}
	if !strings.Contains(msg, "2014") {
		t.Errorf("message %q lost the year entirely", msg)
	}
}

func TestFormatAlertCollapsesRepeatedSpacesInTheTitle(t *testing.T) {
	year := 2014
	cents := int64(7190000)
	row := store.Row{
		Title:      "HARLEY DAVDSON FLHX STREET GLIDE  AZUL -2014 90.195Km",
		Year:       &year,
		PriceCents: &cents,
		City:       "curitiba",
		State:      "PR",
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	if strings.Contains(msg, "  ") {
		t.Errorf("message %q keeps the double space the listing title carried", msg)
	}
}

func TestFormatAlertWithoutPriceOrYear(t *testing.T) {
	row := store.Row{
		Title:  "Harley Davidson Road Glide",
		City:   "sao paulo",
		State:  "SP",
		URL:    "https://mercadolivre.com.br/xyz",
		Source: "mercadolivre",
	}
	msg := FormatAlert(row)

	if !strings.Contains(msg, "preço não informado") {
		t.Errorf("message %q should say the price is missing", msg)
	}
	if strings.Contains(msg, "  ") {
		t.Errorf("message %q has a double space left by the absent year", msg)
	}
	for _, want := range []string{"Road Glide", "sao paulo/SP"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q is missing %q", msg, want)
		}
	}
}

func TestFormatAlertWithoutLocation(t *testing.T) {
	year := 2014
	cents := int64(6500000)
	row := store.Row{
		Title:      "Harley Davidson Street Glide",
		Year:       &year,
		PriceCents: &cents,
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	if strings.Contains(msg, "  ") {
		t.Errorf("message %q has a blank gap left by the absent location", msg)
	}
	if strings.Contains(msg, "- -") {
		t.Errorf("message %q has an empty field between separators", msg)
	}
}

func TestFormatAlertWithStateOnly(t *testing.T) {
	year := 2014
	cents := int64(6500000)
	row := store.Row{
		Title:      "Harley Davidson Street Glide",
		Year:       &year,
		PriceCents: &cents,
		State:      "PR",
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	if strings.Contains(msg, " /PR") {
		t.Errorf("message %q starts the location with a bare separator", msg)
	}
	if !strings.Contains(msg, "PR") {
		t.Errorf("message %q lost the state", msg)
	}
}
