package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/crawl"
	"github.com/andreabreu76/harley-hunter/internal/format"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

const healthHistoryRuns = 30

var regionPriority = map[string]int{"RJ": 0, "SP": 1, "PR": 2}

type server struct {
	store      *store.Store
	sources    []string
	listTmpl   *template.Template
	detailTmpl *template.Template
	healthTmpl *template.Template
}

type sourceHealth struct {
	Name   string
	Status string
	Counts string
}

func NewServer(s *store.Store, sources []string) http.Handler {
	funcs := template.FuncMap{
		"money":      func(cents *int64) string { return format.Thousands(*cents / 100) },
		"moneyCents": func(cents int64) string { return format.Thousands(cents / 100) },
		"priceDrop":  priceDrop,
		"location":   location,
		"datetime":   datetime,
	}

	parse := func(page string) *template.Template {
		return template.Must(template.New("layout").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.html", "templates/"+page))
	}

	srv := &server{
		store:      s,
		sources:    sources,
		listTmpl:   parse("list.html"),
		detailTmpl: parse("detail.html"),
		healthTmpl: parse("health.html"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", srv.list(model.VerdictMatch, "Match"))
	mux.HandleFunc("GET /maybe", srv.list(model.VerdictMaybe, "Talvez"))
	mux.HandleFunc("GET /rejected", srv.list(model.VerdictReject, "Descartados"))
	mux.HandleFunc("GET /listing/{id}", srv.detail)
	mux.HandleFunc("POST /listing/{id}/state", srv.setState)
	mux.HandleFunc("GET /health", srv.health)
	return mux
}

func (s *server) list(v model.Verdict, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.store.ListByVerdict(v)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sortByRegionPriority(rows)
		s.render(w, s.listTmpl, map[string]any{"Title": title, "Rows": rows})
	}
}

func sortByRegionPriority(rows []store.Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		return regionRank(rows[i].State) < regionRank(rows[j].State)
	})
}

func regionRank(state string) int {
	if rank, ok := regionPriority[state]; ok {
		return rank
	}
	return len(regionPriority)
}

func (s *server) detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	row, points, err := s.store.GetRow(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.render(w, s.detailTmpl, map[string]any{"Title": row.Title, "Row": row, "Points": points})
}

func (s *server) setState(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	value := r.URL.Query().Get("value")
	if value != "contacted" && value != "dismissed" && value != "new" {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	if _, _, err := s.store.GetRow(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := s.store.SetUserState(id, value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/listing/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	var items []sourceHealth
	for _, name := range s.sources {
		counts, err := s.store.RecentRunCounts(name, healthHistoryRuns)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var parts []string
		for _, c := range counts {
			parts = append(parts, strconv.Itoa(c))
		}
		items = append(items, sourceHealth{
			Name:   name,
			Status: crawl.HealthStatus(counts),
			Counts: strings.Join(parts, ", "),
		})
	}
	s.render(w, s.healthTmpl, map[string]any{"Title": "Saúde das fontes", "Sources": items})
}

func (s *server) render(w http.ResponseWriter, t *template.Template, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func priceDrop(r store.Row) string {
	if r.PriceCents == nil || r.FirstPriceCents == nil {
		return ""
	}
	diff := *r.FirstPriceCents - *r.PriceCents
	if diff <= 0 {
		return ""
	}
	return fmt.Sprintf(" (baixou R$ %s)", format.Thousands(diff/100))
}

func location(r store.Row) string {
	if r.City == "" {
		return r.State
	}
	if r.State == "" {
		return r.City
	}
	return r.City + "/" + r.State
}

func datetime(t time.Time) string {
	return t.In(time.Local).Format("02/01/2006 15:04")
}
