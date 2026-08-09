package web

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
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

var watchedRegions = []struct {
	Code string
	Name string
}{
	{"RJ", "Rio de Janeiro"},
	{"SP", "São Paulo"},
	{"PR", "Curitiba"},
}

const otherRegionName = "Outras regiões"

type server struct {
	store      *store.Store
	sources    []string
	listTmpl   *template.Template
	detailTmpl *template.Template
	healthTmpl *template.Template
}

type regionGroup struct {
	Name    string
	Rows    []store.Row
	Scanned int
}

type sourceHealth struct {
	Name   string
	Status string
	Last   string
	Counts string
}

type sourceLight struct {
	Name   string
	Status string
	When   string
}

type listView struct {
	verdict model.Verdict
	title   string
	nav     string
	empty   string
}

func NewServer(s *store.Store, sources []string) http.Handler {
	funcs := template.FuncMap{
		"money":      func(cents *int64) string { return format.Thousands(*cents / 100) },
		"moneyCents": func(cents int64) string { return format.Thousands(cents / 100) },
		"km":         func(value *int) string { return format.Thousands(int64(*value)) },
		"priceDrop":  priceDrop,
		"location":   location,
		"stateLabel": stateLabel,
		"scanned":    scannedLabel,
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
	mux.HandleFunc("GET /{$}", srv.list(listView{
		verdict: model.VerdictMatch, title: "Match", nav: "/",
		empty: "Nenhum anúncio no alvo — monitorando a cada rodada",
	}))
	mux.HandleFunc("GET /maybe", srv.list(listView{
		verdict: model.VerdictMaybe, title: "Talvez", nav: "/maybe",
		empty: "Nenhum anúncio na fronteira do alvo — monitorando a cada rodada",
	}))
	mux.HandleFunc("GET /rejected", srv.list(listView{
		verdict: model.VerdictReject, title: "Descartados", nav: "/rejected",
		empty: "Nada descartado por aqui — monitorando a cada rodada",
	}))
	mux.HandleFunc("GET /listing/{id}", srv.detail)
	mux.HandleFunc("POST /listing/{id}/state", srv.setState)
	mux.HandleFunc("GET /health", srv.health)
	return mux
}

func (s *server) list(view listView) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.store.ListByVerdict(view.verdict)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		scanned, err := s.store.CountByState()
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		s.render(w, s.listTmpl, map[string]any{
			"Title":  view.title,
			"Nav":    view.nav,
			"Groups": groupByRegion(rows, scanned),
			"Empty":  view.empty,
		})
	}
}

func groupByRegion(rows []store.Row, scanned map[string]int) []regionGroup {
	groups := make([]regionGroup, 0, len(watchedRegions)+1)
	watched := make(map[string]bool, len(watchedRegions))
	for _, region := range watchedRegions {
		watched[region.Code] = true
		groups = append(groups, regionGroup{Name: region.Name, Scanned: scanned[region.Code]})
	}

	other := regionGroup{Name: otherRegionName}
	for state, count := range scanned {
		if !watched[state] {
			other.Scanned += count
		}
	}

	for _, row := range rows {
		placed := false
		for i, region := range watchedRegions {
			if row.State == region.Code {
				groups[i].Rows = append(groups[i].Rows, row)
				placed = true
				break
			}
		}
		if !placed {
			other.Rows = append(other.Rows, row)
		}
	}
	if len(other.Rows) > 0 {
		groups = append(groups, other)
	}
	return groups
}

func (s *server) detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	row, points, err := s.store.GetRow(id)
	if err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	s.render(w, s.detailTmpl, map[string]any{"Title": row.Title, "Nav": "", "Row": row, "Points": points})
}

func (s *server) setState(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	value := r.URL.Query().Get("value")
	if value != "contacted" && value != "dismissed" && value != "new" {
		fail(w, http.StatusBadRequest, fmt.Errorf("unknown user state %q", value))
		return
	}
	if _, _, err := s.store.GetRow(id); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	if err := s.store.SetUserState(id, value); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	http.Redirect(w, r, "/listing/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	items := make([]sourceHealth, 0, len(s.sources))
	for _, name := range s.sources {
		counts, err := s.store.RecentRunCounts(name, healthHistoryRuns)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		last, ok, err := s.store.LastRunAt(name)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		parts := make([]string, 0, len(counts))
		for _, c := range counts {
			parts = append(parts, strconv.Itoa(c))
		}
		when := "nunca"
		if ok {
			when = datetime(last)
		}
		items = append(items, sourceHealth{
			Name:   name,
			Status: crawl.HealthStatus(counts),
			Last:   when,
			Counts: strings.Join(parts, ", "),
		})
	}
	s.render(w, s.healthTmpl, map[string]any{
		"Title": "Saúde das fontes", "Nav": "/health", "Sources": items,
	})
}

func (s *server) lights() []sourceLight {
	lights := make([]sourceLight, 0, len(s.sources))
	for _, name := range s.sources {
		counts, err := s.store.RecentRunCounts(name, healthHistoryRuns)
		if err != nil {
			log.Printf("web: reading run history for %s: %v", name, err)
			return nil
		}
		last, ok, err := s.store.LastRunAt(name)
		if err != nil {
			log.Printf("web: reading last run for %s: %v", name, err)
			return nil
		}
		when := "sem coletas"
		if ok {
			when = humanSince(time.Since(last))
		}
		lights = append(lights, sourceLight{
			Name:   name,
			Status: crawl.HealthStatus(counts),
			When:   when,
		})
	}
	return lights
}

func (s *server) render(w http.ResponseWriter, t *template.Template, data map[string]any) {
	data["Lights"] = s.lights()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("web: rendering page: %v", err)
	}
}

func fail(w http.ResponseWriter, status int, err error) {
	log.Printf("web: %v", err)
	message := "Erro interno"
	switch status {
	case http.StatusNotFound:
		message = "Anúncio não encontrado"
	case http.StatusBadRequest:
		message = "Valor inválido"
	}
	http.Error(w, message, status)
}

func priceDrop(r store.Row) string {
	if r.PriceCents == nil || r.FirstPriceCents == nil {
		return ""
	}
	diff := *r.FirstPriceCents - *r.PriceCents
	if diff <= 0 {
		return ""
	}
	return fmt.Sprintf("▼ R$ %s desde %s", format.Thousands(diff/100),
		r.FirstSeenAt.In(time.Local).Format("02/01"))
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

func stateLabel(state string) string {
	switch state {
	case "contacted":
		return "contatado"
	case "dismissed":
		return "descartado"
	}
	return ""
}

func scannedLabel(count int) string {
	switch count {
	case 0:
		return "nenhum anúncio avaliado"
	case 1:
		return "1 anúncio avaliado"
	}
	return fmt.Sprintf("%d anúncios avaliados", count)
}

func datetime(t time.Time) string {
	return t.In(time.Local).Format("02/01/2006 15:04")
}

func humanSince(elapsed time.Duration) string {
	switch {
	case elapsed < time.Minute:
		return "agora"
	case elapsed < time.Hour:
		return fmt.Sprintf("há %dmin", int(elapsed.Minutes()))
	case elapsed < 48*time.Hour:
		return fmt.Sprintf("há %dh", int(elapsed.Hours()))
	}
	return fmt.Sprintf("há %dd", int(elapsed.Hours())/24)
}
