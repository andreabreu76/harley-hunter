package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

type exportEnvelope struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Counts      map[string]int  `json:"counts"`
	Listings    []exportListing `json:"listings"`
}

type exportListing struct {
	ID                 int64      `json:"id"`
	Source             string     `json:"source"`
	ExternalID         string     `json:"external_id"`
	URL                string     `json:"url"`
	Title              string     `json:"title"`
	Bike               string     `json:"bike"`
	Variant            string     `json:"variant"`
	Year               *int       `json:"year"`
	Km                 *int       `json:"km"`
	PriceCents         *int64     `json:"price_cents"`
	City               string     `json:"city"`
	State              string     `json:"state"`
	Phone              *string    `json:"phone"`
	Verdict            string     `json:"verdict"`
	UserState          string     `json:"user_state"`
	Status             string     `json:"status"`
	Fingerprint        string     `json:"fingerprint"`
	Notified           bool       `json:"notified"`
	NotifiedPriceCents *int64     `json:"notified_price_cents"`
	PublishedAt        *time.Time `json:"published_at"`
	FirstSeenAt        time.Time  `json:"first_seen_at"`
	LastSeenAt         time.Time  `json:"last_seen_at"`
}

type exportInput struct {
	Rows   []store.Row
	Counts map[string]int
	Now    time.Time
}

var exportedVerdicts = []model.Verdict{
	model.VerdictMatch, model.VerdictMaybe, model.VerdictReject,
}

func exportVerdicts(value string) ([]model.Verdict, error) {
	switch value {
	case "":
		return []model.Verdict{model.VerdictMatch, model.VerdictMaybe}, nil
	case string(model.VerdictMatch):
		return []model.Verdict{model.VerdictMatch}, nil
	case string(model.VerdictMaybe):
		return []model.Verdict{model.VerdictMaybe}, nil
	case "all":
		return exportedVerdicts, nil
	}
	return nil, fmt.Errorf("unknown verdict filter %q", value)
}

func buildExport(in exportInput) exportEnvelope {
	counts := make(map[string]int, len(exportedVerdicts))
	for _, verdict := range exportedVerdicts {
		counts[string(verdict)] = in.Counts[string(verdict)]
	}

	listings := make([]exportListing, 0, len(in.Rows))
	for _, row := range in.Rows {
		listings = append(listings, exportListingFrom(row))
	}

	return exportEnvelope{GeneratedAt: in.Now.UTC(), Counts: counts, Listings: listings}
}

func exportListingFrom(row store.Row) exportListing {
	return exportListing{
		ID: row.ID, Source: row.Source, ExternalID: row.ExternalID, URL: row.URL,
		Title: row.Title, Bike: row.Bike, Variant: row.Variant, Year: row.Year,
		Km: row.Km, PriceCents: row.PriceCents, City: row.City, State: row.State,
		Phone: row.Phone, Verdict: string(row.Verdict), UserState: row.UserState,
		Status: row.Status, Fingerprint: row.Fingerprint, Notified: row.Notified,
		NotifiedPriceCents: row.NotifiedPriceCents,
		PublishedAt:        utcOrNil(row.PublishedAt),
		FirstSeenAt:        row.FirstSeenAt.UTC(),
		LastSeenAt:         row.LastSeenAt.UTC(),
	}
}

func utcOrNil(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}

func (s *server) exportJSON(w http.ResponseWriter, r *http.Request) {
	verdicts, err := exportVerdicts(r.URL.Query().Get("verdict"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	var rows []store.Row
	for _, verdict := range verdicts {
		batch, err := s.store.ListByVerdict(verdict)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		rows = append(rows, batch...)
	}

	counts, err := s.store.CountByVerdict()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	envelope := buildExport(exportInput{Rows: rows, Counts: counts, Now: time.Now()})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(envelope); err != nil {
		log.Printf("web: encoding export: %v", err)
	}
}
