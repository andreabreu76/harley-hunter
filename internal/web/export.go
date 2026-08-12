package web

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/fipe"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

type exportEnvelope struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Counts      map[string]int  `json:"counts"`
	Fipe        []exportFipeRef `json:"fipe"`
	Listings    []exportListing `json:"listings"`
}

type exportFipeRef struct {
	Label      string `json:"label"`
	Bike       string `json:"bike"`
	Variant    string `json:"variant"`
	Year       int    `json:"year"`
	PriceCents int64  `json:"price_cents"`
	Month      string `json:"month"`
}

type exportFipe struct {
	Label       string   `json:"label"`
	Year        int      `json:"year"`
	PriceCents  int64    `json:"price_cents"`
	GapPercent  *float64 `json:"gap_percent"`
	BelowFipe   *bool    `json:"below_fipe"`
	BaseVariant bool     `json:"base_variant"`
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

	PriceDropCents *int64             `json:"price_drop_cents"`
	PriceHistory   []exportPricePoint `json:"price_history"`
	Fipe           *exportFipe        `json:"fipe"`
	Reposts        []exportRepost     `json:"reposts"`
}

type exportRepost struct {
	ID          int64     `json:"id"`
	Source      string    `json:"source"`
	Verdict     string    `json:"verdict"`
	PriceCents  *int64    `json:"price_cents"`
	Km          *int      `json:"km"`
	FirstSeenAt time.Time `json:"first_seen_at"`
}

type exportPricePoint struct {
	PriceCents int64     `json:"price_cents"`
	At         time.Time `json:"at"`
}

type exportInput struct {
	Rows     []store.Row
	History  map[int64][]store.PricePoint
	Refs     *fipe.Table
	FipeRows []fipe.Reference
	Groups   map[string][]store.Row
	Counts   map[string]int
	Now      time.Time
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

	references := make([]exportFipeRef, 0, len(in.FipeRows))
	for _, r := range in.FipeRows {
		references = append(references, exportFipeRef{
			Label: r.Label, Bike: r.Bike, Variant: r.Variant,
			Year: r.Year, PriceCents: r.PriceCents, Month: r.Month,
		})
	}

	listings := make([]exportListing, 0, len(in.Rows))
	for _, row := range in.Rows {
		listings = append(listings, exportListingFrom(
			row, in.History[row.ID], in.Refs, siblingsOf(row, in.Groups)))
	}

	return exportEnvelope{
		GeneratedAt: in.Now.UTC(), Counts: counts,
		Fipe: references, Listings: listings,
	}
}

func exportListingFrom(row store.Row, history []store.PricePoint, refs *fipe.Table, siblings []store.Row) exportListing {
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
		PriceDropCents:     priceDropCents(row),
		PriceHistory:       exportPricePoints(history),
		Fipe:               exportFipeOf(row, refs),
		Reposts:            exportReposts(siblings),
	}
}

func exportReposts(siblings []store.Row) []exportRepost {
	reposts := make([]exportRepost, 0, len(siblings))
	for _, s := range siblings {
		reposts = append(reposts, exportRepost{
			ID: s.ID, Source: s.Source, Verdict: string(s.Verdict),
			PriceCents: s.PriceCents, Km: s.Km, FirstSeenAt: s.FirstSeenAt.UTC(),
		})
	}
	return reposts
}

func exportFipeOf(row store.Row, refs *fipe.Table) *exportFipe {
	if row.Year == nil {
		return nil
	}
	reference, ok := refs.Lookup(row.Bike, row.Variant, *row.Year)
	if !ok {
		return nil
	}

	view := &exportFipe{
		Label: reference.Label, Year: reference.Year,
		PriceCents: reference.PriceCents, BaseVariant: reference.Base,
	}
	if row.PriceCents == nil || reference.PriceCents <= 0 {
		return view
	}

	gap := float64(reference.PriceCents-*row.PriceCents) * 100 / float64(reference.PriceCents)
	percent := math.Round(math.Abs(gap)*10) / 10
	below := gap > 0
	view.GapPercent = &percent
	view.BelowFipe = &below
	return view
}

func priceDropCents(row store.Row) *int64 {
	if row.PriceCents == nil || row.FirstPriceCents == nil {
		return nil
	}
	drop := *row.FirstPriceCents - *row.PriceCents
	if drop <= 0 {
		return nil
	}
	return &drop
}

func exportPricePoints(points []store.PricePoint) []exportPricePoint {
	exported := make([]exportPricePoint, 0, len(points))
	for _, p := range points {
		exported = append(exported, exportPricePoint{PriceCents: p.PriceCents, At: p.ObservedAt.UTC()})
	}
	return exported
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

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	history, err := s.store.PriceHistoryFor(ids)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	references, err := s.store.FipeReferences()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	groups, err := s.store.RepostGroups()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	envelope := buildExport(exportInput{
		Rows: rows, History: history, Refs: fipe.NewTable(references),
		FipeRows: references, Groups: groups, Counts: counts, Now: time.Now(),
	})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(envelope); err != nil {
		log.Printf("web: encoding export: %v", err)
	}
}
