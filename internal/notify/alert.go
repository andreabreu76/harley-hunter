package notify

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/andreabreu76/harley-hunter/internal/format"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func FormatAlert(r store.Row) string {
	price := "preço não informado"
	if r.PriceCents != nil {
		price = "R$ " + format.Thousands(*r.PriceCents/100)
	}
	title := strings.Join(strings.Fields(r.Title), " ")
	year := ""
	if r.Year != nil {
		if stamp := strconv.Itoa(*r.Year); !strings.Contains(title, stamp) {
			year = " " + stamp
		}
	}
	location := r.City
	if r.State != "" {
		if location == "" {
			location = r.State
		} else {
			location += "/" + r.State
		}
	}

	fields := []string{strings.TrimSpace(title + year), price}
	if location != "" {
		fields = append(fields, location)
	}
	return fmt.Sprintf("%s [%s] %s", strings.Join(fields, " - "), r.Source, r.URL)
}
