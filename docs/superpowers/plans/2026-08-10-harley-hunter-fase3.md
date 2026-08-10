# Harley Hunter Fase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fechar os dois furos que fazem o sistema perder um alerta: a dedup que
silencia motos distintas e as quedas de preço que não sobrevivem à rodada.

**Architecture:** A dedup de alerta passa a exigir fonte diferente para
silenciar (mesma fonte com ambos ativos = duas motos). As quedas deixam de
viver em memória: uma coluna `notified_price_cents` guarda o preço já
comunicado, e "queda pendente" passa a ser derivada por comparação em SQL.
`crawl.Notify` drena uma fila única de matches novos e quedas, ordenada por
desconto percentual.

**Tech Stack:** Go 1.2x, `modernc.org/sqlite` (driver puro, sem cgo),
`database/sql`, testes com a biblioteca padrão (`testing`).

**Spec:** `docs/superpowers/specs/2026-08-10-harley-hunter-fase3-design.md`

## Global Constraints

- Código **sem comentários**. Nomes explicam a intenção.
- TDD red-first: teste que falha primeiro, sempre, e rodado para ver falhar.
- Dinheiro em centavos `int64`. Nunca float para valor monetário.
- Valor ausente = ponteiro nil. Nil nunca virar zero silenciosamente.
- Commits sem menção a IA, sem `Co-Authored-By`, sem link de sessão.
- `.env` e `*.db` nunca entram em `git add`.
- Mensagens de commit em inglês, no estilo do repo (`fix:`, `feat:`, `test:`,
  frase descrevendo o comportamento, não o diff).
- Rodar `go build ./... && go test ./...` antes de cada commit.
- O banco de produção (`hunter.db`, ~485 anúncios) já existe: toda mudança de
  schema precisa funcionar via `addMissingColumns`, não só no `CREATE TABLE`.

---

### Task 1: A dedup para de silenciar duas motos da mesma fonte

O furo: `Notify` marca a segunda linha de um fingerprint repetido como
notificada **sem enviar**, e `notified` é irreversível. O par real 237/251
(webmotors, 53.000 e 53.118 km) caiu no mesmo balde de 5.000 km, mesma cidade,
mesmo ano — duas motos, uma silenciada para sempre.

Regra nova: só silencia quando a fonte é **diferente** (cross-post). Mesma
fonte com ambos ativos são duas motos e ambas alertam.

**Files:**
- Modify: `internal/crawl/crawl.go:169-221` (`Notify`), `crawl.go:237-242` (`dedupKey`)
- Test: `internal/crawl/notify_test.go`

**Interfaces:**
- Consumes: `store.Row` (campos `Source`, `Fingerprint`, `Km`), `store.PendingNotifications`, `store.MarkNotified(id int64) error` — assinatura ainda a de hoje.
- Produces: a mecânica de dedup dentro de `Notify`, consumida pela Task 6.

> **Emenda (durante a execução, aprovada pelo dono).** A forma com booleano por
> fonte descrita nos steps abaixo tem um furo da mesma classe do bug-alvo,
> encontrado e verificado empiricamente pelo implementer: o mapa só registra
> fontes que **enviaram**, então em `olx(X)`, `wm(X)`, `wm(Y)` a moto Y é
> silenciada para sempre — webmotors não consta como "já alertou" porque foi a
> olx que alertou X. A forma correta conta anúncios por fonte e compara com
> quantos alertas o fingerprint já produziu: silencia quando a contagem da
> fonte é ≤ o número de alertas do fingerprint. Ver a tabela de quatro casos na
> spec. Os steps abaixo ficam como registro do que foi implementado primeiro; a
> forma final está no commit da Task 1.

- [ ] **Step 1: Escrever o teste que falha**

Em `internal/crawl/notify_test.go`, adicione:

```go
func TestNotifyKeepsBothWhenTheSameSourceRepeatsAFingerprint(t *testing.T) {
	s := openStore(t)

	first := withKm(matchListing("wm-237"), 53000)
	first.Source = "webmotors"
	first.Fingerprint = "aa11bb22cc33dd44"

	second := withKm(matchListing("wm-251"), 53118)
	second.Source = "webmotors"
	second.Fingerprint = first.Fingerprint

	for _, l := range []model.Listing{first, second} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2: two active ads on one source are two bikes", sent)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("%d rows left pending, want 0: both were alerted", len(pending))
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/crawl/ -run TestNotifyKeepsBothWhenTheSameSourceRepeatsAFingerprint -v`
Expected: FAIL — `sent = 1, want 2` (a segunda linha é silenciada pela regra antiga).

- [ ] **Step 3: Trocar a regra de dedup**

Em `internal/crawl/crawl.go`, dentro de `Notify`, troque a declaração de `seen`
e o teste de duplicidade. De:

```go
	seen := make(map[string]bool, len(pending))
	alerted := make(map[int64]bool, len(pending))
	for _, row := range pending {
		key, dedupable := dedupKey(row)
		if dedupable && seen[key] {
```

Para:

```go
	seen := make(map[string]map[string]bool, len(pending))
	alerted := make(map[int64]bool, len(pending))
	for _, row := range pending {
		key, dedupable := dedupKey(row)
		if dedupable && crossPost(seen, key, row.Source) {
```

E onde hoje registra `seen[key] = true`:

```go
		if dedupable {
			seen[key] = true
		}
```

para:

```go
		if dedupable {
			if seen[key] == nil {
				seen[key] = make(map[string]bool)
			}
			seen[key][row.Source] = true
		}
```

Adicione a função, logo abaixo de `dedupKey`:

```go
func crossPost(seen map[string]map[string]bool, key, source string) bool {
	sources := seen[key]
	if len(sources) == 0 {
		return false
	}
	return !sources[source]
}
```

- [ ] **Step 4: Rodar o teste novo para ver passar**

Run: `go test ./internal/crawl/ -run TestNotifyKeepsBothWhenTheSameSourceRepeatsAFingerprint -v`
Expected: PASS

- [ ] **Step 5: Consertar o teste que a regra nova invalida**

`TestNotifyDedupSkipsDoNotConsumeCapSlots` usa 3 duplicatas da **mesma** fonte
(`matchListing` fixa `olx`) e espera 1 envio. Com a regra nova as três alertam,
e o teste passa a medir a coisa errada. O propósito dele — "skip de dedup não
consome slot do cap" — só existe em cross-post. Reescreva o corpo para usar
fontes diferentes:

```go
func TestNotifyDedupSkipsDoNotConsumeCapSlots(t *testing.T) {
	s := openStore(t)

	crossPosted := []string{"olx", "mercadolivre", "webmotors"}
	var listings []model.Listing
	for i, src := range crossPosted {
		l := withKm(matchListing(fmt.Sprintf("dup-%d", i)), 90195)
		l.Source = src
		l.Fingerprint = "b5778ec73e0cac33"
		listings = append(listings, l)
	}
	for i := 0; i < 5; i++ {
		l := withKm(matchListing(fmt.Sprintf("uniq-%d", i)), 10000+i)
		l.Fingerprint = fmt.Sprintf("unique-%d", i)
		listings = append(listings, l)
	}
	for _, l := range listings {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 5 {
		t.Errorf("sent = %d, want 5: the two cross-post skips must not eat cap slots", sent)
	}

	pending, err := s.PendingNotifications(50)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("%d rows left pending, want 1: 3 cross-posts collapse to 1 send, 5 sends spend the cap", len(pending))
	}
}
```

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go build ./... && go test ./...`
Expected: PASS em todos os pacotes. `TestNotifyDeduplicatesByFingerprint` já usa
olx + mercadolivre e continua verde sem alteração.

- [ ] **Step 7: Commit**

```bash
git add internal/crawl/crawl.go internal/crawl/notify_test.go
git commit -m "fix: two active ads on one source are two bikes, not a repost"
```

---

### Task 2: Comando para devolver à fila quem foi silenciado antes do fix

As vítimas da regra antiga já estão no banco com `notified = 1` e nunca
alertariam. O comando devolve à fila as linhas que compartilham fingerprint
**e** fonte com outra linha ativa, preservando a mais antiga de cada grupo como
já notificada.

Comando explícito, não migração automática: reenfileirar alerta é efeito
visível e quem decide a hora é o dono.

**Files:**
- Modify: `internal/store/lifecycle.go` (nova função no fim do arquivo)
- Modify: `cmd/hunter/main.go:33-47` (novo case no switch), `main.go:45` (linha de usage)
- Test: `internal/store/lifecycle_test.go`

**Interfaces:**
- Consumes: `Store.db` (dentro do pacote `store`).
- Produces: `func (s *Store) RequeueSilencedTwins() (int, error)` — devolve quantas linhas voltaram para a fila. Consumida por `cmd/hunter/main.go`.

- [ ] **Step 1: Escrever o teste que falha**

Em `internal/store/lifecycle_test.go`, adicione:

```go
func TestRequeueSilencedTwinsFreesTheYoungerOfTwoOnOneSource(t *testing.T) {
	s := openTemp(t)

	twin := func(id string, km int) model.Listing {
		l := sample(6100000)
		l.Source = "webmotors"
		l.ExternalID = id
		l.Km = &km
		l.Fingerprint = "aa11bb22cc33dd44"
		return l
	}

	older, err := s.Upsert(twin("237", 53000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	younger, err := s.Upsert(twin("251", 53118), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	for _, id := range []int64{older.ID, younger.ID} {
		if err := s.MarkNotified(id); err != nil {
			t.Fatalf("MarkNotified: %v", err)
		}
	}

	freed, err := s.RequeueSilencedTwins()
	if err != nil {
		t.Fatalf("RequeueSilencedTwins: %v", err)
	}
	if freed != 1 {
		t.Fatalf("freed = %d, want 1: only the younger twin was silenced", freed)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != younger.ID {
		t.Errorf("pending = %+v, want just the younger twin %d", pending, younger.ID)
	}
}

func TestRequeueSilencedTwinsLeavesACrossPostAlone(t *testing.T) {
	s := openTemp(t)

	posted := func(source, id string) model.Listing {
		l := sample(6100000)
		l.Source = source
		l.ExternalID = id
		km := 53000
		l.Km = &km
		l.Fingerprint = "aa11bb22cc33dd44"
		return l
	}

	for _, l := range []model.Listing{posted("olx", "1"), posted("mercadolivre", "2")} {
		res, err := s.Upsert(l, time.Now())
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		if err := s.MarkNotified(res.ID); err != nil {
			t.Fatalf("MarkNotified: %v", err)
		}
	}

	freed, err := s.RequeueSilencedTwins()
	if err != nil {
		t.Fatalf("RequeueSilencedTwins: %v", err)
	}
	if freed != 0 {
		t.Errorf("freed = %d, want 0: a cross-post was correctly deduped", freed)
	}
}
```

`openTemp` e `sample` já existem no pacote (`internal/store/store_test.go:11` e
`:21`) e são visíveis de `lifecycle_test.go`, que é o mesmo pacote. Não crie
helper novo. `sample` já traz `Fingerprint: "deadbeef"` e `Km: 31000`; os testes
acima sobrescrevem os dois de propósito.

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/store/ -run TestRequeueSilencedTwins -v`
Expected: FAIL na compilação — `s.RequeueSilencedTwins undefined`.

- [ ] **Step 3: Implementar a função**

No fim de `internal/store/lifecycle.go`:

```go
func (s *Store) RequeueSilencedTwins() (int, error) {
	res, err := s.db.Exec(
		`UPDATE listings SET notified = 0
         WHERE status = ? AND verdict = 'match' AND notified = 1
           AND km IS NOT NULL AND fingerprint <> ''
           AND id NOT IN (
               SELECT MIN(id) FROM listings
               WHERE status = ? AND verdict = 'match'
                 AND km IS NOT NULL AND fingerprint <> ''
               GROUP BY fingerprint, source)`,
		StatusActive, StatusActive)
	if err != nil {
		return 0, fmt.Errorf("requeueing silenced twins: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("counting requeued twins: %w", err)
	}
	return int(affected), nil
}
```

Grupo com uma linha só é o próprio `MIN(id)` e sai pelo `NOT IN`, então não
precisa de `HAVING COUNT(*) > 1`.

- [ ] **Step 4: Rodar o teste para ver passar**

Run: `go test ./internal/store/ -run TestRequeueSilencedTwins -v`
Expected: PASS nos dois testes.

- [ ] **Step 5: Ligar o comando na CLI**

Em `cmd/hunter/main.go`, adicione o case antes do `default`:

```go
	case "repair-silenced":
		if err := runRepairSilenced(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
```

Atualize a linha de usage:

```go
		fmt.Fprintln(os.Stderr, "usage: hunter [-config path] <crawl|serve|repair-silenced>")
```

E adicione a função, depois de `runServe`:

```go
func runRepairSilenced(cfg config.Config) error {
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	freed, err := db.RequeueSilencedTwins()
	if err != nil {
		return err
	}
	fmt.Printf("listings back in the alert queue: %d\n", freed)
	return nil
}
```

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store/lifecycle.go internal/store/lifecycle_test.go cmd/hunter/main.go
git commit -m "feat: give the bikes silenced by the old dedup their alert back"
```

---

### Task 3: A coluna `notified_price_cents` e a âncora das linhas existentes

A coluna guarda **o preço que já foi comunicado** para aquele anúncio. É o que
transforma "queda pendente" em estado derivável, em vez de uma lista em memória
que morre com o processo.

Precisa entrar em dois lugares: no `schema` (bancos novos) e em `addedColumns`
(o `hunter.db` de produção, que já existe). O seed que ancora as linhas
`notified = 1` roda **só no momento em que a coluna é criada** — rodar de novo
reancoraria no preço já caído e apagaria quedas pendentes.

**Files:**
- Modify: `internal/store/store.go:47-112` (`schema`), `store.go:17-40` (`Row`), `store.go:135-149` (`rowColumns`, `scanRow`)
- Modify: `internal/store/migrate.go:8-31` (`addedColumns`, `addMissingColumns`)
- Test: `internal/store/migrate_test.go`

**Interfaces:**
- Consumes: `addMissingColumns(db *sql.DB) error`, chamada por `Open`.
- Produces: `Row.Notified bool` e `Row.NotifiedPriceCents *int64` — lidos pelas tasks 4, 5 e 6. Coluna SQL `notified_price_cents INTEGER`.

- [ ] **Step 1: Escrever o teste que falha**

Em `internal/store/migrate_test.go`, adicione:

```go
func TestOpenAnchorsAlreadyNotifiedRowsAtTheirCurrentPrice(t *testing.T) {
	path := openLegacy(t)

	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopening legacy database: %v", err)
	}
	if _, err := legacy.Exec("UPDATE listings SET notified = 1, price_cents = 7200000"); err != nil {
		t.Fatalf("marking the legacy row notified: %v", err)
	}
	legacy.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	rows, err := s.ListByVerdict("match")
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want the legacy row", len(rows))
	}
	if rows[0].NotifiedPriceCents == nil || *rows[0].NotifiedPriceCents != 7200000 {
		t.Errorf("NotifiedPriceCents = %v, want the price already communicated", rows[0].NotifiedPriceCents)
	}
}

func TestOpenDoesNotReanchorAPendingDropOnASecondRun(t *testing.T) {
	path := openLegacy(t)

	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopening legacy database: %v", err)
	}
	if _, err := legacy.Exec("UPDATE listings SET notified = 1, price_cents = 7200000"); err != nil {
		t.Fatalf("marking the legacy row notified: %v", err)
	}
	legacy.Close()

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	first.Close()

	dropped, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopening to drop the price: %v", err)
	}
	if _, err := dropped.Exec("UPDATE listings SET price_cents = 6800000"); err != nil {
		t.Fatalf("dropping the price: %v", err)
	}
	dropped.Close()

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	t.Cleanup(func() { second.Close() })

	rows, err := second.ListByVerdict("match")
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if rows[0].NotifiedPriceCents == nil || *rows[0].NotifiedPriceCents != 7200000 {
		t.Errorf("NotifiedPriceCents = %v, want the anchor to survive so the drop stays pending",
			rows[0].NotifiedPriceCents)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/store/ -run TestOpen -v`
Expected: FAIL na compilação — `rows[0].NotifiedPriceCents undefined`.

- [ ] **Step 3: Adicionar a coluna ao schema e à Row**

Em `internal/store/store.go`, no bloco `schema`, dentro do `CREATE TABLE
listings`, depois da linha `notified INTEGER NOT NULL DEFAULT 0,`:

```sql
    notified_price_cents INTEGER,
```

No struct `Row`, depois de `FirstPriceCents *int64`:

```go
	Notified           bool
	NotifiedPriceCents *int64
```

Em `rowColumns`, acrescente as duas colunas no fim da lista (depois da subquery
de `price_history`, separadas por vírgula):

```go
const rowColumns = `
    l.id, l.source, l.external_id, l.url, l.title, l.bike, l.variant, l.year,
    l.price_cents, l.km, l.city, l.state, l.image_url, l.phone, l.verdict, l.user_state,
    l.status, l.fingerprint, l.published_at, l.first_seen_at, l.last_seen_at,
    (SELECT price_cents FROM price_history p WHERE p.listing_id = l.id ORDER BY p.observed_at ASC, p.id ASC LIMIT 1),
    l.notified, l.notified_price_cents
`
```

Em `scanRow`, acrescente os dois destinos na mesma ordem, no fim:

```go
	err := scanner.Scan(&r.ID, &r.Source, &r.ExternalID, &r.URL, &r.Title, &r.Bike,
		&r.Variant, &r.Year, &r.PriceCents, &r.Km, &r.City, &r.State, &r.ImageURL,
		&r.Phone, &r.Verdict, &r.UserState, &r.Status, &r.Fingerprint, &r.PublishedAt,
		&r.FirstSeenAt, &r.LastSeenAt, &r.FirstPriceCents, &r.Notified, &r.NotifiedPriceCents)
```

- [ ] **Step 4: Ensinar a migração a criar a coluna e ancorar as linhas**

Em `internal/store/migrate.go`, acrescente o campo `seed` ao struct e a nova
entrada:

```go
var addedColumns = []struct {
	table  string
	column string
	ddl    string
	seed   string
}{
	{table: "listings", column: "phone", ddl: "ALTER TABLE listings ADD COLUMN phone TEXT"},
	{table: "listings", column: "published_at", ddl: "ALTER TABLE listings ADD COLUMN published_at TIMESTAMP"},
	{
		table:  "listings",
		column: "notified_price_cents",
		ddl:    "ALTER TABLE listings ADD COLUMN notified_price_cents INTEGER",
		seed:   "UPDATE listings SET notified_price_cents = price_cents WHERE notified = 1 AND price_cents IS NOT NULL",
	},
}
```

E execute o seed logo após o ALTER, dentro do mesmo bloco condicional:

```go
		if _, err := db.Exec(c.ddl); err != nil {
			return fmt.Errorf("adding column %s.%s: %w", c.table, c.column, err)
		}
		if c.seed == "" {
			continue
		}
		if _, err := db.Exec(c.seed); err != nil {
			return fmt.Errorf("seeding column %s.%s: %w", c.table, c.column, err)
		}
```

O seed fica dentro do `if !present`, que é o que garante uma execução única.

- [ ] **Step 5: Rodar os testes para ver passar**

Run: `go test ./internal/store/ -run TestOpen -v`
Expected: PASS em todos, incluindo
`TestOpenIsSafeToRunTwiceOverTheSameDatabase`.

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go build ./... && go test ./...`
Expected: PASS. `internal/web` também consome `store.Row`; campos novos não
quebram nada, mas confirme que compila.

- [ ] **Step 7: Commit**

```bash
git add internal/store/store.go internal/store/migrate.go internal/store/migrate_test.go
git commit -m "feat: remember which price was already announced for a listing"
```

---

### Task 4: `MarkNotified` grava o preço comunicado e nunca reancora para cima

A âncora fica no **menor** preço já comunicado. Se o preço sobe, a coluna não
se move: preço que sobe e volta ao patamar antigo não é notícia. Só uma queda
abaixo do menor já comunicado volta a alertar.

**Files:**
- Modify: `internal/store/store.go:173-178` (`MarkNotified`)
- Modify: `internal/crawl/crawl.go` (as duas chamadas de `MarkNotified` dentro de `Notify`)
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `Row.NotifiedPriceCents` (Task 3).
- Produces: `func (s *Store) MarkNotified(id int64, priceCents *int64) error` — assinatura nova, com o preço comunicado. Consumida por `crawl.Notify` (tasks 1 e 6) e pelos testes da Task 2.

- [ ] **Step 1: Escrever o teste que falha**

Em `internal/store/store_test.go`, adicione:

```go
func TestMarkNotifiedAnchorsAtTheLowestPriceAnnounced(t *testing.T) {
	s := openTemp(t)

	res, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	anchor := func(cents int64) *int64 { return &cents }

	if err := s.MarkNotified(res.ID, anchor(7200000)); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 7200000 {
		t.Fatalf("anchor = %v, want 7200000", got)
	}

	if err := s.MarkNotified(res.ID, anchor(7500000)); err != nil {
		t.Fatalf("MarkNotified on a higher price: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 7200000 {
		t.Errorf("anchor = %v, want it to stay at 7200000: a price rise is not news", got)
	}

	if err := s.MarkNotified(res.ID, anchor(6800000)); err != nil {
		t.Fatalf("MarkNotified on a lower price: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 6800000 {
		t.Errorf("anchor = %v, want 6800000: a drop moves the anchor down", got)
	}

	if err := s.MarkNotified(res.ID, nil); err != nil {
		t.Fatalf("MarkNotified without a price: %v", err)
	}
	if got := notifiedPriceOf(t, s, res.ID); got == nil || *got != 6800000 {
		t.Errorf("anchor = %v, want 6800000 kept: a priceless round must not erase it", got)
	}
}

func notifiedPriceOf(t *testing.T, s *Store, id int64) *int64 {
	t.Helper()
	row, _, err := s.GetRow(id)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	return row.NotifiedPriceCents
}
```

`openTemp` (`internal/store/store_test.go:11`) e `sample` (`:21`) já existem no
pacote. `notifiedPriceOf` é novo e é o único helper que esta task acrescenta.

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/store/ -run TestMarkNotifiedAnchors -v`
Expected: FAIL na compilação — `too many arguments in call to s.MarkNotified`.

- [ ] **Step 3: Implementar a nova assinatura**

Em `internal/store/store.go`, substitua `MarkNotified`:

```go
func (s *Store) MarkNotified(id int64, priceCents *int64) error {
	_, err := s.db.Exec(
		`UPDATE listings SET notified = 1,
             notified_price_cents = CASE
                 WHEN ? IS NULL THEN notified_price_cents
                 WHEN notified_price_cents IS NULL THEN ?
                 WHEN ? < notified_price_cents THEN ?
                 ELSE notified_price_cents END
         WHERE id = ?`,
		priceCents, priceCents, priceCents, priceCents, id)
	if err != nil {
		return fmt.Errorf("marking listing as notified: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Atualizar os chamadores**

Em `internal/crawl/crawl.go`, dentro de `Notify`, as duas chamadas passam a
levar o preço da linha:

```go
			if err := s.MarkNotified(row.ID, row.PriceCents); err != nil {
```

Nos testes da Task 2 (`internal/store/lifecycle_test.go`), as chamadas
`s.MarkNotified(id)` viram `s.MarkNotified(id, nil)`.

Confirme que não sobrou nenhum chamador antigo:

Run: `grep -rn "MarkNotified(" --include=*.go .`
Expected: toda chamada com dois argumentos.

- [ ] **Step 5: Rodar os testes para ver passar**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go internal/store/lifecycle_test.go internal/crawl/crawl.go
git commit -m "feat: anchor a listing at the lowest price already announced"
```

---

### Task 5: `PendingAlerts`, a fila única no store

Uma query devolve as duas coisas que merecem alerta: match novo
(`notified = 0`) e queda pendente (preço atual abaixo do preço já comunicado).
`PendingNotifications` fica como está — os testes a usam para verificar o que
sobrou na fila de matches.

**Files:**
- Modify: `internal/store/store.go` (nova função depois de `PendingNotifications`)
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: coluna `notified_price_cents` (Task 3).
- Produces: `func (s *Store) PendingAlerts(limit int) ([]Row, error)` — matches novos e quedas pendentes, `first_seen_at ASC, id ASC`. Consumida por `crawl.Notify` (Task 6).

- [ ] **Step 1: Escrever o teste que falha**

Em `internal/store/store_test.go`, adicione:

```go
func TestPendingAlertsCarriesNewMatchesAndPriceDrops(t *testing.T) {
	s := openTemp(t)

	fresh, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	dropping := sample(7200000)
	dropping.ExternalID = "dropping"
	res, err := s.Upsert(dropping, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	anchored := int64(7200000)
	if err := s.MarkNotified(res.ID, &anchored); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	steady := sample(7000000)
	steady.ExternalID = "steady"
	quiet, err := s.Upsert(steady, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	held := int64(7000000)
	if err := s.MarkNotified(quiet.ID, &held); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	pending, err := s.PendingAlerts(10)
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != fresh.ID {
		t.Fatalf("pending = %d rows, want just the new match", len(pending))
	}

	dropped := dropping
	lower := int64(6800000)
	dropped.PriceCents = &lower
	if _, err := s.Upsert(dropped, time.Now()); err != nil {
		t.Fatalf("Upsert after the drop: %v", err)
	}

	pending, err = s.PendingAlerts(10)
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d rows, want the new match and the drop", len(pending))
	}
	var sawDrop bool
	for _, row := range pending {
		if row.ID == res.ID {
			sawDrop = true
		}
		if row.ID == quiet.ID {
			t.Error("a listing whose price did not move must stay out of the queue")
		}
	}
	if !sawDrop {
		t.Error("the drop is missing from the queue")
	}
}

func TestPendingAlertsIgnoresADropOnAListingAlreadyGone(t *testing.T) {
	s := openTemp(t)

	l := sample(7200000)
	res, err := s.Upsert(l, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	anchored := int64(7200000)
	if err := s.MarkNotified(res.ID, &anchored); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	lower := int64(6800000)
	l.PriceCents = &lower
	if _, err := s.Upsert(l, time.Now()); err != nil {
		t.Fatalf("Upsert after the drop: %v", err)
	}
	if _, err := s.db.Exec("UPDATE listings SET status = ? WHERE id = ?", StatusGone, res.ID); err != nil {
		t.Fatalf("closing the listing: %v", err)
	}

	pending, err := s.PendingAlerts(10)
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %d rows, want 0: a closed ad is not an opportunity", len(pending))
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/store/ -run TestPendingAlerts -v`
Expected: FAIL na compilação — `s.PendingAlerts undefined`.

- [ ] **Step 3: Implementar a query**

Em `internal/store/store.go`, depois de `PendingNotifications`:

```go
func (s *Store) PendingAlerts(limit int) ([]Row, error) {
	query := "SELECT " + rowColumns + ` FROM listings l
        WHERE l.verdict = 'match' AND l.status = 'active'
          AND (l.notified = 0
               OR (l.price_cents IS NOT NULL AND l.notified_price_cents IS NOT NULL
                   AND l.price_cents < l.notified_price_cents))
        ORDER BY l.first_seen_at ASC, l.id ASC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("querying pending alerts: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/store/ -run TestPendingAlerts -v`
Expected: PASS nos dois.

- [ ] **Step 5: Rodar a suíte inteira e commitar**

```bash
go build ./... && go test ./...
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: one queue for new matches and pending price drops"
```

---

### Task 6: `Notify` drena a fila única, ordenada por urgência

Fecha o furo: as quedas param de viver em memória. `Report.Drops`,
`PriceDrop` e `priceDrop` saem. `Notify` lê a fila do store, ordena por
desconto percentual e corta no cap — o que não couber continua pendente por
construção, e uma falha de envio não perde nada.

Urgência = desconto percentual contra a melhor referência disponível:
o preço já comunicado quando houve queda; senão a FIPE; senão 0. Desempate por
`first_seen_at`, que é a ordem que o SQL já entrega (`sort.SliceStable`).

**Files:**
- Modify: `internal/crawl/crawl.go:53-57` (remover `PriceDrop`), `crawl.go:59-67` (remover `Drops` do `Report`), `crawl.go:146-148` (remover a coleta), `crawl.go:169-235` (`Notify` e `priceDrop`)
- Modify: `cmd/hunter/main.go:72` e `main.go:99-106` (`sendAlerts`)
- Test: `internal/crawl/drop_test.go` (reescrito)

**Interfaces:**
- Consumes: `store.PendingAlerts` (Task 5), `store.MarkNotified(id, priceCents)` (Task 4), `Row.NotifiedPriceCents` (Task 3), a mecânica de dedup já no arquivo (Task 1 — preservar, não reescrever), `fipe.Table.Lookup(bike, variant string, year int) (*fipe.Reference, bool)`.
- Produces: `func Notify(ctx context.Context, s *store.Store, n notify.Notifier, limit int, refs *fipe.Table) (int, error)`. O quinto parâmetro deixa de ser `[]PriceDrop` e passa a ser a tabela FIPE — `nil` é válido e vale como "sem referência".

- [ ] **Step 1: Escrever o teste que falha**

Substitua o conteúdo de `internal/crawl/drop_test.go` por:

```go
package crawl

import (
	"context"
	"strings"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/fipe"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func pricedAt(source, id, price string) model.RawListing {
	raw := harley(source, id)
	raw.PriceText = price
	return raw
}

func runWith(t *testing.T, s *store.Store, items ...model.RawListing) Report {
	t.Helper()
	report, err := Run(context.Background(), []Source{fakeSource{name: "olx", items: items}}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return report
}

func drainAlerts(t *testing.T, s *store.Store) {
	t.Helper()
	if _, err := Notify(context.Background(), s, &recordingNotifier{}, 5, nil); err != nil {
		t.Fatalf("draining the pending alerts: %v", err)
	}
}

func TestNotifyAlertsThePriceDropLeadingWithTheSignal(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼ R$ 4.000: ") {
		t.Errorf("alert = %q, want it to lead with the drop", n.messages[0])
	}
	if !strings.Contains(n.messages[0], "R$ 68.000") {
		t.Errorf("alert = %q, want the price it dropped to", n.messages[0])
	}
	if n.alerts[0].URL != "https://example.com/1" {
		t.Errorf("alert url = %q, want the listing url", n.alerts[0].URL)
	}
}

func TestNotifyKeepsADropPendingWhenTheCapIsSpent(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"), pricedAt("olx", "9", "R$ 71.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 1, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: the cap allows one", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼") {
		t.Errorf("alert = %q, want the measured drop ahead of a match with no reference", n.messages[0])
	}

	rest := &recordingNotifier{}
	sent, err = Notify(context.Background(), s, rest, 5, nil)
	if err != nil {
		t.Fatalf("second Notify: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1: what did not fit must survive the round", sent)
	}
}

func TestNotifyKeepsADropPendingWhenDeliveryFails(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	if _, err := Notify(context.Background(), s, &recordingNotifier{failAt: 1}, 5, nil); err == nil {
		t.Fatal("Notify should surface the delivery error")
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify after the failure: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1: a failed delivery leaves the drop pending", sent)
	}
}

func TestNotifyCollapsesTwoDropsIntoTheAccumulatedOne(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 70.000"))
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: two drops before delivery are one piece of news", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼ R$ 4.000: ") {
		t.Errorf("alert = %q, want the accumulated drop from 72.000 to 68.000", n.messages[0])
	}
}

func TestNotifyStaysQuietWhenThePriceGoesBackUp(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0: a price rise is not news", sent)
	}

	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))
	back := &recordingNotifier{}
	sent, err = Notify(context.Background(), s, back, 5, nil)
	if err != nil {
		t.Fatalf("Notify after coming back down: %v", err)
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0: coming back to a price already announced is not news", sent)
	}
}

func TestNotifyRanksTheBiggerDiscountFirst(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"), pricedAt("olx", "2", "R$ 72.000"))
	drainAlerts(t, s)
	runWith(t, s, pricedAt("olx", "1", "R$ 70.000"), pricedAt("olx", "2", "R$ 64.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 1, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if !strings.HasPrefix(n.messages[0], "▼ R$ 8.000: ") {
		t.Errorf("alert = %q, want the 8.000 drop ahead of the 2.000 one", n.messages[0])
	}
}

func TestNotifyRanksTheDeeperFipeDiscountFirst(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 72.000"), pricedAt("olx", "2", "R$ 60.000"))

	refs := fipe.NewTable([]fipe.Reference{{
		Bike:       model.BikeStreetGlide,
		Variant:    model.VariantSpecial,
		Year:       2015,
		PriceCents: 7500000,
	}})

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 1, refs)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if !strings.Contains(n.messages[0], "R$ 60.000") {
		t.Errorf("alert = %q, want the listing furthest below the fipe first", n.messages[0])
	}
}

func TestNotifyAlertsOnceWhenADropAlsoFlipsMaybeToMatch(t *testing.T) {
	s := openStore(t)
	runWith(t, s, pricedAt("olx", "1", "R$ 80.000"))

	rows, err := s.ListByVerdict(model.VerdictMaybe)
	if err != nil || len(rows) != 1 {
		t.Fatalf("seed should be a maybe: %d rows, %v", len(rows), err)
	}

	runWith(t, s, pricedAt("olx", "1", "R$ 68.000"))

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1: the flip alerts once, not twice", sent)
	}
	if strings.HasPrefix(n.messages[0], "▼") {
		t.Errorf("alert = %q, want the plain new-match alert", n.messages[0])
	}
}

func TestNotifyAlertsACrossPostDropOnlyOnce(t *testing.T) {
	s := openStore(t)

	posted := func(source, id string, cents int64, km int) model.Listing {
		l := matchListing(id)
		l.Source = source
		l.PriceCents = &cents
		l.Km = &km
		l.Fingerprint = "aa11bb22cc33dd44"
		return l
	}

	for _, l := range []model.Listing{posted("olx", "1", 7200000, 53000), posted("mercadolivre", "2", 7200000, 53000)} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	drainAlerts(t, s)

	for _, l := range []model.Listing{posted("olx", "1", 6800000, 53000), posted("mercadolivre", "2", 6800000, 53000)} {
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert after the drop: %v", err)
		}
	}

	n := &recordingNotifier{}
	sent, err := Notify(context.Background(), s, n, 5, nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1: both sides of a cross-post dropping is one piece of news", sent)
	}
}
```

Este arquivo passa a usar `time`; acrescente `"time"` ao bloco de imports.

Nota sobre a fixture: `harley()` (`internal/crawl/crawl_test.go:64`) fixa
`YearText: "2015"`, `KmText: "31.000 km"`, `LocationText: "Curitiba - PR"` e
título com "Special", então todo `pricedAt("olx", ...)` do mesmo teste
compartilha o **mesmo** fingerprint. Isso é proposital aqui: são todos da fonte
`olx`, e pela regra da Task 1 anúncios ativos da mesma fonte alertam
separadamente — o que corta o excedente nesses testes é o cap, não a dedup. Só
`TestNotifyAlertsACrossPostDropOnlyOnce` usa fontes diferentes, e é ali que a
dedup age.

Os testes que verificavam `Report.Drops` (`TestRunReportsAPriceDropOnAMatch`,
`TestRunReportsNoDropOnTheFirstSighting`, `TestRunIgnoresAPriceIncrease`,
`TestRunIgnoresADropOnAListingThatIsNotAMatch`,
`TestNotifySurfacesAFailedDropDelivery`,
`TestNotifyIgnoresADropWhoseListingIsGone`,
`TestNotifyDropsShareThePerRunCapWithNewMatches`) desaparecem: o campo que eles
inspecionavam deixa de existir, e cada comportamento que importava está coberto
acima ou nos testes de `PendingAlerts` da Task 5.

- [ ] **Step 2: Rodar os testes para ver falhar**

Run: `go test ./internal/crawl/ -v`
Expected: FAIL na compilação — `report.Drops` não existe mais nos testes, mas
`Notify` ainda espera `[]PriceDrop` no quinto parâmetro.

- [ ] **Step 3: Remover o caminho em memória**

Em `internal/crawl/crawl.go`, apague o tipo `PriceDrop`, o campo
`Drops []PriceDrop` do `Report`, a função `priceDrop`, e no laço de `Run`
apague as três linhas que coletavam a queda:

```go
			if drop, ok := priceDrop(res, listing); ok {
				report.Drops = append(report.Drops, drop)
			}
```

O `if res.IsNew { report.NewMatches++ }` continua.

- [ ] **Step 4: Reescrever `Notify`**

> **Atenção — não reescreva a dedup.** A Task 1 já resolveu a mecânica de
> dedup dentro deste laço (contagem de anúncios por fonte contra o número de
> alertas do fingerprint), e ela cobre um caso de três anúncios que uma forma
> mais simples silencia por engano. Abra `internal/crawl/crawl.go`, veja como
> a dedup está escrita **hoje** no arquivo, e preserve essa mecânica exatamente
> como está — variáveis, ordem das operações e a posição do check de cap depois
> da decisão de dedup. Esta task muda quatro coisas e nada mais: (1) a fonte da
> fila (`PendingAlerts` em vez de `PendingNotifications`), (2) a ordenação por
> urgência, (3) a mensagem, que passa a ser de queda quando houver queda
> pendente, (4) o segundo laço dos `drops` e o `alerted`, que somem. Se o
> resultado do seu diff apagar a contagem por fonte, você reverteu um bug fix —
> refaça.

O esqueleto abaixo mostra **onde** as quatro mudanças entram. Os trechos
marcados como preservados devem sair do arquivo, não daqui:

```go
func Notify(ctx context.Context, s *store.Store, n notify.Notifier, limit int, refs *fipe.Table) (int, error) {
	if limit <= 0 {
		limit = DefaultAlertsPerRun
	}
	pending, err := s.PendingAlerts(limit * pendingOversample)   // (1) muda
	if err != nil {
		return 0, err
	}
	sort.SliceStable(pending, func(i, j int) bool {              // (2) novo
		return urgency(pending[i], refs) > urgency(pending[j], refs)
	})

	sent := 0
	// dedup bookkeeping: preservar exatamente o que está no arquivo
	for _, row := range pending {
		// decisão de dedup: preservar; no ramo que silencia, a chamada de
		// MarkNotified agora leva row.PriceCents
		if sent >= limit {
			break
		}
		message := notify.FormatAlert(row)                       // (3) novo
		if previous, ok := pendingDrop(row); ok {
			message = notify.FormatPriceDrop(row, previous)
		}
		if err := n.Send(ctx, notify.Alert{Message: message, URL: row.URL}); err != nil {
			return sent, fmt.Errorf("sending alert for listing %d: %w", row.ID, err)
		}
		if err := s.MarkNotified(row.ID, row.PriceCents); err != nil {
			return sent, err
		}
		// registro do envio no bookkeeping de dedup: preservar
		sent++
	}
	return sent, nil                                            // (4) o laço de drops sai
}
```

Os comentários acima são instruções para você, não código — o repo proíbe
comentários, então nenhum deles vai para o arquivo.

func pendingDrop(row store.Row) (int64, bool) {
	if row.PriceCents == nil || row.NotifiedPriceCents == nil {
		return 0, false
	}
	if *row.PriceCents >= *row.NotifiedPriceCents {
		return 0, false
	}
	return *row.NotifiedPriceCents, true
}

func urgency(row store.Row, refs *fipe.Table) float64 {
	if row.PriceCents == nil {
		return 0
	}
	if previous, ok := pendingDrop(row); ok {
		return float64(previous-*row.PriceCents) / float64(previous)
	}
	if row.Year == nil {
		return 0
	}
	ref, ok := refs.Lookup(row.Bike, row.Variant, *row.Year)
	if !ok || ref.PriceCents <= *row.PriceCents {
		return 0
	}
	return float64(ref.PriceCents-*row.PriceCents) / float64(ref.PriceCents)
}
```

O `alerted map[int64]bool` sai junto: não existe mais segunda passada. Acrescente
`"sort"` e `"github.com/andreabreu76/harley-hunter/internal/fipe"` aos imports.
`fipe` não importa `store` (usa uma interface local), então não há ciclo.
`Table.Lookup` já trata receiver nil, o que faz `refs = nil` valer como "sem
referência".

- [ ] **Step 5: Atualizar o chamador na CLI**

Em `cmd/hunter/main.go`, a chamada em `runCrawl` passa a ser:

```go
	sendAlerts(cfg, db)
```

E `sendAlerts`:

```go
func sendAlerts(cfg config.Config, db *store.Store) {
	refs, err := db.FipeReferences()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fipe references unavailable, alerts will not rank by discount: %v\n", err)
	}
	shown, err := crawl.Notify(context.Background(), db, notify.NewMacOS(), cfg.Crawl.MaxAlertsPerRun, fipe.NewTable(refs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "alerts failed after %d notifications: %v\n", shown, err)
		return
	}
	fmt.Printf("alerts shown: %d\n", shown)
}
```

Uma FIPE indisponível degrada a ordenação e aparece no stderr; não bloqueia o
alerta.

- [ ] **Step 6: Rodar os testes para ver passar**

Run: `go build ./... && go test ./...`
Expected: PASS em todos os pacotes.

- [ ] **Step 7: Commit**

```bash
git add internal/crawl/crawl.go internal/crawl/drop_test.go cmd/hunter/main.go
git commit -m "fix: a price drop now survives a spent cap and a failed send"
```

---

### Task 7: Verificação contra o banco de produção

O `hunter.db` real tem ~485 anúncios e a coluna nova ainda não existe nele. Esta
task confirma que a migração e o reparo funcionam sobre dados de verdade, numa
**cópia**, antes de o launchd rodar em cima do original.

**Files:**
- Nenhum arquivo de código. Verificação e, se necessário, recompilação do
  binário de produção.

**Interfaces:**
- Consumes: tudo das tasks 1-6.

- [ ] **Step 1: Copiar o banco de produção**

```bash
cd ~/src/github.com/andreabreu76/harley-hunter
cp hunter.db hunter.db-wal hunter.db-shm /private/tmp/ 2>/dev/null
mv /private/tmp/hunter.db /private/tmp/hunter-fase3-check.db
mv /private/tmp/hunter.db-wal /private/tmp/hunter-fase3-check.db-wal 2>/dev/null
mv /private/tmp/hunter.db-shm /private/tmp/hunter-fase3-check.db-shm 2>/dev/null
```

O banco roda em WAL: copiar só o `.db` deixa de fora o que ainda está no
write-ahead log. Nunca rodar a verificação sobre `hunter.db` direto — o launchd
dispara a cada 7200s e pode cair no meio.

- [ ] **Step 2: Migrar a cópia e conferir a âncora**

Crie `/private/tmp/fase3.yaml` copiando `config/config.yaml` e trocando **apenas**
`database_path` para `/private/tmp/hunter-fase3-check.db`:

```bash
sed 's|^database_path:.*|database_path: /private/tmp/hunter-fase3-check.db|' \
    config/config.yaml > /private/tmp/fase3.yaml
go run ./cmd/hunter -config /private/tmp/fase3.yaml repair-silenced
```

O `repair-silenced` abre o store, o que dispara `addMissingColumns` e cria a
coluna com o seed. Confirme que a coluna existe e que as linhas notificadas
ficaram ancoradas:

```bash
sqlite3 /private/tmp/hunter-fase3-check.db \
  "SELECT COUNT(*) FROM listings WHERE notified = 1 AND notified_price_cents IS NULL AND price_cents IS NOT NULL;"
```

Expected: `0` — toda linha já notificada com preço tem âncora. Se `sqlite3` não
estiver instalado, pule esta conferência e confie nos testes da Task 3.

Expected do comando anterior: roda sem erro e imprime
`listings back in the alert queue: N`.
`N` deve ser pequeno (as vítimas reais da colisão, o par 237/251 entre elas).
Se `N` vier alto (dezenas), **pare** e reporte: significa que a colisão
same-source é mais comum que o previsto e o dono precisa decidir se quer esse
volume de alertas de uma vez.

- [ ] **Step 3: Conferir que a fila não explodiu**

```bash
go run ./cmd/hunter -config /private/tmp/fase3.yaml serve
```

Abra `localhost:8080`, confirme que os cards continuam renderizando (a `Row`
ganhou dois campos, o dashboard consome a mesma struct) e que nada quebrou na
página. Encerre com Ctrl-C.

- [ ] **Step 4: Limpar**

```bash
rm -f /private/tmp/hunter-fase3-check.db* /private/tmp/fase3.yaml
```

- [ ] **Step 5: Relatar ao dono antes de recompilar produção**

Não recompile `~/bin/hunter` nem toque em `hunter.db` sem o aval do dono.
Reporte: quantas linhas o `repair-silenced` devolveria à fila, e que a
migração roda limpa sobre a cópia. A decisão de quando aplicar em produção é
dele.

---

## Self-Review

**Cobertura da spec:**

| Requisito da spec | Task |
|---|---|
| Regra fingerprint+fonte (tabela de decisão) | 1 |
| `dedupKey` mantém a exigência de km | 1 (inalterado) |
| Reanúncio continua alertando | 1 (lado gone não está pendente) |
| Reparo das vítimas já no banco | 2 |
| Coluna `notified_price_cents` | 3 |
| Seed one-shot, sem reancorar em segunda execução | 3 |
| Subida não reancora | 4 |
| Queda pendente derivada em SQL | 5 |
| Fila única, `Report.Drops` removido | 6 |
| Urgência: queda → FIPE → 0, desempate por `first_seen_at` | 6 |
| Dedup vale para a fila inteira | 6 (`crossPost` no loop unificado) |
| Cross-post silenciado também ancora preço | 6 (`MarkNotified` no ramo do skip) |
| Migração sobre o banco de produção real | 7 |

Todos os 11 testes listados na spec estão distribuídos entre as tasks 1-6.

**Consistência de tipos:** `MarkNotified(id int64, priceCents *int64) error`
definida na Task 4 e usada com dois argumentos nas tasks 2, 4 e 6.
`PendingAlerts(limit int) ([]Row, error)` definida na 5, consumida na 6.
`crossPost` definida na 1, reusada na 6. `Row.NotifiedPriceCents *int64`
definida na 3, lida nas 4, 5 e 6. `Notify` termina com cinco parâmetros, o
último `*fipe.Table`, o que mantém compilando as chamadas `Notify(..., nil)`
dos testes já existentes em `notify_test.go`.

**Sem placeholders:** todo step de código traz o código. Os dois pontos em que
o implementador precisa olhar o repo antes de escrever (nomes dos helpers de
teste do pacote `store`, nas tasks 2 e 4) vêm com o comando `grep` exato e a
instrução de não criar helper novo.
