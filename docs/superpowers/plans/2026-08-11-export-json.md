# Export JSON Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Servir em `GET /export.json` um JSON enriquecido dos anúncios do banco,
para que um agente analise match e talvez sem ter que cruzar tabelas.

**Architecture:** Arquivo novo `internal/web/export.go` no pacote que já existe.
O handler faz as leituras no store e delega a uma função pura `buildExport`, que
recebe tudo pronto e devolve o envelope — testável sem HTTP e sem banco. Duas
leituras novas no store evitam uma query por anúncio.

**Tech Stack:** Go 1.x, `net/http` com `http.ServeMux` de rotas por método,
`encoding/json`, `modernc.org/sqlite`, testes com `testing` + `net/http/httptest`.

## Global Constraints

- **Sem comentários no código.** Nomes carregam a intenção.
- **TDD red-first sempre.** Escrever o teste, rodar para ver falhar pelo motivo
  esperado, e só então implementar.
- **Dinheiro em centavos `int64`.** Nunca float para valor monetário.
  `gap_percent` é a única exceção e não é dinheiro.
- **Ausente = ponteiro nil.** Nil nunca vira zero em silêncio, e no JSON sai
  como `null`, nunca como `0` ou `""`.
- **Mensagens de commit em inglês**, descrevendo comportamento e não diff.
- **Datas do JSON em UTC**, RFC 3339.
- **Slices vazios saem como `[]`**, nunca `null`.
- Antes de cada commit: `go build ./... && go vet ./... && go test ./...`.
- O repo não tem remote. Não existe PR. Trabalho em branch local `fase-4`,
  criada de `main`, merge `--no-ff` no fim.

## File Structure

| Arquivo | Responsabilidade |
|---|---|
| `internal/store/store.go` (modificar) | Ganha `CountByVerdict` e `PriceHistoryFor`, ao lado do `CountByState` que já existe |
| `internal/store/store_test.go` (modificar) | Testes das duas leituras novas |
| `internal/web/export.go` (criar) | Structs de saída, `buildExport` puro, handler `exportJSON`, parser do filtro |
| `internal/web/export_test.go` (criar) | Testes do endpoint, do envelope e de cada bloco enriquecido |
| `internal/web/server.go` (modificar) | Uma linha de rota no mux |
| `Makefile` (modificar) | Alvo `export` |
| `README.md` (modificar) | A rota na descrição do dashboard |

`server.go` já tem 516 linhas; o export vive em arquivo próprio para não engordá-lo.

---

### Task 0: Branch de trabalho

**Files:** nenhum

- [ ] **Step 1: Criar a branch a partir de `main`**

```bash
git switch -c fase-4
```

- [ ] **Step 2: Confirmar que a árvore está limpa**

Run: `git status --short`
Expected: nenhuma saída.

---

### Task 1: Leituras novas no store

**Files:**
- Modify: `internal/store/store.go` (acrescentar após `CountByState`, linha 302)
- Test: `internal/store/store_test.go` (acrescentar ao fim)

**Interfaces:**
- Consumes: `openTemp(t)` e `sample(cents)`, helpers que já existem em
  `internal/store/store_test.go`. `sample` devolve um `model.Listing` com
  `ExternalID: "abc123"`, `Verdict: model.VerdictMatch`, `Km: 31000`,
  `Fingerprint: "deadbeef"`.
- Produces:
  - `func (s *Store) CountByVerdict() (map[string]int, error)`
  - `func (s *Store) PriceHistoryFor(ids []int64) (map[int64][]PricePoint, error)`
  - `PricePoint` já existe: `{PriceCents int64; ObservedAt time.Time}`.

- [ ] **Step 1: Escrever o teste falhando do `CountByVerdict`**

Acrescentar ao fim de `internal/store/store_test.go`:

```go
func TestCountByVerdictCountsTheWholeTable(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	for _, spec := range []struct {
		id      string
		verdict model.Verdict
	}{
		{"c1", model.VerdictMatch},
		{"c2", model.VerdictMaybe},
		{"c3", model.VerdictMaybe},
		{"c4", model.VerdictReject},
	} {
		listing := sample(7200000)
		listing.ExternalID = spec.id
		listing.Verdict = spec.verdict
		if _, err := s.Upsert(listing, now); err != nil {
			t.Fatalf("Upsert %s: %v", spec.id, err)
		}
	}

	counts, err := s.CountByVerdict()
	if err != nil {
		t.Fatalf("CountByVerdict: %v", err)
	}
	for verdict, want := range map[string]int{"match": 1, "maybe": 2, "reject": 1} {
		if counts[verdict] != want {
			t.Errorf("%s: expected %d, got %d", verdict, want, counts[verdict])
		}
	}
	if len(counts) != 3 {
		t.Errorf("expected 3 verdicts, got %d: %v", len(counts), counts)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/store/ -run TestCountByVerdictCountsTheWholeTable`
Expected: FAIL na compilação, `s.CountByVerdict undefined (type *Store has no field or method CountByVerdict)`.

- [ ] **Step 3: Implementar o `CountByVerdict`**

Acrescentar em `internal/store/store.go`, logo após `CountByState`:

```go
func (s *Store) CountByVerdict() (map[string]int, error) {
	rows, err := s.db.Query("SELECT verdict, COUNT(*) FROM listings GROUP BY verdict")
	if err != nil {
		return nil, fmt.Errorf("counting listings by verdict: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var verdict string
		var count int
		if err := rows.Scan(&verdict, &count); err != nil {
			return nil, fmt.Errorf("scanning verdict count: %w", err)
		}
		counts[verdict] = count
	}
	return counts, rows.Err()
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/store/ -run TestCountByVerdictCountsTheWholeTable`
Expected: PASS.

- [ ] **Step 5: Escrever os testes falhando do `PriceHistoryFor`**

Acrescentar ao fim de `internal/store/store_test.go`:

```go
func TestPriceHistoryForGroupsByListingInObservationOrder(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	tracked := sample(7500000)
	tracked.ExternalID = "h1"
	first, err := s.Upsert(tracked, now)
	if err != nil {
		t.Fatalf("Upsert h1: %v", err)
	}
	dropped := int64(7100000)
	tracked.PriceCents = &dropped
	if _, err := s.Upsert(tracked, now.Add(48*time.Hour)); err != nil {
		t.Fatalf("Upsert h1 again: %v", err)
	}

	other := sample(6900000)
	other.ExternalID = "h2"
	second, err := s.Upsert(other, now)
	if err != nil {
		t.Fatalf("Upsert h2: %v", err)
	}

	history, err := s.PriceHistoryFor([]int64{first.ID, second.ID})
	if err != nil {
		t.Fatalf("PriceHistoryFor: %v", err)
	}

	points := history[first.ID]
	if len(points) != 2 {
		t.Fatalf("expected 2 points for the tracked listing, got %d", len(points))
	}
	if points[0].PriceCents != 7500000 || points[1].PriceCents != 7100000 {
		t.Errorf("expected 7500000 then 7100000, got %d then %d",
			points[0].PriceCents, points[1].PriceCents)
	}
	if len(history[second.ID]) != 1 {
		t.Errorf("expected 1 point for the other listing, got %d", len(history[second.ID]))
	}
}

func TestPriceHistoryForIgnoresListingsNotAsked(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	wanted := sample(7500000)
	wanted.ExternalID = "h1"
	asked, err := s.Upsert(wanted, now)
	if err != nil {
		t.Fatalf("Upsert h1: %v", err)
	}
	skipped := sample(6900000)
	skipped.ExternalID = "h2"
	if _, err := s.Upsert(skipped, now); err != nil {
		t.Fatalf("Upsert h2: %v", err)
	}

	history, err := s.PriceHistoryFor([]int64{asked.ID})
	if err != nil {
		t.Fatalf("PriceHistoryFor: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected history for 1 listing, got %d: %v", len(history), history)
	}
}

func TestPriceHistoryForWithoutIDsReturnsEmptyMap(t *testing.T) {
	s := openTemp(t)

	history, err := s.PriceHistoryFor(nil)
	if err != nil {
		t.Fatalf("PriceHistoryFor: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected an empty map, got %v", history)
	}
}
```

- [ ] **Step 6: Rodar e ver falhar**

Run: `go test ./internal/store/ -run TestPriceHistoryFor`
Expected: FAIL na compilação, `s.PriceHistoryFor undefined`.

- [ ] **Step 7: Implementar o `PriceHistoryFor`**

Acrescentar em `internal/store/store.go`, após o `CountByVerdict`, e incluir
`"strings"` no bloco de imports:

```go
func (s *Store) PriceHistoryFor(ids []int64) (map[int64][]PricePoint, error) {
	history := make(map[int64][]PricePoint, len(ids))
	if len(ids) == 0 {
		return history, nil
	}

	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")

	rows, err := s.db.Query(
		`SELECT listing_id, price_cents, observed_at FROM price_history
         WHERE listing_id IN (`+placeholders+`)
         ORDER BY listing_id, observed_at ASC, id ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("querying price history: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var p PricePoint
		if err := rows.Scan(&id, &p.PriceCents, &p.ObservedAt); err != nil {
			return nil, fmt.Errorf("scanning price point: %w", err)
		}
		history[id] = append(history[id], p)
	}
	return history, rows.Err()
}
```

- [ ] **Step 8: Rodar a suíte inteira do store**

Run: `go test ./internal/store/`
Expected: PASS, sem regressão nos testes existentes.

- [ ] **Step 9: Commit**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: the store counts by verdict and reads price history in bulk"
```

---

### Task 2: A rota, o filtro de veredito e o envelope base

**Files:**
- Create: `internal/web/export.go`
- Create: `internal/web/export_test.go`
- Modify: `internal/web/server.go` (registrar a rota no mux, junto das outras)

**Interfaces:**
- Consumes: `store.ListByVerdict(v model.Verdict) ([]store.Row, error)`,
  `store.CountByVerdict()` (Task 1), `fail(w, status, err)` e os helpers de teste
  `get(t, srv, path) (int, string)`, `emptyStore(t)`, `upsert(t, s, listing, at) int64`,
  `useSaoPauloZone(t)` e `expireEverythingUnseen(t, s, base)` — todos já existem
  em `internal/web/` (os três últimos em `server_test.go` e `lifecycle_test.go`).
- Produces:
  - `func exportVerdicts(value string) ([]model.Verdict, error)`
  - `type exportInput struct { Rows []store.Row; Counts map[string]int; Now time.Time }`
  - `func buildExport(in exportInput) exportEnvelope`
  - `func (s *server) exportJSON(w http.ResponseWriter, r *http.Request)`
  - Structs `exportEnvelope` e `exportListing`, que as Tasks 3 a 6 ampliam.
  - Helpers de teste `exportStore(t)` e `exportListingOf(id, verdict, cents)`.

- [ ] **Step 1: Escrever os testes falhando**

Criar `internal/web/export_test.go`:

```go
package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func exportListingOf(id string, verdict model.Verdict, cents int64) model.Listing {
	year := 2015
	km := 31000
	return model.Listing{
		Source: model.SourceOLX, ExternalID: id, URL: "https://example.com/" + id,
		Title: "Harley Street Glide " + id, Bike: model.BikeStreetGlide,
		Variant: model.VariantBase, Year: &year, PriceCents: &cents, Km: &km,
		City: "curitiba", State: "PR", Verdict: verdict,
	}
}

func exportStore(t *testing.T) *store.Store {
	t.Helper()
	s := emptyStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	upsert(t, s, exportListingOf("e1", model.VerdictMatch, 7200000), at)
	upsert(t, s, exportListingOf("e2", model.VerdictMaybe, 8100000), at)
	upsert(t, s, exportListingOf("e3", model.VerdictReject, 9900000), at)
	return s
}

type decodedExport struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Counts      map[string]int `json:"counts"`
	Listings    []struct {
		ID         int64   `json:"id"`
		ExternalID string  `json:"external_id"`
		Verdict    string  `json:"verdict"`
		Status     string  `json:"status"`
		PriceCents *int64  `json:"price_cents"`
		Km         *int    `json:"km"`
		Phone      *string `json:"phone"`
	} `json:"listings"`
}

func exportOf(t *testing.T, srv http.Handler, path string) decodedExport {
	t.Helper()
	code, body := get(t, srv, path)
	if code != http.StatusOK {
		t.Fatalf("%s: status %d, body %s", path, code, body)
	}
	var decoded decodedExport
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("%s is not valid json: %v\n%s", path, err, body)
	}
	return decoded
}

func verdictsIn(decoded decodedExport) []string {
	found := make([]string, 0, len(decoded.Listings))
	for _, l := range decoded.Listings {
		found = append(found, l.Verdict)
	}
	return found
}

func TestExportDefaultsToMatchAndMaybe(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Listings) != 2 {
		t.Fatalf("expected match and maybe, got %v", verdictsIn(decoded))
	}
	for _, verdict := range verdictsIn(decoded) {
		if verdict == "reject" {
			t.Errorf("the default export should leave rejects out, got %v", verdictsIn(decoded))
		}
	}
}

func TestExportNarrowsToASingleVerdict(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	for _, wanted := range []string{"match", "maybe"} {
		decoded := exportOf(t, srv, "/export.json?verdict="+wanted)
		if len(decoded.Listings) != 1 || decoded.Listings[0].Verdict != wanted {
			t.Errorf("verdict=%s returned %v", wanted, verdictsIn(decoded))
		}
	}
}

func TestExportAllBringsTheRejects(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json?verdict=all")

	if len(decoded.Listings) != 3 {
		t.Fatalf("expected the three verdicts, got %v", verdictsIn(decoded))
	}
}

func TestExportRejectsAnUnknownVerdict(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	code, _ := get(t, srv, "/export.json?verdict=lixo")

	if code != http.StatusBadRequest {
		t.Errorf("expected 400 for an unknown verdict, got %d", code)
	}
}

func TestExportCountsTheWholeDatabaseNotTheSlice(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json?verdict=match")

	if len(decoded.Listings) != 1 {
		t.Fatalf("expected a single listing, got %d", len(decoded.Listings))
	}
	for verdict, want := range map[string]int{"match": 1, "maybe": 1, "reject": 1} {
		if decoded.Counts[verdict] != want {
			t.Errorf("counts[%s]: expected %d, got %d", verdict, want, decoded.Counts[verdict])
		}
	}
}

func TestExportKeepsTheClosedListing(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	upsert(t, s, exportListingOf("e4", model.VerdictMatch, 7200000), base)
	expireEverythingUnseen(t, s, base)
	srv := NewServer(s, []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Listings) != 1 {
		t.Fatalf("a closed listing still belongs in the export, got %d", len(decoded.Listings))
	}
	if decoded.Listings[0].Status != "gone" {
		t.Errorf("the closed listing should carry its status, got %q", decoded.Listings[0].Status)
	}
}

func TestExportServesJSONContentType(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	rec := recorderFor(t, srv, "/export.json")

	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("unexpected content type %q", got)
	}
}

func TestExportOfAnEmptyDatabaseIsAnEmptyList(t *testing.T) {
	srv := NewServer(emptyStore(t), []string{model.SourceOLX})

	code, body := get(t, srv, "/export.json")

	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if !strings.Contains(body, `"listings": []`) {
		t.Errorf("an empty database should export an empty array, got %s", body)
	}
	for _, verdict := range []string{"match", "maybe", "reject"} {
		if !strings.Contains(body, `"`+verdict+`": 0`) {
			t.Errorf("counts should carry %s at zero, got %s", verdict, body)
		}
	}
}

func TestExportKeepsAbsentValuesNull(t *testing.T) {
	s := emptyStore(t)
	bare := exportListingOf("e9", model.VerdictMatch, 0)
	bare.PriceCents = nil
	bare.Km = nil
	upsert(t, s, bare, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	srv := NewServer(s, []string{model.SourceOLX})

	_, body := get(t, srv, "/export.json")

	for _, field := range []string{"price_cents", "km", "phone", "published_at"} {
		if !strings.Contains(body, `"`+field+`": null`) {
			t.Errorf("%s should be null when absent, got %s", field, body)
		}
	}
}

func TestExportDatesAreUTCWhateverTheLocalZone(t *testing.T) {
	useSaoPauloZone(t)
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	_, body := get(t, srv, "/export.json")

	if !strings.Contains(body, `"first_seen_at": "2026-08-01T12:00:00Z"`) {
		t.Errorf("dates should be exported in UTC, got %s", body)
	}
}

func TestExportDoesNotEscapeURLs(t *testing.T) {
	s := emptyStore(t)
	tracked := exportListingOf("e8", model.VerdictMatch, 7200000)
	tracked.URL = "https://example.com/moto?ref=busca&pos=2"
	upsert(t, s, tracked, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	srv := NewServer(s, []string{model.SourceOLX})

	_, body := get(t, srv, "/export.json")

	if !strings.Contains(body, "https://example.com/moto?ref=busca&pos=2") {
		t.Errorf("the url should survive unescaped, got %s", body)
	}
}
```

Acrescentar o helper `recorderFor` ao fim do mesmo arquivo:

```go
func recorderFor(t *testing.T, srv http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}
```

E `"net/http/httptest"` ao bloco de imports.

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/web/ -run TestExport`
Expected: FAIL. Os testes de rota devolvem 404 (a rota não existe) e o
`TestExportServesJSONContentType` acusa content type vazio.

- [ ] **Step 3: Criar o `export.go` com o envelope base**

Criar `internal/web/export.go`:

```go
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
```

`SetEscapeHTML(false)` está lá porque as URLs de marketplace carregam `&` na
query string, e o padrão do `encoding/json` o transformaria em `&`.

- [ ] **Step 4: Registrar a rota**

Em `internal/web/server.go`, dentro do `NewServer`, logo antes de
`mux.HandleFunc("GET /health", srv.health)`:

```go
	mux.HandleFunc("GET /export.json", srv.exportJSON)
```

- [ ] **Step 5: Rodar e ver passar**

Run: `go test ./internal/web/ -run TestExport -v`
Expected: PASS em todos os `TestExport*`.

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS, sem regressão nas páginas HTML.

- [ ] **Step 7: Commit**

```bash
git add internal/web/export.go internal/web/export_test.go internal/web/server.go
git commit -m "feat: serve the listings as json at /export.json"
```

---

### Task 3: Histórico de preço e queda

**Files:**
- Modify: `internal/web/export.go`
- Modify: `internal/web/export_test.go`

**Interfaces:**
- Consumes: `store.PriceHistoryFor(ids) (map[int64][]store.PricePoint, error)` (Task 1),
  `buildExport(in exportInput) exportEnvelope` (Task 2).
- Produces:
  - `type exportPricePoint struct { PriceCents int64; At time.Time }` com tags
    `json:"price_cents"` e `json:"at"`.
  - `exportListing` ganha `PriceDropCents *int64 \`json:"price_drop_cents"\`` e
    `PriceHistory []exportPricePoint \`json:"price_history"\``.
  - `exportInput` passa a ser
    `{ Rows []store.Row; History map[int64][]store.PricePoint; Counts map[string]int; Now time.Time }`.
  - `func exportListingFrom(row store.Row, history []store.PricePoint) exportListing`.

- [ ] **Step 1: Escrever os testes falhando**

Acrescentar ao `decodedExport` os dois campos, dentro do struct anônimo de
`Listings`:

```go
		PriceDropCents *int64 `json:"price_drop_cents"`
		PriceHistory   []struct {
			PriceCents int64     `json:"price_cents"`
			At         time.Time `json:"at"`
		} `json:"price_history"`
```

E acrescentar ao fim de `internal/web/export_test.go`:

```go
func droppedStore(t *testing.T) *store.Store {
	t.Helper()
	s := emptyStore(t)
	at := time.Date(2026, 7, 2, 11, 4, 0, 0, time.UTC)

	tracked := exportListingOf("d1", model.VerdictMatch, 7500000)
	upsert(t, s, tracked, at)
	dropped := int64(7100000)
	tracked.PriceCents = &dropped
	upsert(t, s, tracked, at.Add(48*time.Hour))

	upsert(t, s, exportListingOf("d2", model.VerdictMatch, 6900000), at)
	return s
}

func listingByExternalID(t *testing.T, decoded decodedExport, id string) int {
	t.Helper()
	for i, l := range decoded.Listings {
		if l.ExternalID == id {
			return i
		}
	}
	t.Fatalf("listing %s not found in the export", id)
	return -1
}

func TestExportCarriesTheWholePriceHistory(t *testing.T) {
	srv := NewServer(droppedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")
	tracked := decoded.Listings[listingByExternalID(t, decoded, "d1")]

	if len(tracked.PriceHistory) != 2 {
		t.Fatalf("expected 2 price points, got %d", len(tracked.PriceHistory))
	}
	if tracked.PriceHistory[0].PriceCents != 7500000 {
		t.Errorf("first point should be the original price, got %d",
			tracked.PriceHistory[0].PriceCents)
	}
	if tracked.PriceHistory[1].PriceCents != 7100000 {
		t.Errorf("second point should be the drop, got %d",
			tracked.PriceHistory[1].PriceCents)
	}
	if !tracked.PriceHistory[0].At.Before(tracked.PriceHistory[1].At) {
		t.Error("price points should come in observation order")
	}
}

func TestExportReportsThePriceDrop(t *testing.T) {
	srv := NewServer(droppedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")
	tracked := decoded.Listings[listingByExternalID(t, decoded, "d1")]

	if tracked.PriceDropCents == nil {
		t.Fatal("a listing that dropped should report the drop")
	}
	if *tracked.PriceDropCents != 400000 {
		t.Errorf("expected a drop of 400000 cents, got %d", *tracked.PriceDropCents)
	}
}

func TestExportLeavesTheDropNullWhenThePriceHeld(t *testing.T) {
	srv := NewServer(droppedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")
	steady := decoded.Listings[listingByExternalID(t, decoded, "d2")]

	if steady.PriceDropCents != nil {
		t.Errorf("a steady price should not report a drop, got %d", *steady.PriceDropCents)
	}
}

func TestExportGivesAnEmptyHistoryToAPricelessListing(t *testing.T) {
	s := emptyStore(t)
	bare := exportListingOf("d3", model.VerdictMatch, 0)
	bare.PriceCents = nil
	upsert(t, s, bare, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	srv := NewServer(s, []string{model.SourceOLX})

	_, body := get(t, srv, "/export.json")

	if !strings.Contains(body, `"price_history": []`) {
		t.Errorf("a listing without history should export an empty array, got %s", body)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/web/ -run "TestExportCarriesTheWholePriceHistory|TestExportReportsThePriceDrop|TestExportLeaves|TestExportGivesAnEmpty" -v`
Expected: FAIL. `price_history` chega vazio e `price_drop_cents` chega nulo,
porque os campos ainda não são montados.

- [ ] **Step 3: Implementar**

Em `internal/web/export.go`, acrescentar o tipo e os dois campos:

```go
type exportPricePoint struct {
	PriceCents int64     `json:"price_cents"`
	At         time.Time `json:"at"`
}
```

No `exportListing`, após `LastSeenAt`:

```go
	PriceDropCents *int64             `json:"price_drop_cents"`
	PriceHistory   []exportPricePoint `json:"price_history"`
```

No `exportInput`, após `Rows`:

```go
	History map[int64][]store.PricePoint
```

No `buildExport`, trocar a montagem da lista:

```go
	listings := make([]exportListing, 0, len(in.Rows))
	for _, row := range in.Rows {
		listings = append(listings, exportListingFrom(row, in.History[row.ID]))
	}
```

Trocar o `exportListingFrom` inteiro por esta versão, que só acrescenta os dois
últimos campos ao literal:

```go
func exportListingFrom(row store.Row, history []store.PricePoint) exportListing {
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
	}
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
```

No handler `exportJSON`, entre a leitura de `counts` e a chamada de `buildExport`:

```go
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	history, err := s.store.PriceHistoryFor(ids)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
```

E passar `History: history` no `exportInput`.

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/web/ -run TestExport -v`
Expected: PASS em todos.

- [ ] **Step 5: Commit**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/web/export.go internal/web/export_test.go
git commit -m "feat: the export carries the price history and the drop since the first sighting"
```

---

### Task 4: Bloco FIPE

**Files:**
- Modify: `internal/web/export.go`
- Modify: `internal/web/export_test.go`

**Interfaces:**
- Consumes: `store.FipeReferences() ([]fipe.Reference, error)`,
  `fipe.NewTable(refs) *fipe.Table`, `table.Lookup(bike, variant, year) (*fipe.Reference, bool)`.
  `fipe.Reference` traz `{Code, Label, Bike, Variant string; Year int; PriceCents int64; Month string; Base bool}`,
  e o `Base` do retorno do `Lookup` vale `true` quando a variante pedida era
  `model.VariantUnknown` e o casamento caiu na variante base.
- Produces:
  - `type exportFipe struct { Label string; Year int; PriceCents int64; GapPercent *float64; BelowFipe *bool; BaseVariant bool }`
    com tags `json:"label"`, `json:"year"`, `json:"price_cents"`,
    `json:"gap_percent"`, `json:"below_fipe"`, `json:"base_variant"`.
  - `type exportFipeRef struct { Label, Bike, Variant string; Year int; PriceCents int64; Month string }`
    com tags `json:"label"`, `json:"bike"`, `json:"variant"`, `json:"year"`,
    `json:"price_cents"`, `json:"month"`.
  - `exportEnvelope` ganha `Fipe []exportFipeRef \`json:"fipe"\`` entre `Counts` e `Listings`.
  - `exportListing` ganha `Fipe *exportFipe \`json:"fipe"\``.
  - `exportInput` ganha `Refs *fipe.Table` e `FipeRows []fipe.Reference`.
  - `func exportListingFrom(row store.Row, history []store.PricePoint, refs *fipe.Table) exportListing`.

- [ ] **Step 1: Escrever os testes falhando**

Acrescentar ao `decodedExport` o campo de envelope, entre `Counts` e `Listings`:

```go
	Fipe []struct {
		Label      string `json:"label"`
		Year       int    `json:"year"`
		PriceCents int64  `json:"price_cents"`
		Month      string `json:"month"`
	} `json:"fipe"`
```

E dentro do struct de `Listings`:

```go
		Fipe *struct {
			Label       string   `json:"label"`
			Year        int      `json:"year"`
			PriceCents  int64    `json:"price_cents"`
			GapPercent  *float64 `json:"gap_percent"`
			BelowFipe   *bool    `json:"below_fipe"`
			BaseVariant bool     `json:"base_variant"`
		} `json:"fipe"`
```

Acrescentar ao fim de `internal/web/export_test.go`, com `"github.com/andreabreu76/harley-hunter/internal/fipe"` no bloco de imports:

```go
func fipedStore(t *testing.T) *store.Store {
	t.Helper()
	s := emptyStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	reference := fipe.Reference{
		Code: "810055-1", Label: "FLHXS STREET GLIDE SPECIAL",
		Bike: model.BikeStreetGlide, Variant: model.VariantBase,
		Year: 2015, PriceCents: 8000000, Month: "agosto de 2026",
	}
	if err := s.SaveFipeReference(reference, at); err != nil {
		t.Fatalf("SaveFipeReference: %v", err)
	}

	upsert(t, s, exportListingOf("f1", model.VerdictMatch, 7200000), at)
	upsert(t, s, exportListingOf("f2", model.VerdictMatch, 8800000), at)

	unmatched := exportListingOf("f3", model.VerdictMatch, 7200000)
	unmatched.Bike = model.BikeRoadGlide
	upsert(t, s, unmatched, at)

	yearless := exportListingOf("f4", model.VerdictMatch, 7200000)
	yearless.Year = nil
	upsert(t, s, yearless, at)

	priceless := exportListingOf("f5", model.VerdictMatch, 0)
	priceless.PriceCents = nil
	upsert(t, s, priceless, at)

	unknownVariant := exportListingOf("f6", model.VerdictMatch, 7200000)
	unknownVariant.Variant = model.VariantUnknown
	upsert(t, s, unknownVariant, at)

	return s
}

func TestExportListsTheFipeTableInTheEnvelope(t *testing.T) {
	srv := NewServer(fipedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Fipe) != 1 {
		t.Fatalf("expected the single stored reference, got %d", len(decoded.Fipe))
	}
	if decoded.Fipe[0].PriceCents != 8000000 || decoded.Fipe[0].Month != "agosto de 2026" {
		t.Errorf("unexpected reference %+v", decoded.Fipe[0])
	}
}

func TestExportMeasuresTheGapBelowFipe(t *testing.T) {
	srv := NewServer(fipedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")
	cheap := decoded.Listings[listingByExternalID(t, decoded, "f1")]

	if cheap.Fipe == nil {
		t.Fatal("a listing that matches the table should carry its reference")
	}
	if cheap.Fipe.GapPercent == nil || *cheap.Fipe.GapPercent != 10 {
		t.Errorf("7200000 against 8000000 is a 10%% gap, got %v", cheap.Fipe.GapPercent)
	}
	if cheap.Fipe.BelowFipe == nil || !*cheap.Fipe.BelowFipe {
		t.Error("7200000 is below a fipe of 8000000")
	}
	if cheap.Fipe.BaseVariant {
		t.Error("a listing with a known variant is not a base match")
	}
}

func TestExportMeasuresTheGapAboveFipe(t *testing.T) {
	srv := NewServer(fipedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")
	pricey := decoded.Listings[listingByExternalID(t, decoded, "f2")]

	if pricey.Fipe == nil || pricey.Fipe.GapPercent == nil {
		t.Fatal("a listing above fipe still carries its gap")
	}
	if *pricey.Fipe.GapPercent != 10 {
		t.Errorf("8800000 against 8000000 is a 10%% gap, got %v", *pricey.Fipe.GapPercent)
	}
	if pricey.Fipe.BelowFipe == nil || *pricey.Fipe.BelowFipe {
		t.Error("8800000 is above a fipe of 8000000")
	}
}

func TestExportLeavesFipeNullWithoutAMatch(t *testing.T) {
	srv := NewServer(fipedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")

	for _, id := range []string{"f3", "f4"} {
		listing := decoded.Listings[listingByExternalID(t, decoded, id)]
		if listing.Fipe != nil {
			t.Errorf("%s has no fipe match and should export null, got %+v", id, listing.Fipe)
		}
	}
}

func TestExportKeepsTheReferenceWithoutAnAskingPrice(t *testing.T) {
	srv := NewServer(fipedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")
	priceless := decoded.Listings[listingByExternalID(t, decoded, "f5")]

	if priceless.Fipe == nil {
		t.Fatal("a listing without a price still matches the table")
	}
	if priceless.Fipe.PriceCents != 8000000 {
		t.Errorf("the reference price should survive, got %d", priceless.Fipe.PriceCents)
	}
	if priceless.Fipe.GapPercent != nil || priceless.Fipe.BelowFipe != nil {
		t.Error("without an asking price there is no gap to report")
	}
}

func TestExportFlagsAMatchThroughTheBaseVariant(t *testing.T) {
	srv := NewServer(fipedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")
	unknown := decoded.Listings[listingByExternalID(t, decoded, "f6")]

	if unknown.Fipe == nil {
		t.Fatal("an unknown variant falls back to the base reference")
	}
	if !unknown.Fipe.BaseVariant {
		t.Error("a fallback match should be flagged as a base match")
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/web/ -run "TestExportListsTheFipeTable|TestExportMeasures|TestExportLeavesFipeNull|TestExportKeepsTheReference|TestExportFlagsAMatch" -v`
Expected: FAIL. O envelope não tem `fipe` e cada anúncio traz `fipe: null`.

- [ ] **Step 3: Implementar**

Em `internal/web/export.go`, acrescentar `"math"` e
`"github.com/andreabreu76/harley-hunter/internal/fipe"` aos imports, e os tipos:

```go
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
```

No `exportEnvelope`, entre `Counts` e `Listings`:

```go
	Fipe []exportFipeRef `json:"fipe"`
```

No `exportListing`, após `LastSeenAt`:

```go
	Fipe *exportFipe `json:"fipe"`
```

No `exportInput`, após `History`:

```go
	Refs     *fipe.Table
	FipeRows []fipe.Reference
```

No `buildExport`, montar a tabela do envelope e repassar as referências:

```go
	references := make([]exportFipeRef, 0, len(in.FipeRows))
	for _, r := range in.FipeRows {
		references = append(references, exportFipeRef{
			Label: r.Label, Bike: r.Bike, Variant: r.Variant,
			Year: r.Year, PriceCents: r.PriceCents, Month: r.Month,
		})
	}

	listings := make([]exportListing, 0, len(in.Rows))
	for _, row := range in.Rows {
		listings = append(listings, exportListingFrom(row, in.History[row.ID], in.Refs))
	}

	return exportEnvelope{
		GeneratedAt: in.Now.UTC(), Counts: counts,
		Fipe: references, Listings: listings,
	}
```

Trocar a assinatura do `exportListingFrom` para
`func exportListingFrom(row store.Row, history []store.PricePoint, refs *fipe.Table) exportListing`
e acrescentar `Fipe: exportFipeOf(row, refs)` ao literal. Acrescentar:

```go
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
```

No handler `exportJSON`, antes do `buildExport`:

```go
	references, err := s.store.FipeReferences()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
```

E passar `Refs: fipe.NewTable(references), FipeRows: references` no `exportInput`.

A leitura da FIPE derruba a resposta com 500 quando falha, ao contrário do
`fipeTable()` do dashboard, que loga e segue com a tabela vazia. Um JSON que
some com a referência silenciosamente engana o consumidor.

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/web/ -run TestExport -v`
Expected: PASS em todos.

- [ ] **Step 5: Commit**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/web/export.go internal/web/export_test.go
git commit -m "feat: the export prices each listing against its fipe reference"
```

---

### Task 5: Reanúncios

**Files:**
- Modify: `internal/web/export.go`
- Modify: `internal/web/export_test.go`

**Interfaces:**
- Consumes: `store.RepostGroups() (map[string][]store.Row, error)` e
  `siblingsOf(row store.Row, groups map[string][]store.Row) []store.Row`, que já
  existe em `internal/web/server.go:231` e devolve `nil` quando o anúncio não tem
  fingerprint ou não tem km.
- Produces:
  - `type exportRepost struct { ID int64; Source, Verdict string; PriceCents *int64; Km *int; FirstSeenAt time.Time }`
    com tags `json:"id"`, `json:"source"`, `json:"verdict"`, `json:"price_cents"`,
    `json:"km"`, `json:"first_seen_at"`.
  - `exportListing` ganha `Reposts []exportRepost \`json:"reposts"\``.
  - `exportInput` ganha `Groups map[string][]store.Row`.
  - `func exportListingFrom(row store.Row, history []store.PricePoint, refs *fipe.Table, siblings []store.Row) exportListing`.

- [ ] **Step 1: Escrever os testes falhando**

Acrescentar ao struct de `Listings` do `decodedExport`:

```go
		Reposts []struct {
			ID          int64     `json:"id"`
			Source      string    `json:"source"`
			Verdict     string    `json:"verdict"`
			PriceCents  *int64    `json:"price_cents"`
			Km          *int      `json:"km"`
			FirstSeenAt time.Time `json:"first_seen_at"`
		} `json:"reposts"`
```

E acrescentar ao fim de `internal/web/export_test.go`:

```go
func repostedStore(t *testing.T) *store.Store {
	t.Helper()
	s := emptyStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	twin := exportListingOf("r1", model.VerdictMatch, 7200000)
	twin.Fingerprint = "street_glide|2015|6|curitiba"
	upsert(t, s, twin, at)

	elsewhere := exportListingOf("r2", model.VerdictReject, 7900000)
	elsewhere.Source = model.SourceMercadoLivre
	elsewhere.Fingerprint = twin.Fingerprint
	upsert(t, s, elsewhere, at.Add(-72*time.Hour))

	upsert(t, s, exportListingOf("r3", model.VerdictMatch, 6800000), at)
	return s
}

func TestExportCarriesTheRepostSiblingWithItsOwnFields(t *testing.T) {
	srv := NewServer(repostedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json?verdict=match")
	twin := decoded.Listings[listingByExternalID(t, decoded, "r1")]

	if len(twin.Reposts) != 1 {
		t.Fatalf("expected a single sibling, got %d", len(twin.Reposts))
	}
	sibling := twin.Reposts[0]
	if sibling.Source != model.SourceMercadoLivre {
		t.Errorf("the sibling source should travel with it, got %q", sibling.Source)
	}
	if sibling.Verdict != "reject" {
		t.Errorf("the sibling verdict should travel with it, got %q", sibling.Verdict)
	}
	if sibling.PriceCents == nil || *sibling.PriceCents != 7900000 {
		t.Errorf("the sibling price should travel with it, got %v", sibling.PriceCents)
	}
	if sibling.Km == nil || *sibling.Km != 31000 {
		t.Errorf("the sibling km should travel with it, got %v", sibling.Km)
	}
	if sibling.FirstSeenAt.IsZero() {
		t.Error("the sibling should carry when it was first seen")
	}
}

func TestExportCarriesASiblingLeftOutOfTheSlice(t *testing.T) {
	srv := NewServer(repostedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json?verdict=match")

	for _, listing := range decoded.Listings {
		if listing.Verdict == "reject" {
			t.Fatal("verdict=match should not export the reject as a listing")
		}
	}
	twin := decoded.Listings[listingByExternalID(t, decoded, "r1")]
	if len(twin.Reposts) != 1 {
		t.Errorf("a sibling outside the slice still belongs in reposts, got %d", len(twin.Reposts))
	}
}

func TestExportGivesAnEmptyRepostListToALoneListing(t *testing.T) {
	srv := NewServer(repostedStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json?verdict=match")
	alone := decoded.Listings[listingByExternalID(t, decoded, "r3")]

	if len(alone.Reposts) != 0 {
		t.Errorf("a listing without a twin has no reposts, got %d", len(alone.Reposts))
	}

	_, body := get(t, srv, "/export.json?verdict=match")
	if !strings.Contains(body, `"reposts": []`) {
		t.Errorf("an empty repost list should be an array, got %s", body)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/web/ -run "TestExportCarriesTheRepost|TestExportCarriesASibling|TestExportGivesAnEmptyRepost" -v`
Expected: FAIL. `reposts` chega vazio nos três.

- [ ] **Step 3: Implementar**

Em `internal/web/export.go`, acrescentar o tipo:

```go
type exportRepost struct {
	ID          int64     `json:"id"`
	Source      string    `json:"source"`
	Verdict     string    `json:"verdict"`
	PriceCents  *int64    `json:"price_cents"`
	Km          *int      `json:"km"`
	FirstSeenAt time.Time `json:"first_seen_at"`
}
```

No `exportListing`, após `Fipe`:

```go
	Reposts []exportRepost `json:"reposts"`
```

No `exportInput`, após `Refs`/`FipeRows`:

```go
	Groups map[string][]store.Row
```

No `buildExport`, repassar os irmãos:

```go
		listings = append(listings, exportListingFrom(
			row, in.History[row.ID], in.Refs, siblingsOf(row, in.Groups)))
```

Trocar a assinatura do `exportListingFrom` para
`func exportListingFrom(row store.Row, history []store.PricePoint, refs *fipe.Table, siblings []store.Row) exportListing`
e acrescentar `Reposts: exportReposts(siblings)` ao literal. Acrescentar:

```go
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
```

No handler `exportJSON`, antes do `buildExport`:

```go
	groups, err := s.store.RepostGroups()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
```

E passar `Groups: groups` no `exportInput`.

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/web/ -run TestExport -v`
Expected: PASS em todos.

- [ ] **Step 5: Commit**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/web/export.go internal/web/export_test.go
git commit -m "feat: the export names the repost siblings of each listing"
```

---

### Task 6: Saúde das fontes no envelope

**Files:**
- Modify: `internal/web/export.go`
- Modify: `internal/web/export_test.go`

**Interfaces:**
- Consumes: `store.RecentRunCounts(source string, limit int) ([]int, error)`,
  `store.LastRunAt(source string) (time.Time, bool, error)`,
  `store.RecordRun(source string, started, finished time.Time, itemCount int, status, errMessage string) error`,
  `crawl.HealthStatus(counts []int) string`, e a constante `healthHistoryRuns = 30`
  de `internal/web/server.go:24`.
- Produces:
  - `type exportSource struct { Name, Status string; LastRunAt *time.Time; RecentCounts []int }`
    com tags `json:"name"`, `json:"status"`, `json:"last_run_at"`, `json:"recent_counts"`.
  - `exportEnvelope` ganha `Sources []exportSource \`json:"sources"\`` entre `Counts` e `Fipe`.
  - `exportInput` ganha `Sources []exportSource`.
  - `func (s *server) exportSources() ([]exportSource, error)`.

- [ ] **Step 1: Escrever os testes falhando**

Acrescentar ao `decodedExport`, entre `Counts` e `Fipe`:

```go
	Sources []struct {
		Name         string     `json:"name"`
		Status       string     `json:"status"`
		LastRunAt    *time.Time `json:"last_run_at"`
		RecentCounts []int      `json:"recent_counts"`
	} `json:"sources"`
```

E acrescentar ao fim de `internal/web/export_test.go`, com
`"github.com/andreabreu76/harley-hunter/internal/crawl"` nos imports:

```go
func sourceIn(t *testing.T, decoded decodedExport, name string) int {
	t.Helper()
	for i, s := range decoded.Sources {
		if s.Name == name {
			return i
		}
	}
	t.Fatalf("source %s not found in the export", name)
	return -1
}

func TestExportReportsTheHealthOfEachSource(t *testing.T) {
	s := exportStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	for i, count := range []int{9, 11, 10} {
		started := at.Add(time.Duration(i) * time.Hour)
		if err := s.RecordRun(model.SourceOLX, started, started.Add(time.Minute), count, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	srv := NewServer(s, []string{model.SourceOLX, model.SourceWebmotors})

	decoded := exportOf(t, srv, "/export.json")

	if len(decoded.Sources) != 2 {
		t.Fatalf("expected both configured sources, got %d", len(decoded.Sources))
	}
	olx := decoded.Sources[sourceIn(t, decoded, model.SourceOLX)]
	if olx.Status != crawl.HealthOK {
		t.Errorf("a source with productive runs should be ok, got %q", olx.Status)
	}
	if len(olx.RecentCounts) != 3 || olx.RecentCounts[0] != 10 {
		t.Errorf("recent counts should come newest first, got %v", olx.RecentCounts)
	}
	if olx.LastRunAt == nil || !olx.LastRunAt.Equal(at.Add(2*time.Hour).Add(time.Minute)) {
		t.Errorf("unexpected last run %v", olx.LastRunAt)
	}
}

func TestExportLeavesASilentSourceWithoutARun(t *testing.T) {
	srv := NewServer(exportStore(t), []string{model.SourceOLX})

	decoded := exportOf(t, srv, "/export.json")
	olx := decoded.Sources[sourceIn(t, decoded, model.SourceOLX)]

	if olx.LastRunAt != nil {
		t.Errorf("a source that never ran has no last run, got %v", olx.LastRunAt)
	}

	_, body := get(t, srv, "/export.json")
	if !strings.Contains(body, `"recent_counts": []`) {
		t.Errorf("a source that never ran should carry an empty array, got %s", body)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/web/ -run "TestExportReportsTheHealth|TestExportLeavesASilent" -v`
Expected: FAIL. `sources` chega vazio nos dois.

- [ ] **Step 3: Implementar**

Em `internal/web/export.go`, acrescentar
`"github.com/andreabreu76/harley-hunter/internal/crawl"` aos imports e o tipo:

```go
type exportSource struct {
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	LastRunAt    *time.Time `json:"last_run_at"`
	RecentCounts []int      `json:"recent_counts"`
}
```

No `exportEnvelope`, entre `Counts` e `Fipe`:

```go
	Sources []exportSource `json:"sources"`
```

No `exportInput`, acrescentar `Sources []exportSource`, e no retorno do
`buildExport` acrescentar `Sources: in.Sources`.

Acrescentar o leitor:

```go
func (s *server) exportSources() ([]exportSource, error) {
	sources := make([]exportSource, 0, len(s.sources))
	for _, name := range s.sources {
		counts, err := s.store.RecentRunCounts(name, healthHistoryRuns)
		if err != nil {
			return nil, err
		}
		if counts == nil {
			counts = []int{}
		}
		last, ok, err := s.store.LastRunAt(name)
		if err != nil {
			return nil, err
		}
		source := exportSource{
			Name: name, Status: crawl.HealthStatus(counts), RecentCounts: counts,
		}
		if ok {
			utc := last.UTC()
			source.LastRunAt = &utc
		}
		sources = append(sources, source)
	}
	return sources, nil
}
```

No handler `exportJSON`, antes do `buildExport`:

```go
	sources, err := s.exportSources()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
```

E passar `Sources: sources` no `exportInput`.

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/web/ -run TestExport -v`
Expected: PASS em todos.

- [ ] **Step 5: Commit**

```bash
go build ./... && go vet ./... && go test ./...
git add internal/web/export.go internal/web/export_test.go
git commit -m "feat: the export tells whether each source is still collecting"
```

---

### Task 7: Atalho no Makefile e a rota no README

**Files:**
- Modify: `Makefile`
- Modify: `README.md:43`

**Interfaces:**
- Consumes: a rota `GET /export.json` (Task 2) e o alvo `serve` que já existe.
- Produces: alvo `make export`, que grava `export.json` na raiz do repo.

- [ ] **Step 1: Acrescentar o alvo ao `Makefile`**

Na linha `.PHONY`, incluir `export` após `serve`. Depois do alvo `serve`
(linha 20), acrescentar:

```makefile
export: ## baixa o json do dashboard em export.json (exige o serve rodando)
	curl -sf "http://127.0.0.1:8080/export.json$(if $(VERDICT),?verdict=$(VERDICT))" -o export.json \
		&& echo "export.json gravado" || echo "o dashboard não respondeu — rode make serve antes"
```

- [ ] **Step 2: Ignorar o arquivo gerado**

Acrescentar `export.json` ao `.gitignore`.

- [ ] **Step 3: Conferir o alvo**

Run: `make help`
Expected: a linha `export` aparece na lista, entre `crawl` e `fmt`.

- [ ] **Step 4: Documentar a rota no README**

Em `README.md`, substituir a linha 43 e a seguinte por:

```markdown
Sobe em <http://127.0.0.1:8080> com as abas de match, maybe, rejeitados e saúde
das fontes. Encerra com Ctrl+C.

Com o dashboard no ar, `GET /export.json` devolve o banco em JSON, para entregar
a um agente. Traz match e maybe por padrão; `?verdict=match`, `?verdict=maybe` e
`?verdict=all` estreitam ou ampliam o recorte. Cada anúncio vai com a referência
FIPE e o gap em percentual, a queda desde o primeiro preço visto, o histórico
completo e os reanúncios irmãos. O envelope leva ainda a contagem de todo o
banco e a saúde de cada fonte. `make export` grava o arquivo, e `make export
VERDICT=all` traz os descartados junto.
```

- [ ] **Step 5: Verificação de ponta a ponta contra uma cópia do banco**

Nunca apontar essa verificação para o `hunter.db` da raiz do repo: o launchd
pode disparar uma coleta no meio. Copiar também o `-wal` e o `-shm`, porque o
banco roda em WAL e sem eles a cópia vem velha.

```bash
mkdir -p /tmp/harley-export-check
cp hunter.db hunter.db-wal hunter.db-shm /tmp/harley-export-check/
sed 's|^database_path:.*|database_path: /tmp/harley-export-check/hunter.db|' \
	config/config.yaml > /tmp/harley-export-check/config.yaml
grep database_path /tmp/harley-export-check/config.yaml
```

Expected: `database_path: /tmp/harley-export-check/hunter.db`. Se o `grep` não
mostrar isso, parar e corrigir antes de subir o servidor.

Conferir que a porta está livre, senão o servidor sobe contra o banco errado:

```bash
lsof -ti tcp:8080 || echo "porta livre"
```

Expected: `porta livre`. Se algo responder, encerrar aquele processo antes de
seguir.

```bash
go build -o /tmp/harley-export-check/hunter ./cmd/hunter
/tmp/harley-export-check/hunter -config /tmp/harley-export-check/config.yaml serve &
sleep 1
curl -s "http://127.0.0.1:8080/export.json" | head -40
curl -s "http://127.0.0.1:8080/export.json" | grep -c '"external_id":'
curl -s -o /dev/null -w '%{http_code}\n' "http://127.0.0.1:8080/export.json?verdict=lixo"
curl -s "http://127.0.0.1:8080/export.json?verdict=all" | grep -c '"external_id":'
kill %1
```

Expected: JSON válido abrindo por `generated_at`, `counts`, `sources`, `fipe` e
`listings`; a primeira contagem bate com match + maybe do banco (77 hoje), a
segunda é maior que ela (657 hoje), e o filtro inválido devolve `400`.

`"external_id"` é o que se conta, e não `"id"`, porque os irmãos de reanúncio
também carregam um `id` e inflariam a contagem.

- [ ] **Step 6: Limpar a cópia**

```bash
rm -rf /tmp/harley-export-check
```

- [ ] **Step 7: Commit**

```bash
go build ./... && go vet ./... && go test ./...
git add Makefile README.md .gitignore
git commit -m "docs: the dashboard exports the database as json"
```

---

### Task 8: Fechar a fase

**Files:** nenhum

- [ ] **Step 1: Conferir que nada de scratch entrou**

Run: `git status --short`
Expected: nenhuma saída. Nenhum `zz_*`, `*.bak`, `.env`, `*.db` ou `export.json`.

- [ ] **Step 2: Rodar a verificação completa**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS em todos os pacotes.

- [ ] **Step 3: Merge em `main`**

```bash
git switch main
git merge --no-ff fase-4 -m "merge: fase 4 do harley-hunter"
```

- [ ] **Step 4: Atualizar o binário de produção**

Editar o repo não muda produção.

```bash
make build
```
