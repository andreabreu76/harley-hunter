# Harley Hunter — Plano de Implementação (Fase 1: núcleo)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar o sistema completo funcionando ponta a ponta com duas fontes (OLX e Mercado Livre): coleta agendada, classificação, banco com histórico de preço, alerta por SMS e dashboard local.

**Architecture:** Binário Go único com subcomandos `crawl` e `serve`. Cada fonte implementa uma interface mínima e devolve dados crus; normalização e matcher decidem tudo depois. SQLite guarda estado e histórico. O `launchd` dispara a coleta periodicamente no Mac.

**Tech Stack:** Go 1.26, SQLite via `modernc.org/sqlite` (sem cgo), `goquery` para HTML, `yaml.v3` para configuração, `html/template` para o dashboard, Twilio via HTTP direto (sem SDK).

**Spec:** `docs/superpowers/specs/2026-08-09-harley-hunter-design.md`

## Global Constraints

- Módulo Go: `github.com/andreabreu76/harley-hunter`. Go 1.26.
- Sem cgo. O driver SQLite é `modernc.org/sqlite`, nunca `mattn/go-sqlite3`.
- Código e identificadores em inglês.
- Sem comentários no código. O código deve ser autoexplicativo.
- Commits sem menção a IA, sem `Co-Authored-By`, sem link de sessão.
- Testes não acessam a rede. Fixtures ficam em `testdata/`.
- Toda credencial vem de variável de ambiente. Nada de segredo em arquivo versionado.
- Campos ausentes são representados por ponteiro nulo, nunca por zero.
- Preço sempre em centavos (`int64`). Nunca `float` para dinheiro.

## Estrutura de arquivos

| Arquivo | Responsabilidade |
|---|---|
| `cmd/hunter/main.go` | despacho dos subcomandos `crawl` e `serve` |
| `internal/model/listing.go` | tipos compartilhados `RawListing`, `Listing`, `Verdict` |
| `internal/format/format.go` | separação de milhares, usada pelo SMS e pelo dashboard |
| `internal/config/config.go` | carregamento do `config.yaml` |
| `internal/normalize/price.go` | texto de preço para centavos |
| `internal/normalize/year.go` | texto de ano para inteiro |
| `internal/normalize/km.go` | texto de quilometragem para inteiro |
| `internal/normalize/bike.go` | detecção de modelo e variante |
| `internal/normalize/regions.go` | tabela de cidades e regiões metropolitanas |
| `internal/normalize/location.go` | texto de local para cidade e UF |
| `internal/normalize/normalize.go` | `RawListing` para `Listing`, mais impressão digital |
| `internal/match/match.go` | avaliação dos quatro eixos |
| `internal/store/store.go` | abertura, migração e consultas do SQLite |
| `internal/store/upsert.go` | inserção com deduplicação e histórico de preço |
| `internal/source/source.go` | interface `Source` |
| `internal/source/olx.go` | coletor da OLX |
| `internal/source/mercadolivre.go` | coletor do Mercado Livre |
| `internal/crawl/crawl.go` | orquestração da rodada e registro de saúde |
| `internal/notify/notify.go` | interface `Notifier` |
| `internal/notify/twilio.go` | envio de SMS |
| `internal/web/server.go` | rotas do dashboard |
| `internal/web/templates/*.html` | páginas |
| `config/config.yaml` | critérios, fontes ativas, caminhos |
| `deploy/com.andreabreu.harleyhunter.plist` | agendamento no `launchd` |

`internal/model` existe para que `source`, `normalize`, `match` e `store` compartilhem tipos sem dependência circular.

---

### Task 1: Esqueleto do módulo e configuração

**Files:**
- Create: `go.mod`, `cmd/hunter/main.go`, `internal/config/config.go`, `config/config.yaml`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nada
- Produces: `config.Config` com campos `DatabasePath string`, `Sources []string`, `Match config.MatchCriteria`, `Crawl config.CrawlSettings`; e `config.Load(path string) (Config, error)`

- [ ] **Step 1: Inicializar o módulo**

```bash
cd ~/src/github.com/andreabreu76/harley-hunter
go mod init github.com/andreabreu76/harley-hunter
go get gopkg.in/yaml.v3
```

- [ ] **Step 2: Escrever o teste que falha**

`internal/config/config_test.go`:

```go
package config

import "testing"

func TestLoadReadsCriteria(t *testing.T) {
	cfg, err := Load("testdata/config.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.DatabasePath != "hunter.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "hunter.db")
	}
	if len(cfg.Sources) != 2 || cfg.Sources[0] != "olx" {
		t.Errorf("Sources = %v, want [olx mercadolivre]", cfg.Sources)
	}
	if cfg.Match.MaxPriceCents != 7500000 {
		t.Errorf("MaxPriceCents = %d, want 7500000", cfg.Match.MaxPriceCents)
	}
	if cfg.Match.MaybeMaxPriceCents != 8500000 {
		t.Errorf("MaybeMaxPriceCents = %d, want 8500000", cfg.Match.MaybeMaxPriceCents)
	}
	if len(cfg.Match.Years) != 2 || cfg.Match.Years[0] != 2014 {
		t.Errorf("Years = %v, want [2014 2015]", cfg.Match.Years)
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.yaml"); err == nil {
		t.Fatal("Load should return an error for a missing file")
	}
}

func TestLoadRejectsEmptyMatchCriteria(t *testing.T) {
	if _, err := Load("testdata/no-match.yaml"); err == nil {
		t.Fatal("Load should reject a config with no match criteria")
	}
}
```

`internal/config/testdata/no-match.yaml` — um config sem a seção `match`, que
deve ser recusado:

```yaml
database_path: hunter.db
sources:
  - olx
```

`internal/config/testdata/config.yaml`:

```yaml
database_path: hunter.db
sources:
  - olx
  - mercadolivre
match:
  years: [2014, 2015]
  maybe_years: [2013, 2016]
  max_price_cents: 7500000
  maybe_max_price_cents: 8500000
crawl:
  timeout_seconds: 90
  max_concurrent: 4
  max_sms_per_run: 5
```

Critérios de match vazios são recusados na carga, e não tratados como
permissivos. Um `config.yaml` sem a seção `match` daria `Years: nil` e
`MaxPriceCents: 0`, e nesse estado todo anúncio com ano e preço conhecidos é
rejeitado: a coleta roda inteira, não levanta erro nenhum e não encontra nada.
É o mesmo modo de falha silencioso que o painel de saúde das fontes existe para
combater — o sistema parecendo funcionar enquanto está cego.

- [ ] **Step 3: Rodar o teste e confirmar a falha**

Run: `go test ./internal/config/ -run TestLoad -v`
Expected: FAIL — o pacote não compila, `Load` não existe.

- [ ] **Step 4: Implementar o carregamento**

`internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type MatchCriteria struct {
	Years              []int `yaml:"years"`
	MaybeYears         []int `yaml:"maybe_years"`
	MaxPriceCents      int64 `yaml:"max_price_cents"`
	MaybeMaxPriceCents int64 `yaml:"maybe_max_price_cents"`
}

type CrawlSettings struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`
	MaxConcurrent  int `yaml:"max_concurrent"`
	MaxSMSPerRun   int `yaml:"max_sms_per_run"`
}

type Config struct {
	DatabasePath string        `yaml:"database_path"`
	Sources      []string      `yaml:"sources"`
	Match        MatchCriteria `yaml:"match"`
	Crawl        CrawlSettings `yaml:"crawl"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	if len(cfg.Sources) == 0 {
		return Config{}, fmt.Errorf("config has no sources enabled")
	}
	if len(cfg.Match.Years) == 0 {
		return Config{}, fmt.Errorf("config has no target years")
	}
	if cfg.Match.MaxPriceCents <= 0 {
		return Config{}, fmt.Errorf("config has no max price")
	}
	return cfg, nil
}
```

- [ ] **Step 5: Rodar o teste e confirmar que passa**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 6: Criar o config real e o esqueleto do binário**

`config/config.yaml`:

```yaml
database_path: hunter.db
sources:
  - olx
  - mercadolivre
match:
  years: [2014, 2015]
  maybe_years: [2013, 2016]
  max_price_cents: 7500000
  maybe_max_price_cents: 8500000
crawl:
  timeout_seconds: 90
  max_concurrent: 4
  max_sms_per_run: 5
```

`cmd/hunter/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/andreabreu76/harley-hunter/internal/config"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch flag.Arg(0) {
	case "crawl":
		fmt.Printf("crawl: %d sources enabled\n", len(cfg.Sources))
	case "serve":
		fmt.Println("serve: not implemented yet")
	default:
		fmt.Fprintln(os.Stderr, "usage: hunter [-config path] <crawl|serve>")
		os.Exit(2)
	}
}
```

- [ ] **Step 7: Verificar que o binário roda**

Run: `go run ./cmd/hunter crawl`
Expected: `crawl: 2 sources enabled`

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum cmd internal/config config/config.yaml
git commit -m "feat: module scaffold and yaml configuration"
```

---

### Task 2: Tipos compartilhados

**Files:**
- Create: `internal/model/listing.go`
- Test: `internal/model/listing_test.go`

**Interfaces:**
- Consumes: nada
- Produces: `model.Verdict` (`VerdictMatch`, `VerdictMaybe`, `VerdictReject`), `model.RawListing`, `model.Listing`, e `model.CombineVerdicts(axes map[string]Verdict) Verdict`

- [ ] **Step 1: Escrever o teste que falha**

`internal/model/listing_test.go`:

```go
package model

import "testing"

func TestCombineVerdicts(t *testing.T) {
	cases := []struct {
		name string
		axes map[string]Verdict
		want Verdict
	}{
		{"all match", map[string]Verdict{"model": VerdictMatch, "year": VerdictMatch}, VerdictMatch},
		{"one maybe", map[string]Verdict{"model": VerdictMatch, "year": VerdictMaybe}, VerdictMaybe},
		{"reject wins over maybe", map[string]Verdict{"model": VerdictReject, "year": VerdictMaybe}, VerdictReject},
		{"empty is reject", map[string]Verdict{}, VerdictReject},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CombineVerdicts(c.axes); got != c.want {
				t.Errorf("CombineVerdicts(%v) = %q, want %q", c.axes, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar a falha**

Run: `go test ./internal/model/ -v`
Expected: FAIL — pacote não compila.

- [ ] **Step 3: Implementar os tipos**

`internal/model/listing.go`:

```go
package model

type Verdict string

const (
	VerdictMatch  Verdict = "match"
	VerdictMaybe  Verdict = "maybe"
	VerdictReject Verdict = "reject"
)

const (
	AxisModel    = "model"
	AxisYear     = "year"
	AxisPrice    = "price"
	AxisLocation = "location"
)

const (
	BikeStreetGlide   = "street_glide"
	BikeRoadGlide     = "road_glide"
	BikeElectraGlide  = "electra_glide"
	BikeUltra         = "ultra"
	BikeTouringUnknown = "touring_unknown"
	BikeOther         = "other"
)

const (
	VariantBase    = "base"
	VariantSpecial = "special"
	VariantCVO     = "cvo"
	VariantUnknown = "unknown"
)

type RawListing struct {
	Source     string
	ExternalID string
	URL        string
	Title      string
	RawText    string
	PriceText  string
	YearText   string
	KmText     string
	LocationText string
	ImageURL   string
}

type Listing struct {
	Source        string
	ExternalID    string
	URL           string
	Title         string
	RawText       string
	Bike          string
	Variant       string
	Year          *int
	PriceCents    *int64
	Km            *int
	City          string
	State         string
	ImageURL      string
	Verdict       Verdict
	VerdictReason map[string]Verdict
	Fingerprint   string
}

func CombineVerdicts(axes map[string]Verdict) Verdict {
	if len(axes) == 0 {
		return VerdictReject
	}
	result := VerdictMatch
	for _, v := range axes {
		if v == VerdictReject {
			return VerdictReject
		}
		if v == VerdictMaybe {
			result = VerdictMaybe
		}
	}
	return result
}
```

- [ ] **Step 4: Rodar o teste e confirmar que passa**

Run: `go test ./internal/model/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model
git commit -m "feat: shared listing types and verdict combination"
```

---

### Task 3: Normalização de preço, ano e quilometragem

**Files:**
- Create: `internal/normalize/price.go`, `internal/normalize/year.go`, `internal/normalize/km.go`
- Test: `internal/normalize/price_test.go`, `internal/normalize/year_test.go`, `internal/normalize/km_test.go`

**Interfaces:**
- Consumes: nada
- Produces: `normalize.ParsePrice(s string) (int64, bool)`, `normalize.ParseYear(s string) (int, bool)`, `normalize.ParseKm(s string) (int, bool)`. O segundo retorno é `false` quando o valor está ausente ou é implausível.

- [ ] **Step 1: Escrever os testes que falham**

`internal/normalize/price_test.go`:

```go
package normalize

import "testing"

func TestParsePrice(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"R$ 74.900", 7490000, true},
		{"R$ 74.900,00", 7490000, true},
		{"74900", 7490000, true},
		{"74,9 mil", 7490000, true},
		{"75 mil", 7500000, true},
		{"R$ 68.500,50", 6850050, true},
		{"R$ 74.900 negociável", 7490000, true},
		{"R$ 74,9 mil", 7490000, true},
		{"Vendo Street Glide 15/15, 42.000 km, R$ 74.900", 7490000, true},
		{"a combinar", 0, false},
		{"Consulte", 0, false},
		{"", 0, false},
		{"R$ 1,00", 0, false},
		{"12.000 km", 0, false},
		{"12 mil km", 0, false},
		{"42 mil km, valor 74 mil", 7400000, true},
		{"1 mil curtidas, moto por 74 mil", 7400000, true},
		{"20 mil seguidores no insta, vendo por 74 mil", 7400000, true},
		{"3 mil curtidas no post, R$ 74.900", 7490000, true},
		{"10 mil likes! Road Glide R$ 72.000", 7200000, true},
		{"20 mil comentários, moto por 74 mil", 7400000, true},
		{"Entrada de R$ 20.000, moto R$ 74.900", 7490000, true},
		{"R$ 74.900, aceito entrada de R$ 20.000", 7490000, true},
		{"Parcelas de R$ 1.800, valor total R$ 74.900", 7490000, true},
		{"Moto R$ 74.900, troco por ate R$ 90.000", 7490000, true},
		{"Street Glide R$ 74.900, aceito troca ate R$ 60.000", 7490000, true},
		{"Road Glide R$ 72.000, avalio moto ate R$ 95.000", 7200000, true},
		{"Aceito troca, R$ 74.900", 7490000, true},
		{"Vendo ou troco, R$ 74.900", 7490000, true},
		{"Moto avaliada em R$ 74.900", 7490000, true},
		{"Street Glide 2015 até 2016, R$ 74.900", 7490000, true},
		{"Aceito troca ate R$ 60.000, moto R$ 74.900", 7490000, true},
		{"Entrada de R$ 20.000, valor total 74 mil", 7400000, true},
		{"Entrada 20 mil, moto 74 mil", 7400000, true},
		{"Sinal de R$ 15.000, restante 74 mil", 7400000, true},
		{"Entrada 20 mil, Street Glide R$ 74.900", 7490000, true},
		{"R$ 74.900, troco por ate 90 mil", 7490000, true},
		{"Aceito troca ate 90 mil, moto R$ 74.900", 7490000, true},
		{"Moto 74 mil, troco por ate 90 mil", 7400000, true},
		{"12 mil kms", 0, false},
		{"42 mil kms rodados", 0, false},
		{"12 mil quilometros", 0, false},
		{"Vendo Road Glide, 42 mil kms rodados, aceito troca", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParsePrice(c.in)
			if ok != c.ok {
				t.Fatalf("ParsePrice(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("ParsePrice(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
```

Os casos com `km` são o coração deste teste. `ParsePrice` recebe tanto o campo estruturado de preço quanto, como alternativa, o texto livre inteiro do anúncio — que quase sempre menciona quilometragem. Um filtro que simplesmente recusasse qualquer texto contendo "km" quebraria o caso `"Vendo Street Glide 15/15, 42.000 km, R$ 74.900"`, e um filtro ausente transformaria `"12 mil km"` em R$ 12.000. A implementação resolve isso pela ordem de reconhecimento, não por exclusão.

O caso `"negociável"` também é deliberado: é a palavra mais comum em anúncio de moto e não pode ser confundida com preço indisponível.

`ParsePrice` reúne TODOS os candidatos num conjunto único e escolhe o maior
plausível, em vez de percorrer ramos independentes e devolver no primeiro que
sobreviver. Essa é a propriedade central: um decoy plausível não pode vencer só
por aparecer antes.

Ramos independentes com retorno antecipado falham em qualquer notação mista.
`"Entrada de R$ 20.000, valor total 74 mil"` marca a entrada com `R$` e o preço
real com "mil"; um ramo `R$` que retorne assim que acha algo devolve a entrada e
nunca chega no preço. `"Entrada 20 mil, moto 74 mil"` é o espelho, sem nenhum
`R$` no texto, e um ramo de milhares que pare no primeiro sobrevivente devolve
20 mil. Ambos fabricam preço baixo, que passa sob o teto e vira alerta falso.

Os dois coletores aplicam os MESMOS filtros, e a simetria importa. O teto de
troca aparece nas duas notações — `"troco por até R$ 90.000"` e
`"troco por até 90 mil"` — e o Instagram prefere a segunda. Guardar só o
coletor de `R$` deixava o teto em milhares entrar no conjunto e, por ser o maior
valor, vencer a comparação. Cada coletor descarta tanto o que vem depois de um
`até` colado quanto, no caso dos milhares, o que é seguido de palavra
não-monetária. O número puro só é considerado quando nada mais foi
encontrado, porque só faz sentido quando a string inteira é o campo de preço.

Valores precedidos IMEDIATAMENTE por `até` são descartados antes da comparação.
`"Moto R$ 74.900, troco por até R$ 90.000"` cita um teto de avaliação da moto do
comprador, não o preço da que está à venda — e R$ 90.000 não é implausível, é
aceito como preço e depois reprova no matcher, fazendo uma moto dentro do alvo
desaparecer.

O discriminador é o marcador de teto colado ao número, não a vizinhança da
palavra "troca". Procurar `troc`, `permut` ou `avali` numa janela larga destrói
preço legítimo, porque essas palavras descrevem o anúncio e não o valor:
`"Aceito troca, R$ 74.900"`, `"Vendo ou troco, R$ 74.900"` e
`"Moto avaliada em R$ 74.900"` perderiam o preço inteiro. Pior, a janela larga
também apagaria os dois valores de
`"Aceito troca até R$ 60.000, moto R$ 74.900"`, trocando um preço errado por
nenhum preço. `"Street Glide 2015 até 2016, R$ 74.900"` mostra que nem todo
`até` governa o número seguinte — por isso a checagem exige o marcador colado.

Entre vários valores marcados com `R$`, vence o MAIOR plausível. Anúncio de moto
financiada cita entrada e parcela ao lado do preço — `"Entrada de R$ 20.000,
moto R$ 74.900"` e `"Parcelas de R$ 1.800, valor total R$ 74.900"` — e a entrada
é sempre menor que o valor da moto. Pegar a primeira ocorrência devolveria a
entrada, um preço baixo e falso que passaria por Match. A regra falha só no
formato promocional `"De R$ 82.000 por R$ 74.900"`, onde devolve o preço antigo;
o dano ali é contido, porque o valor mais alto tende a estourar o teto e o
anúncio cai em Talvez em vez de virar alerta falso.

Os prefixos da lista precisam corresponder ao que o grupo `([a-z]*)` de fato
captura, e ele para no acento. `"comentários"` é capturado como `coment`, então
o prefixo listado tem de ser `coment` e não `comentari` — a forma mais longa
nunca casaria. Pelo mesmo motivo `visualiza`, `avalia` e `quil` funcionam:
todos param antes do acento da palavra real.

O filtro de palavras não-monetárias vai além de quilometragem e cobre termos de
engajamento — curtidas, seguidores, visualizações, likes. O caso que motiva isso
é o mais perigoso do parser inteiro: `"20 mil seguidores no insta, vendo por
74 mil"` devolvia R$ 20.000, um preço FABRICADO a partir da contagem de
seguidores. Vinte mil reais passa no teste de plausibilidade e fica abaixo do
teto, então o anúncio viraria Match e dispararia SMS por uma moto cujo preço
real é outro. Como o Instagram é fonte-alvo e legenda de loja cita engajamento o
tempo todo, o risco é corriqueiro, não hipotético.

A varredura percorre TODAS as ocorrências de milhares em vez de olhar só a
primeira. Anúncio real escreve `"42 mil km, valor 74 mil"`, com a quilometragem
antes do preço; parar na primeira ocorrência descartaria o ramo inteiro e o
preço se perderia.

O guard de quilometragem reconhece prefixo, não token exato. Anúncio real escreve `"42 mil kms rodados"` e `"12 mil quilômetros"` tanto quanto `"12 mil km"`, e comparar com a string `"km"` deixaria os dois primeiros virarem preço. O prefixo `quil` cobre a forma acentuada porque o grupo `[a-z]*` do regex para no `ô`. Um anúncio sem preço cujo texto diz `"42 mil kms rodados"` viraria R$ 42.000 — dentro do teto, classificado como Match e disparando SMS por uma moto que sequer anunciou preço.

`internal/normalize/year_test.go`:

```go
package normalize

import "testing"

func TestParseYear(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"2015", 2015, true},
		{"2014/2015", 2015, true},
		{"15/15", 2015, true},
		{"14/15", 2015, true},
		{"HD STREET GLIDE ESPECIAL 15/15 IMPECAVEL", 2015, true},
		{"Harley Street Glide 2014", 2014, true},
		{"mod. 2015", 2015, true},
		{"", 0, false},
		{"Harley 1690 Rushmore", 0, false},
		{"3000", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParseYear(c.in)
			if ok != c.ok {
				t.Fatalf("ParseYear(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("ParseYear(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
```

O caso `"Harley 1690 Rushmore"` protege contra cilindrada virar ano.

`internal/normalize/km_test.go`:

```go
package normalize

import "testing"

func TestParseKm(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"12.000 km", 12000, true},
		{"12000km", 12000, true},
		{"12 mil km", 12000, true},
		{"45.320 KM", 45320, true},
		{"", 0, false},
		{"R$ 74.900", 0, false},
		{"900.000 km", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParseKm(c.in)
			if ok != c.ok {
				t.Fatalf("ParseKm(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("ParseKm(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Rodar os testes e confirmar a falha**

Run: `go test ./internal/normalize/ -v`
Expected: FAIL — funções não existem.

- [ ] **Step 3: Implementar `ParsePrice`**

`internal/normalize/price.go`:

```go
package normalize

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	minPlausiblePriceCents = 500000
	maxPlausiblePriceCents = 50000000
)

var (
	unavailablePrice = regexp.MustCompile(`(?i)combinar|consulte|sob\s+consulta`)
	thousandsSuffix  = regexp.MustCompile(`(?i)([\d.,]+)\s*mil\s*([a-z]*)`)
	priceWithSymbol  = regexp.MustCompile(`(?i)r\$\s*([\d.,]+)`)
	bareNumber       = regexp.MustCompile(`^\s*([\d.,]+)\s*$`)
)

func ParsePrice(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || unavailablePrice.MatchString(s) {
		return 0, false
	}

	best := int64(0)
	consider := func(cents int64) {
		if value, ok := plausible(cents); ok && value > best {
			best = value
		}
	}

	for _, loc := range priceWithSymbol.FindAllStringSubmatchIndex(s, -1) {
		if precededByCeilingMarker(s, loc[0]) {
			continue
		}
		if value, err := strconv.ParseFloat(decimalize(s[loc[2]:loc[3]]), 64); err == nil {
			consider(int64(value*100 + 0.5))
		}
	}

	for _, loc := range thousandsSuffix.FindAllStringSubmatchIndex(s, -1) {
		if precededByCeilingMarker(s, loc[0]) || isNonPriceWord(s[loc[4]:loc[5]]) {
			continue
		}
		if value, err := strconv.ParseFloat(decimalize(s[loc[2]:loc[3]]), 64); err == nil {
			consider(int64(value*1000*100 + 0.5))
		}
	}

	if best == 0 {
		if m := bareNumber.FindStringSubmatch(s); m != nil {
			if value, err := strconv.ParseFloat(decimalize(m[1]), 64); err == nil {
				consider(int64(value*100 + 0.5))
			}
		}
	}

	return best, best > 0
}

var nonPricePrefixes = []string{
	"km", "quil", "curtid", "seguidor", "visualiza", "like", "view",
	"inscrit", "coment", "compartilh", "avalia",
}

func isNonPriceWord(s string) bool {
	s = strings.ToLower(s)
	for _, prefix := range nonPricePrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

var ceilingMarkers = []string{"ate", "até"}

func precededByCeilingMarker(s string, at int) bool {
	start := at - 8
	if start < 0 {
		start = 0
	}
	window := strings.TrimRight(strings.ToLower(s[start:at]), " ")
	for _, marker := range ceilingMarkers {
		if strings.HasSuffix(window, marker) {
			return true
		}
	}
	return false
}

func decimalize(s string) string {
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		return strings.ReplaceAll(s, ",", ".")
	}
	if strings.Count(s, ".") >= 1 {
		parts := strings.Split(s, ".")
		last := parts[len(parts)-1]
		if len(last) == 3 {
			return strings.ReplaceAll(s, ".", "")
		}
	}
	return s
}

func plausible(cents int64) (int64, bool) {
	if cents < minPlausiblePriceCents || cents > maxPlausiblePriceCents {
		return 0, false
	}
	return cents, true
}
```

- [ ] **Step 4: Implementar `ParseYear`**

`internal/normalize/year.go`:

```go
package normalize

import (
	"regexp"
	"strconv"
	"time"
)

var (
	fourDigitYear = regexp.MustCompile(`\b(19[5-9]\d|20[0-4]\d)\b`)
	twoDigitPair  = regexp.MustCompile(`\b(\d{2})\s*/\s*(\d{2})\b`)
	fourDigitPair = regexp.MustCompile(`\b(19\d{2}|20\d{2})\s*/\s*(19\d{2}|20\d{2})\b`)
)

func ParseYear(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	maxYear := time.Now().Year() + 1

	if m := fourDigitPair.FindStringSubmatch(s); m != nil {
		year, _ := strconv.Atoi(m[2])
		return validYear(year, maxYear)
	}
	if m := twoDigitPair.FindStringSubmatch(s); m != nil {
		year, _ := strconv.Atoi(m[2])
		return validYear(2000+year, maxYear)
	}
	if m := fourDigitYear.FindString(s); m != "" {
		year, _ := strconv.Atoi(m)
		return validYear(year, maxYear)
	}
	return 0, false
}

func validYear(year, maxYear int) (int, bool) {
	if year < 1950 || year > maxYear {
		return 0, false
	}
	return year, true
}
```

- [ ] **Step 5: Implementar `ParseKm`**

`internal/normalize/km.go`:

```go
package normalize

import (
	"regexp"
	"strconv"
	"strings"
)

const maxPlausibleKm = 400000

var (
	kmThousands = regexp.MustCompile(`(?i)([\d.,]+)\s*mil\s*km`)
	kmPlain     = regexp.MustCompile(`(?i)([\d.,]+)\s*km`)
)

func ParseKm(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if m := kmThousands.FindStringSubmatch(s); m != nil {
		value, err := strconv.ParseFloat(decimalize(m[1]), 64)
		if err != nil {
			return 0, false
		}
		return plausibleKm(int(value * 1000))
	}
	if m := kmPlain.FindStringSubmatch(s); m != nil {
		digits := strings.NewReplacer(".", "", ",", "").Replace(m[1])
		value, err := strconv.Atoi(digits)
		if err != nil {
			return 0, false
		}
		return plausibleKm(value)
	}
	return 0, false
}

func plausibleKm(km int) (int, bool) {
	if km <= 0 || km > maxPlausibleKm {
		return 0, false
	}
	return km, true
}
```

- [ ] **Step 6: Rodar os testes e confirmar que passam**

Run: `go test ./internal/normalize/ -v`
Expected: PASS em todos os casos.

- [ ] **Step 7: Commit**

```bash
git add internal/normalize
git commit -m "feat: parse price, year and mileage from listing text"
```

---

### Task 4: Detecção de modelo e variante

**Files:**
- Create: `internal/normalize/bike.go`
- Test: `internal/normalize/bike_test.go`

**Interfaces:**
- Consumes: constantes `model.BikeStreetGlide`, `model.BikeRoadGlide`, `model.BikeElectraGlide`, `model.BikeUltra`, `model.BikeTouringUnknown`, `model.BikeOther`, `model.VariantBase`, `model.VariantSpecial`, `model.VariantCVO`, `model.VariantUnknown` da Task 2
- Produces: `normalize.DetectBike(text string) (bike string, variant string)` e `normalize.Fold(s string) string` (minúsculas sem acento, reaproveitado pela Task 5)

- [ ] **Step 1: Escrever o teste que falha**

`internal/normalize/bike_test.go`:

```go
package normalize

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestDetectBike(t *testing.T) {
	cases := []struct {
		in          string
		wantBike    string
		wantVariant string
	}{
		{"Harley Davidson Street Glide 2015", model.BikeStreetGlide, model.VariantBase},
		{"HD STREET GLIDE ESPECIAL 15/15 IMPECAVEL", model.BikeStreetGlide, model.VariantSpecial},
		{"Street Glide Special 2014", model.BikeStreetGlide, model.VariantSpecial},
		{"streetglide 2015", model.BikeStreetGlide, model.VariantBase},
		{"Harley FLHXS 2015", model.BikeStreetGlide, model.VariantSpecial},
		{"CVO Street Glide 2015", model.BikeStreetGlide, model.VariantCVO},
		{"Road Glide Special 2015 - valor a combinar", model.BikeRoadGlide, model.VariantSpecial},
		{"roadglide 2015", model.BikeRoadGlide, model.VariantBase},
		{"Harley FLTRXSE 2015", model.BikeRoadGlide, model.VariantCVO},
		{"Electra Glide Ultra Limited 2015", model.BikeElectraGlide, model.VariantUnknown},
		{"Harley Davidson Ultra Limited 2014", model.BikeUltra, model.VariantUnknown},
		{"Harley Davidson Touring 1690 2015", model.BikeTouringUnknown, model.VariantUnknown},
		{"Honda Gold Wing 2015", model.BikeOther, model.VariantUnknown},
		{"Harley Davidson Iron 883", model.BikeOther, model.VariantUnknown},
		{"harley-davidson street-glide 2015", model.BikeStreetGlide, model.VariantBase},
		{"Harley-Davidson Road-Glide Special 2015", model.BikeRoadGlide, model.VariantSpecial},
		{"H-D Street Glide 2014", model.BikeStreetGlide, model.VariantBase},
		{"Harley-Davidson Electra-Glide 2015", model.BikeElectraGlide, model.VariantUnknown},
		{"Harley FLHX Street Glide 2015", model.BikeStreetGlide, model.VariantBase},
		{"Harley FLTRX Road Glide 2015", model.BikeRoadGlide, model.VariantBase},
		{"Harley FLTRX-SE 2015", model.BikeRoadGlide, model.VariantCVO},
		{"Harley FLTRX SE 2015", model.BikeRoadGlide, model.VariantCVO},
		{"Harley FLHXSE 2015", model.BikeStreetGlide, model.VariantCVO},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			bike, variant := DetectBike(c.in)
			if bike != c.wantBike {
				t.Errorf("DetectBike(%q) bike = %q, want %q", c.in, bike, c.wantBike)
			}
			if variant != c.wantVariant {
				t.Errorf("DetectBike(%q) variant = %q, want %q", c.in, variant, c.wantVariant)
			}
		})
	}
}

func TestFoldRemovesAccents(t *testing.T) {
	if got := Fold("SÃO JOSÉ DOS PINHAIS"); got != "sao jose dos pinhais" {
		t.Errorf("Fold = %q, want %q", got, "sao jose dos pinhais")
	}
}
```

O caso `"Electra Glide Ultra Limited 2015"` é o que impede a armadilha central: três modelos contêm "glide" e apenas Street e Road são alvo.

- [ ] **Step 2: Rodar o teste e confirmar a falha**

Run: `go test ./internal/normalize/ -run TestDetectBike -v`
Expected: FAIL — `DetectBike` não existe.

- [ ] **Step 3: Instalar a dependência de normalização Unicode**

```bash
go get golang.org/x/text/unicode/norm
```

- [ ] **Step 4: Implementar a detecção**

`internal/normalize/bike.go`:

```go
package normalize

import (
	"strings"
	"unicode"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"golang.org/x/text/unicode/norm"
)

var compactor = strings.NewReplacer(" ", "", "-", "", ".", "", "/", "")

func Fold(s string) string {
	decomposed := norm.NFD.String(strings.ToLower(s))
	var b strings.Builder
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func DetectBike(text string) (string, string) {
	t := Fold(text)
	compact := compactor.Replace(t)

	switch {
	case containsAny(t, compact, "electra glide", "electraglide", "flht"):
		return model.BikeElectraGlide, model.VariantUnknown
	case containsAny(t, compact, "road glide", "roadglide", "fltrx"):
		return model.BikeRoadGlide, detectVariant(t, compact, "fltrxse", "fltrxs")
	case containsAny(t, compact, "street glide", "streetglide", "stglide", "flhx"):
		return model.BikeStreetGlide, detectVariant(t, compact, "flhxse", "flhxs")
	case containsAny(t, compact, "ultra limited", "ultraclassic", "ultra classic"):
		return model.BikeUltra, model.VariantUnknown
	case isHarley(t) && containsAny(t, compact, "touring", "1690", "1745", "rushmore"):
		return model.BikeTouringUnknown, model.VariantUnknown
	default:
		return model.BikeOther, model.VariantUnknown
	}
}

func detectVariant(t, compact, cvoCode, specialCode string) string {
	if strings.Contains(t, "cvo") || containsCode(compact, cvoCode) {
		return model.VariantCVO
	}
	if containsAny(t, compact, "special", "especial") || containsCode(compact, specialCode) {
		return model.VariantSpecial
	}
	return model.VariantBase
}

func containsCode(compact, code string) bool {
	for from := 0; from <= len(compact)-len(code); {
		offset := strings.Index(compact[from:], code)
		if offset < 0 {
			return false
		}
		end := from + offset + len(code)
		if end >= len(compact) || compact[end] < 'a' || compact[end] > 'z' {
			return true
		}
		from = from + offset + 1
	}
	return false
}

func isHarley(t string) bool {
	return strings.Contains(t, "harley") || strings.Contains(t, "hd ")
}

func containsAny(t, compact string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(t, n) || strings.Contains(compact, strings.ReplaceAll(n, " ", "")) {
			return true
		}
	}
	return false
}
```

Os códigos de variante são procurados na forma compacta, mas exigindo que o
código não seja seguido de letra. `"FLHX Street"` vira `"flhxstreet"`, que
contém `flhxs` — o código da versão Special — e sem a checagem uma FLHX base
seria gravada como Special. Procurar só no texto com espaços resolveria isso e
abriria o buraco simétrico: `"FLTRX-SE"` e `"FLTRX SE"` deixariam de casar
`fltrxse`, e este corpus hifeniza livremente em torno dos nomes. A fronteira é
só à direita porque, na forma compacta, tudo vira uma palavra só e o caractere
anterior é quase sempre uma letra do fabricante.

A ordem do `switch` é a regra de correção: Electra Glide é testada antes de Road e Street porque o texto pode conter mais de um termo, e a variante mais específica precisa ganhar. O código `flhxse` é testado antes de `flhxs` pelo mesmo motivo.

O `compactor` remove pontuação além de espaços porque anúncio brasileiro escreve `"Harley-Davidson Street-Glide"` tanto quanto a forma com espaços. Removendo só espaços, `"street-glide"` não casa nem `"street glide"` nem `"streetglide"`, e uma Street Glide dentro do alvo é classificada como `other` e descartada em silêncio. A correção fica no `compactor`, não em `Fold`, justamente para não alterar a normalização de nomes de cidade que a Task 5 faz com `Fold`.

O código `flhtk` não aparece no ramo Ultra porque `flht`, no ramo Electra Glide, já o captura — e a classificação resultante está correta, já que a FLHTK é uma Electra Glide Ultra Limited. Incluí-lo ali seria código inalcançável.

- [ ] **Step 5: Rodar os testes e confirmar que passam**

Run: `go test ./internal/normalize/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/normalize/bike.go internal/normalize/bike_test.go go.mod go.sum
git commit -m "feat: detect bike model and variant from listing text"
```

---

### Task 5: Localização e regiões metropolitanas

**Files:**
- Create: `internal/normalize/regions.go`, `internal/normalize/location.go`
- Test: `internal/normalize/location_test.go`

**Interfaces:**
- Consumes: `normalize.Fold` da Task 4
- Produces: `normalize.ParseLocation(s string) (city string, state string)` e `normalize.LocationTier(city, state string) string`, cujo retorno é `"metro"`, `"state"` ou `"outside"`

- [ ] **Step 1: Escrever o teste que falha**

`internal/normalize/location_test.go`:

```go
package normalize

import "testing"

func TestParseLocation(t *testing.T) {
	cases := []struct {
		in        string
		wantCity  string
		wantState string
	}{
		{"Curitiba - PR", "curitiba", "PR"},
		{"São Paulo, SP", "sao paulo", "SP"},
		{"Rio de Janeiro / RJ", "rio de janeiro", "RJ"},
		{"São José dos Pinhais - PR", "sao jose dos pinhais", "PR"},
		{"Niterói", "niteroi", ""},
		{"", "", ""},
		{"Rio de Janeiro - RJ - Brasil", "rio de janeiro", "RJ"},
		{"Guarulhos - SP (Cumbica)", "guarulhos", "SP"},
		{"São Paulo (SP)", "sao paulo", "SP"},
		{"Curitiba - Paraná", "curitiba", "PR"},
		{"Copacabana, Rio de Janeiro - RJ", "rio de janeiro", "RJ"},
		{"Embu-Guaçu - SP", "embu guacu", "SP"},
		{"Embu Guaçu - SP", "embu guacu", "SP"},
		{"Lapa, São Paulo - SP", "sao paulo", "SP"},
		{"Curitiba- PR", "curitiba", "PR"},
		{"Curitiba-PR", "curitiba", "PR"},
		{"Niterói-RJ", "niteroi", "RJ"},
		{"Mogi das Cruzes-SP", "mogi das cruzes", "SP"},
		{"Curitiba PR", "curitiba", "PR"},
		{"Sao Jose dos Pinhais PR", "sao jose dos pinhais", "PR"},
		{"Campinas - São Paulo", "campinas", "SP"},
		{"Volta Redonda - Rio de Janeiro", "volta redonda", "RJ"},
		{"Cabo Frio, Rio de Janeiro", "cabo frio", "RJ"},
		{"Rio de Janeiro", "rio de janeiro", "RJ"},
		{"São Paulo", "sao paulo", "SP"},
		{"Rio de Janeiro, Copacabana", "rio de janeiro", ""},
		{"São Paulo, Moema", "sao paulo", ""},
		{"Vila Mariana, São Paulo", "vila mariana", "SP"},
		{"São Paulo Zona Sul", "sao paulo", "SP"},
		{"Rio de Janeiro Zona Oeste", "rio de janeiro", "RJ"},
		{"Curitiba Centro", "curitiba", "PR"},
		{"Campinas - São Paulo - Brasil", "campinas", "SP"},
		{"Volta Redonda - Rio de Janeiro - Brasil", "volta redonda", "RJ"},
		{"Santos, São Paulo (Zona Leste)", "santos", "SP"},
		{"Vila Isabel, Volta Redonda - RJ", "volta redonda", "RJ"},
		{"Centro, Campinas - SP", "campinas", "SP"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			city, state := ParseLocation(c.in)
			if city != c.wantCity || state != c.wantState {
				t.Errorf("ParseLocation(%q) = (%q, %q), want (%q, %q)", c.in, city, state, c.wantCity, c.wantState)
			}
		})
	}
}

func TestLocationTier(t *testing.T) {
	cases := []struct {
		city  string
		state string
		want  string
	}{
		{"sao paulo", "SP", "metro"},
		{"guarulhos", "SP", "metro"},
		{"niteroi", "RJ", "metro"},
		{"sao jose dos pinhais", "PR", "metro"},
		{"campinas", "SP", "state"},
		{"londrina", "PR", "state"},
		{"belo horizonte", "MG", "outside"},
		{"", "", "outside"},
		{"niteroi", "", "metro"},
		{"embu guacu", "SP", "metro"},
		{"lapa", "SP", "state"},
		{"campinas", "SP", "state"},
		{"volta redonda", "RJ", "state"},
	}
	for _, c := range cases {
		t.Run(c.city+"/"+c.state, func(t *testing.T) {
			if got := LocationTier(c.city, c.state); got != c.want {
				t.Errorf("LocationTier(%q, %q) = %q, want %q", c.city, c.state, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar a falha**

Run: `go test ./internal/normalize/ -run TestLocation -v`
Expected: FAIL — funções não existem.

- [ ] **Step 3: Implementar a tabela de regiões**

`internal/normalize/regions.go`:

```go
package normalize

var metroCities = map[string]string{
	"rio de janeiro":         "RJ",
	"niteroi":                "RJ",
	"sao goncalo":            "RJ",
	"duque de caxias":        "RJ",
	"nova iguacu":            "RJ",
	"nilopolis":              "RJ",
	"sao joao de meriti":     "RJ",
	"belford roxo":           "RJ",
	"mage":                   "RJ",
	"itaborai":               "RJ",
	"marica":                 "RJ",
	"queimados":              "RJ",
	"mesquita":               "RJ",
	"japeri":                 "RJ",
	"seropedica":             "RJ",
	"itaguai":                "RJ",
	"guapimirim":             "RJ",
	"tangua":                 "RJ",
	"paracambi":              "RJ",
	"rio bonito":             "RJ",
	"cachoeiras de macacu":   "RJ",
	"petropolis":             "RJ",
	"sao paulo":              "SP",
	"guarulhos":              "SP",
	"osasco":                 "SP",
	"santo andre":            "SP",
	"sao bernardo do campo":  "SP",
	"sao caetano do sul":     "SP",
	"diadema":                "SP",
	"maua":                   "SP",
	"ribeirao pires":         "SP",
	"barueri":                "SP",
	"carapicuiba":            "SP",
	"cotia":                  "SP",
	"taboao da serra":        "SP",
	"embu das artes":         "SP",
	"itapevi":                "SP",
	"jandira":                "SP",
	"santana de parnaiba":    "SP",
	"mogi das cruzes":        "SP",
	"suzano":                 "SP",
	"itaquaquecetuba":        "SP",
	"ferraz de vasconcelos":  "SP",
	"poa":                    "SP",
	"aruja":                  "SP",
	"caieiras":               "SP",
	"cajamar":                "SP",
	"franco da rocha":        "SP",
	"francisco morato":       "SP",
	"mairipora":              "SP",
	"itapecerica da serra":   "SP",
	"embu guacu":            "SP",
	"curitiba":               "PR",
	"sao jose dos pinhais":   "PR",
	"pinhais":                "PR",
	"colombo":                "PR",
	"araucaria":              "PR",
	"campo largo":            "PR",
	"almirante tamandare":    "PR",
	"piraquara":              "PR",
	"fazenda rio grande":     "PR",
	"campina grande do sul":  "PR",
	"quatro barras":          "PR",
	"rio branco do sul":      "PR",
	"mandirituba":            "PR",
	"contenda":               "PR",
	"balsa nova":             "PR",
	"lapa":                   "PR",
	"campo magro":            "PR",
	"itaperucu":              "PR",
}

var targetStates = map[string]bool{"RJ": true, "SP": true, "PR": true}

var stateNames = map[string]string{
	"acre": "AC", "alagoas": "AL", "amapa": "AP", "amazonas": "AM",
	"bahia": "BA", "ceara": "CE", "distrito federal": "DF",
	"espirito santo": "ES", "goias": "GO", "maranhao": "MA",
	"mato grosso": "MT", "mato grosso do sul": "MS", "minas gerais": "MG",
	"para": "PA", "paraiba": "PB", "parana": "PR", "pernambuco": "PE",
	"piaui": "PI", "rio de janeiro": "RJ", "rio grande do norte": "RN",
	"rio grande do sul": "RS", "rondonia": "RO", "roraima": "RR",
	"santa catarina": "SC", "sao paulo": "SP", "sergipe": "SE",
	"tocantins": "TO",
}
```

- [ ] **Step 4: Implementar a análise de local**

`internal/normalize/location.go`:

```go
package normalize

import (
	"regexp"
	"strings"
)

var (
	segmentSplit  = regexp.MustCompile(`[,/()]+|\s+-\s*|\s*-\s+`)
	trailingState = regexp.MustCompile(`(?i)[\s\-]([a-z]{2})\s*$`)
)

func ParseLocation(s string) (string, string) {
	if strings.TrimSpace(s) == "" {
		return "", ""
	}

	folded := Fold(s)
	if m := trailingState.FindStringSubmatch(folded); m != nil && isBrazilianState(strings.ToUpper(m[1])) {
		folded = folded[:len(folded)-len(m[0])] + ", " + m[1]
	}

	segments := splitSegments(folded)
	if len(segments) == 0 {
		return "", ""
	}

	state, stateIndex := findState(segments)

	fallback := ""
	for i, seg := range segments {
		if i == stateIndex {
			continue
		}
		metroState, ok := metroCities[cityKey(seg)]
		if !ok {
			continue
		}
		if state != "" && metroState == state {
			return cityKey(seg), state
		}
		if fallback == "" {
			fallback = cityKey(seg)
		}
	}
	if fallback != "" {
		return fallback, state
	}

	for i, seg := range segments {
		if i == stateIndex {
			continue
		}
		if city, embedded := locationFromText(seg); city != "" {
			if state == "" {
				state = embedded
			}
			return city, state
		}
	}

	start := len(segments) - 1
	if stateIndex >= 0 {
		start = stateIndex - 1
	}
	if start >= 0 {
		return segments[start], state
	}
	if stateIndex >= 0 {
		return cityKey(segments[stateIndex]), state
	}
	return "", state
}

func splitSegments(folded string) []string {
	var out []string
	for _, seg := range segmentSplit.Split(folded, -1) {
		if seg = strings.Trim(seg, " -"); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func findState(segments []string) (string, int) {
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if len(seg) == 2 && isBrazilianState(strings.ToUpper(seg)) {
			return strings.ToUpper(seg), i
		}
		if i > 0 || len(segments) == 1 {
			if uf, ok := stateNames[seg]; ok {
				return uf, i
			}
		}
	}
	return "", -1
}

func cityKey(s string) string {
	return strings.ReplaceAll(s, "-", " ")
}

func LocationTier(city, state string) string {
	if city != "" {
		if metroState, ok := metroCities[cityKey(city)]; ok {
			if state == "" || state == metroState {
				return "metro"
			}
		}
	}
	if targetStates[state] {
		return "state"
	}
	return "outside"
}

func isBrazilianState(s string) bool {
	switch s {
	case "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS",
		"MG", "PA", "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC",
		"SP", "SE", "TO":
		return true
	}
	return false
}
```

O local vem em formatos muito mais variados que `Cidade - UF`. `ParseLocation`
quebra a string em segmentos por vírgula, barra, parênteses e hífen cercado de
espaço, procura o estado de trás para frente (sigla de duas letras ou nome por
extenso) e então procura a cidade testando cada segmento contra a tabela.

Cada regra existe por um formato real que a versão presa ao sufixo rejeitava por
completo: `"Rio de Janeiro - RJ - Brasil"` e `"Guarulhos - SP (Cumbica)"` têm
texto depois da UF; `"São Paulo (SP)"` põe a UF entre parênteses;
`"Curitiba - Paraná"` escreve o estado por extenso, que é o formato do Mercado
Livre. Todos viravam `outside`, ou seja, anúncio dentro do alvo descartado.
`"Copacabana, Rio de Janeiro - RJ"` prefixa o bairro e rebaixava um Match a
Talvez.

Quando mais de um segmento bate a tabela — `"Lapa, São Paulo - SP"`, em que Lapa
é município do Paraná e bairro de São Paulo — vence o segmento cuja UF na tabela
coincide com o estado detectado. Sem esse desempate a cidade sairia como `lapa`
com estado `SP`, combinação que não é metro e rebaixaria o anúncio.

Duas armadilhas desta função foram descobertas testando-a contra formatos reais
e cada uma tem um teste dedicado.

A primeira: o segmento que forneceu o estado precisa ser excluído da busca por
cidade. `"Campinas - São Paulo"` tem `sao paulo` como estado por extenso, e essa
mesma string existe na tabela de cidades — sem a exclusão ela vence o desempate,
a cidade sai como `sao paulo` e uma moto em Campinas é classificada como Match
metropolitano. O `stateIndex` existe só para isso. Quando o estado é o único
segmento, como em `"Rio de Janeiro"` sem UF, o fallback final o reaproveita como
cidade, que é o comportamento correto para a capital.

A terceira: um nome de estado por extenso só conta como estado quando NÃO é o
primeiro segmento, ou quando é o único. `"Rio de Janeiro"` e `"São Paulo"` são
simultaneamente cidade e estado, e o que os desambigua é a posição: em
`"Rio de Janeiro, Copacabana"` a capital vem primeiro e é cidade; em
`"Cabo Frio, Rio de Janeiro"` vem depois e é estado. Siglas de duas letras
continuam aceitas em qualquer posição.

Quando nenhum segmento consta da tabela de cidades, o fallback devolve o
segmento IMEDIATAMENTE ANTERIOR ao estado — `stateIndex - 1` — e não o primeiro
nem o último. Anúncio real põe a cidade colada ao estado; o que vem antes dela é
bairro e o que vem depois é ruído. `"Vila Isabel, Volta Redonda - RJ"` deve
devolver `volta redonda`, não `vila isabel`. O tier não muda nesse caso, já que
nenhuma das duas está na tabela metropolitana, mas a cidade é gravada no banco e
exibida no dashboard, então o dado errado apareceria para quem revisa.

O limite superior da varredura é o que importa e é fácil errar: varrer de trás
para frente sem parar no estado devolve o ruído do fim, quebrando
`"Campinas - São Paulo - Brasil"` (viria `brasil`) e
`"Santos, São Paulo (Zona Leste)"` (viria `zona leste`) — casos que esta mesma
tabela de testes exige. Quando nenhum estado é encontrado, a varredura começa no
último segmento, porque aí não há sufixo a evitar.

O gate é por posição inicial e não por posição final porque o sufixo depois do
estado é comum: `"Campinas - São Paulo - Brasil"` e
`"Santos, São Paulo (Zona Leste)"` têm o estado no meio. Exigir que fosse o
último segmento faria o estado passar despercebido, o segmento órfão `sao paulo`
casaria a tabela de cidades com estado vazio, e uma moto em Campinas viraria
Match na capital. A cláusula `len(segments) == 1` preserva a capital sozinha.

A segunda: a UF colada por hífen ou espaço simples, `"Curitiba-PR"` e
`"Curitiba PR"`, não é separada pela segmentação, porque o hífen só separa com
espaço ao lado. Por isso `trailingState` extrai a sigla final antes de segmentar.
`"Embu-Guaçu"` não é afetada porque seu último token tem cinco letras.

O hífen só separa quando tem espaço de pelo menos um lado, para que
`"Embu-Guaçu"` não se parta em dois. `cityKey` normaliza hífen para espaço na
comparação, de modo que as duas grafias encontram a mesma entrada.

- [ ] **Step 5: Rodar os testes e confirmar que passam**

Run: `go test ./internal/normalize/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/normalize/regions.go internal/normalize/location.go internal/normalize/location_test.go
git commit -m "feat: resolve city and state into metro area tiers"
```

---

### Task 6: Montagem do Listing e impressão digital

**Files:**
- Create: `internal/normalize/normalize.go`
- Test: `internal/normalize/normalize_test.go`

**Interfaces:**
- Consumes: `ParsePrice`, `ParseYear`, `ParseKm`, `DetectBike`, `ParseLocation` das Tasks 3 a 5; `model.RawListing` e `model.Listing` da Task 2
- Produces: `normalize.Normalize(raw model.RawListing) model.Listing` e `normalize.Fingerprint(l model.Listing) string`

- [ ] **Step 1: Escrever o teste que falha**

`internal/normalize/normalize_test.go`:

```go
package normalize

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestNormalizeUsesStructuredFieldsFirst(t *testing.T) {
	raw := model.RawListing{
		Source:       "olx",
		ExternalID:   "123",
		Title:        "Harley Davidson Street Glide Special",
		PriceText:    "R$ 72.000",
		YearText:     "2015",
		KmText:       "31.000 km",
		LocationText: "Curitiba - PR",
	}
	l := Normalize(raw)

	if l.Bike != model.BikeStreetGlide || l.Variant != model.VariantSpecial {
		t.Errorf("bike/variant = %q/%q", l.Bike, l.Variant)
	}
	if l.Year == nil || *l.Year != 2015 {
		t.Errorf("Year = %v, want 2015", l.Year)
	}
	if l.PriceCents == nil || *l.PriceCents != 7200000 {
		t.Errorf("PriceCents = %v, want 7200000", l.PriceCents)
	}
	if l.Km == nil || *l.Km != 31000 {
		t.Errorf("Km = %v, want 31000", l.Km)
	}
	if l.City != "curitiba" || l.State != "PR" {
		t.Errorf("location = %q/%q", l.City, l.State)
	}
}

func TestNormalizeFallsBackToFreeText(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "abc",
		RawText:    "Vendo Road Glide Special 15/15, 42.000 km, R$ 74.900, Sao Paulo SP",
	}
	l := Normalize(raw)

	if l.Bike != model.BikeRoadGlide {
		t.Errorf("Bike = %q, want road_glide", l.Bike)
	}
	if l.Year == nil || *l.Year != 2015 {
		t.Errorf("Year = %v, want 2015", l.Year)
	}
	if l.PriceCents == nil || *l.PriceCents != 7490000 {
		t.Errorf("PriceCents = %v, want 7490000", l.PriceCents)
	}
}

func TestNormalizeLeavesMissingFieldsNil(t *testing.T) {
	raw := model.RawListing{
		Source:     "olx",
		ExternalID: "999",
		Title:      "Harley Street Glide",
		PriceText:  "a combinar",
	}
	l := Normalize(raw)

	if l.PriceCents != nil {
		t.Errorf("PriceCents = %v, want nil", l.PriceCents)
	}
	if l.Year != nil {
		t.Errorf("Year = %v, want nil", l.Year)
	}
}

func TestNormalizeResolvesCityDeterministically(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "det1",
		RawText:    "Street Glide 2015 em Sao Jose dos Pinhais, aceito troca",
	}
	first := Normalize(raw)
	for i := 0; i < 50; i++ {
		if got := Normalize(raw); got.City != first.City || got.Fingerprint != first.Fingerprint {
			t.Fatalf("run %d gave %q/%s, first gave %q/%s", i, got.City, got.Fingerprint, first.City, first.Fingerprint)
		}
	}
	if first.City != "sao jose dos pinhais" {
		t.Errorf("City = %q, want sao jose dos pinhais", first.City)
	}
}

func TestNormalizeCityMatchesWholeWordsOnly(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "word1",
		RawText:    "Street Glide 2015, mais imagens no WhatsApp, Rio de Janeiro RJ",
	}
	if l := Normalize(raw); l.City != "rio de janeiro" {
		t.Errorf("City = %q, want rio de janeiro", l.City)
	}
}

func TestNormalizePrefersLongestCityMatch(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "long1",
		RawText:    "Street Glide 2015, moto na Lapa, Sao Paulo capital",
	}
	l := Normalize(raw)
	if l.City != "sao paulo" || l.State != "SP" {
		t.Errorf("location = %q/%q, want sao paulo/SP", l.City, l.State)
	}
}

func TestNormalizeIgnoresFiscalYears(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "fiscal1",
		RawText:    "IPVA 2026 pago. Vendo Road Glide 2015, Curitiba - PR",
	}
	l := Normalize(raw)
	if l.Year == nil || *l.Year != 2015 {
		t.Errorf("Year = %v, want 2015", l.Year)
	}
}

func TestNormalizeReadsPriceAfterMileage(t *testing.T) {
	raw := model.RawListing{
		Source:     "instagram",
		ExternalID: "price1",
		RawText:    "Street Glide 2015, 42 mil km, valor 74 mil, Curitiba - PR",
	}
	l := Normalize(raw)
	if l.PriceCents == nil || *l.PriceCents != 7400000 {
		t.Errorf("PriceCents = %v, want 7400000", l.PriceCents)
	}
}

func TestNormalizeFindsCityWrittenWithAttachedState(t *testing.T) {
	cases := []struct {
		text string
		city string
	}{
		{"Street Glide 2015, moto em Curitiba-PR, aceito troca", "curitiba"},
		{"Road Glide 2015 (Guarulhos-SP) impecavel", "guarulhos"},
		{"Street Glide 2014, Embu-Guacu SP", "embu guacu"},
	}
	for _, c := range cases {
		t.Run(c.city, func(t *testing.T) {
			l := Normalize(model.RawListing{Source: "instagram", ExternalID: c.city, RawText: c.text})
			if l.City != c.city {
				t.Errorf("City = %q, want %q", l.City, c.city)
			}
		})
	}
}

func TestNormalizeIgnoresMoreFiscalYearShapes(t *testing.T) {
	cases := []string{
		"IPVA/2026 pago. Street Glide 2015, Curitiba - PR",
		"Documento 2026 ok. Road Glide 2015, Curitiba - PR",
		"Emplacada 2026. Street Glide 2015, Curitiba - PR",
		"Documentação 2026 em dia. Street Glide 2015, Curitiba - PR",
		"Documentos 2026 ok. Road Glide 2015, Curitiba - PR",
		"IPVA 2026 PAGO. VENDO STREET GLIDE 2015, CURITIBA-PR",
	}
	for _, text := range cases {
		t.Run(string([]rune(text)[:12]), func(t *testing.T) {
			l := Normalize(model.RawListing{Source: "instagram", ExternalID: text[:8], RawText: text})
			if l.Year == nil || *l.Year != 2015 {
				t.Errorf("Year = %v, want 2015", l.Year)
			}
		})
	}
}

func TestFingerprintIsStableAndDiscriminating(t *testing.T) {
	year := 2015
	km := 31200
	base := model.Listing{Bike: model.BikeStreetGlide, Year: &year, Km: &km, City: "curitiba"}

	otherKm := 33000
	sameBucket := base
	sameBucket.Km = &otherKm

	if Fingerprint(base) != Fingerprint(sameBucket) {
		t.Error("listings within the same mileage bucket should share a fingerprint")
	}

	farKm := 90000
	different := base
	different.Km = &farKm
	if Fingerprint(base) == Fingerprint(different) {
		t.Error("listings with very different mileage should not share a fingerprint")
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar a falha**

Run: `go test ./internal/normalize/ -run TestNormalize -v`
Expected: FAIL — `Normalize` não existe.

- [ ] **Step 3: Implementar**

`internal/normalize/normalize.go`:

```go
package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

const mileageBucketSize = 5000

var fiscalYear = regexp.MustCompile(`\b(ipva|licenciad\w*|licenciamento|crlv|seguro|financiamento|document\w*|emplacad\w*)\s*(?:/|de)?\s*(19|20)\d{2}`)

func Normalize(raw model.RawListing) model.Listing {
	full := strings.TrimSpace(raw.Title + " " + raw.RawText)

	l := model.Listing{
		Source:     raw.Source,
		ExternalID: raw.ExternalID,
		URL:        raw.URL,
		Title:      raw.Title,
		RawText:    raw.RawText,
		ImageURL:   raw.ImageURL,
	}

	l.Bike, l.Variant = DetectBike(full)

	if cents, ok := ParsePrice(raw.PriceText); ok {
		l.PriceCents = &cents
	} else if cents, ok := ParsePrice(full); ok {
		l.PriceCents = &cents
	}

	if year, ok := ParseYear(raw.YearText); ok {
		l.Year = &year
	} else if year, ok := ParseYear(fiscalYear.ReplaceAllString(Fold(full), " ")); ok {
		l.Year = &year
	}

	if km, ok := ParseKm(raw.KmText); ok {
		l.Km = &km
	} else if km, ok := ParseKm(full); ok {
		l.Km = &km
	}

	l.City, l.State = ParseLocation(raw.LocationText)
	if l.City == "" {
		l.City, l.State = locationFromText(full)
	}

	l.Fingerprint = Fingerprint(l)
	return l
}

func locationFromText(text string) (string, string) {
	folded := cityKey(Fold(text))
	bestCity, bestState, bestIndex := "", "", 0
	for city, state := range metroCities {
		index := wordIndex(folded, city)
		if index < 0 {
			continue
		}
		if bestCity == "" || len(city) > len(bestCity) ||
			(len(city) == len(bestCity) && index < bestIndex) {
			bestCity, bestState, bestIndex = city, state, index
		}
	}
	return bestCity, bestState
}

func wordIndex(text, term string) int {
	for from := 0; from <= len(text)-len(term); {
		offset := strings.Index(text[from:], term)
		if offset < 0 {
			return -1
		}
		start := from + offset
		if !wordChar(text, start-1) && !wordChar(text, start+len(term)) {
			return start
		}
		from = start + 1
	}
	return -1
}

func wordChar(text string, i int) bool {
	if i < 0 || i >= len(text) {
		return false
	}
	c := text[i]
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

func Fingerprint(l model.Listing) string {
	year := "?"
	if l.Year != nil {
		year = fmt.Sprint(*l.Year)
	}
	bucket := "?"
	if l.Km != nil {
		bucket = fmt.Sprint(*l.Km / mileageBucketSize)
	}
	seed := strings.Join([]string{l.Bike, year, bucket, l.City}, "|")
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8])
}
```

`locationFromText` percorre um mapa, e a ordem de iteração de mapa em Go é
aleatória por construção. Retornar a primeira cidade encontrada tornaria a
função não determinística, e como a cidade entra no `Fingerprint`, a impressão
digital mudaria entre execuções — destruindo exatamente a detecção de reanúncio
que ela existe para fazer. A tabela colide consigo mesma (`pinhais` é sufixo de
`sao jose dos pinhais`), então a colisão não é hipotética. O critério de escolha
é determinístico: vence o nome mais longo e, em empate de tamanho, o de menor
posição no texto.

O texto passa por `cityKey` antes da busca, convertendo hífen em espaço. Isso
resolve dois casos de uma vez: `"Curitiba-PR"` escrito no corpo do anúncio passa
a encontrar `curitiba`, e `"Embu-Guaçu"` encontra a entrada da tabela, que é
grafada com espaço desde a Task 5. Sem essa normalização o caminho de texto
livre rejeitaria exatamente a forma que o caminho estruturado aceita.

O mesmo mecanismo serve de última tentativa em `ParseLocation`: quando nenhum
segmento bate exatamente a tabela, procura-se uma cidade DENTRO do segmento.
`"São Paulo Zona Sul"` é como o Mercado Livre nomeia a localização, e sem essa
busca o texto inteiro vira uma "cidade" inexistente na tabela e devolve
`outside` — a capital paulista, uma das três regiões-alvo, rejeitada por
completo nessa fonte.

A busca é por palavra inteira, não por substring. `mage` aparece dentro de
`imagens`, e "mais imagens no WhatsApp" é frase corriqueira em anúncio — sem o
limite de palavra, o anúncio seria gravado como se estivesse em Magé. O critério
de nome mais longo resolve o outro caso: em "moto na Lapa, São Paulo capital",
`sao paulo` vence `lapa`, evitando que uma moto paulista seja gravada no Paraná.

O texto é normalizado com `Fold` antes de remover os anos fiscais, e o padrão
usa `document\w*`. As duas coisas dependem uma da outra: o padrão não tem
`(?i)` justamente porque `Fold` já baixou a caixa, e `IPVA` e `CRLV` aparecem
quase sempre em maiúsculas no anúncio. Trocar `Fold(full)` de volta por `full`
faria toda palavra-chave maiúscula deixar de casar — por isso a tabela tem um
caso inteiramente em caixa alta, que falha se alguém desfizer a dependência. Sem o `Fold`, `"Documentação 2026"` escaparia: `\w` em Go é
ASCII e para no `ç`, então nenhuma variação do padrão alcança a palavra
acentuada como ela aparece no anúncio. O `\w*` cobre o plural `"Documentos"`.

Anos fiscais são removidos antes de procurar o ano no texto livre. `"IPVA 2026
pago. Vendo Road Glide 2015"` devolveria 2026, que o matcher rejeita de imediato
— um anúncio dentro do alvo perdido por causa do ano do licenciamento.

O `Fingerprint` agrupa quilometragem em faixas de 5.000 km porque o mesmo vendedor reanuncia com número redondo diferente ("31.200" vira "31 mil"), e exigir igualdade exata perderia todo reanúncio.

- [ ] **Step 4: Rodar os testes e confirmar que passam**

Run: `go test ./internal/normalize/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/normalize/normalize.go internal/normalize/normalize_test.go
git commit -m "feat: assemble normalized listings with reposting fingerprint"
```

---

### Task 7: Matcher

**Files:**
- Create: `internal/match/match.go`
- Test: `internal/match/match_test.go`

**Interfaces:**
- Consumes: `model.Listing`, `model.Verdict`, `model.CombineVerdicts` da Task 2; `normalize.LocationTier` da Task 5; `config.MatchCriteria` da Task 1
- Produces: `match.Evaluate(l model.Listing, c config.MatchCriteria) (model.Verdict, map[string]model.Verdict)`

- [ ] **Step 1: Escrever o teste que falha**

`internal/match/match_test.go`:

```go
package match

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/model"
)

func criteria() config.MatchCriteria {
	return config.MatchCriteria{
		Years:              []int{2014, 2015},
		MaybeYears:         []int{2013, 2016},
		MaxPriceCents:      7500000,
		MaybeMaxPriceCents: 8500000,
	}
}

func listing(bike string, year int, cents int64, city, state string) model.Listing {
	l := model.Listing{Bike: bike, City: city, State: state}
	if year != 0 {
		l.Year = &year
	}
	if cents != 0 {
		l.PriceCents = &cents
	}
	return l
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name string
		in   model.Listing
		want model.Verdict
	}{
		{"street glide in target", listing(model.BikeStreetGlide, 2015, 7200000, "curitiba", "PR"), model.VerdictMatch},
		{"road glide in target", listing(model.BikeRoadGlide, 2015, 7490000, "sao paulo", "SP"), model.VerdictMatch},
		{"metro area counts", listing(model.BikeStreetGlide, 2014, 7000000, "niteroi", "RJ"), model.VerdictMatch},
		{"electra glide rejected", listing(model.BikeElectraGlide, 2015, 7000000, "curitiba", "PR"), model.VerdictReject},
		{"other brand rejected", listing(model.BikeOther, 2015, 7000000, "curitiba", "PR"), model.VerdictReject},
		{"adjacent year is maybe", listing(model.BikeStreetGlide, 2016, 7000000, "curitiba", "PR"), model.VerdictMaybe},
		{"missing year is maybe", listing(model.BikeStreetGlide, 0, 7000000, "curitiba", "PR"), model.VerdictMaybe},
		{"missing price is maybe", listing(model.BikeStreetGlide, 2015, 0, "curitiba", "PR"), model.VerdictMaybe},
		{"slightly over budget is maybe", listing(model.BikeStreetGlide, 2015, 8000000, "curitiba", "PR"), model.VerdictMaybe},
		{"far over budget rejected", listing(model.BikeStreetGlide, 2015, 9500000, "curitiba", "PR"), model.VerdictReject},
		{"same state is maybe", listing(model.BikeStreetGlide, 2015, 7000000, "campinas", "SP"), model.VerdictMaybe},
		{"other state rejected", listing(model.BikeStreetGlide, 2015, 7000000, "belo horizonte", "MG"), model.VerdictReject},
		{"unknown touring is maybe", listing(model.BikeTouringUnknown, 2015, 7000000, "curitiba", "PR"), model.VerdictMaybe},
		{"old year rejected", listing(model.BikeStreetGlide, 2009, 7000000, "curitiba", "PR"), model.VerdictReject},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, axes := Evaluate(c.in, criteria())
			if got != c.want {
				t.Errorf("Evaluate = %q, want %q (axes: %v)", got, c.want, axes)
			}
			if len(axes) != 4 {
				t.Errorf("expected 4 axes, got %d", len(axes))
			}
		})
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar a falha**

Run: `go test ./internal/match/ -v`
Expected: FAIL — pacote não compila.

- [ ] **Step 3: Implementar**

`internal/match/match.go`:

```go
package match

import (
	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
)

func Evaluate(l model.Listing, c config.MatchCriteria) (model.Verdict, map[string]model.Verdict) {
	axes := map[string]model.Verdict{
		model.AxisModel:    evaluateBike(l.Bike),
		model.AxisYear:     evaluateYear(l.Year, c),
		model.AxisPrice:    evaluatePrice(l.PriceCents, c),
		model.AxisLocation: evaluateLocation(l.City, l.State),
	}
	return model.CombineVerdicts(axes), axes
}

func evaluateBike(bike string) model.Verdict {
	switch bike {
	case model.BikeStreetGlide, model.BikeRoadGlide:
		return model.VerdictMatch
	case model.BikeTouringUnknown:
		return model.VerdictMaybe
	default:
		return model.VerdictReject
	}
}

func evaluateYear(year *int, c config.MatchCriteria) model.Verdict {
	if year == nil {
		return model.VerdictMaybe
	}
	if contains(c.Years, *year) {
		return model.VerdictMatch
	}
	if contains(c.MaybeYears, *year) {
		return model.VerdictMaybe
	}
	return model.VerdictReject
}

func evaluatePrice(cents *int64, c config.MatchCriteria) model.Verdict {
	if cents == nil {
		return model.VerdictMaybe
	}
	if *cents <= c.MaxPriceCents {
		return model.VerdictMatch
	}
	if *cents <= c.MaybeMaxPriceCents {
		return model.VerdictMaybe
	}
	return model.VerdictReject
}

func evaluateLocation(city, state string) model.Verdict {
	switch normalize.LocationTier(city, state) {
	case "metro":
		return model.VerdictMatch
	case "state":
		return model.VerdictMaybe
	default:
		return model.VerdictReject
	}
}

func contains(list []int, v int) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Rodar o teste e confirmar que passa**

Run: `go test ./internal/match/ -v`
Expected: PASS nos 14 casos.

- [ ] **Step 5: Commit**

```bash
git add internal/match
git commit -m "feat: classify listings across model, year, price and location axes"
```

---

### Task 8: Persistência com deduplicação e histórico de preço

**Files:**
- Create: `internal/store/store.go`, `internal/store/upsert.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `model.Listing`, `model.Verdict` da Task 2
- Produces:
  - `store.Open(path string) (*store.Store, error)` — já aplica as migrações
  - `(*Store).Close() error`
  - `(*Store).Upsert(l model.Listing, now time.Time) (store.UpsertResult, error)` com `UpsertResult{ID int64, IsNew bool, PriceChanged bool, PreviousCents *int64}`
  - `(*Store).PendingNotifications(limit int) ([]store.Row, error)` — apenas `verdict = 'match'` e `notified = 0`
  - `(*Store).MarkNotified(id int64) error`
  - `(*Store).ListByVerdict(v model.Verdict) ([]store.Row, error)`
  - `(*Store).GetRow(id int64) (store.Row, []store.PricePoint, error)`
  - `(*Store).SetUserState(id int64, state string) error`
  - `(*Store).RecordRun(source string, started, finished time.Time, itemCount int, status, errMessage string) error`
  - `(*Store).RecentRunCounts(source string, limit int) ([]int, error)`
  - `store.Row` com campos `ID int64`, `Source`, `ExternalID`, `URL`, `Title`, `Bike`, `Variant string`, `Year *int`, `PriceCents *int64`, `Km *int`, `City`, `State`, `ImageURL string`, `Verdict model.Verdict`, `UserState string`, `Fingerprint string`, `FirstSeenAt`, `LastSeenAt time.Time`, `FirstPriceCents *int64`
  - `store.PricePoint` com `PriceCents int64` e `ObservedAt time.Time`

- [ ] **Step 1: Instalar o driver**

```bash
go get modernc.org/sqlite
```

- [ ] **Step 2: Escrever o teste que falha**

`internal/store/store_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sample(cents int64) model.Listing {
	year := 2015
	km := 31000
	return model.Listing{
		Source:      "olx",
		ExternalID:  "abc123",
		URL:         "https://olx.com.br/abc123",
		Title:       "Harley Street Glide 2015",
		Bike:        model.BikeStreetGlide,
		Variant:     model.VariantBase,
		Year:        &year,
		PriceCents:  &cents,
		Km:          &km,
		City:        "curitiba",
		State:       "PR",
		Verdict:     model.VerdictMatch,
		Fingerprint: "deadbeef",
	}
}

func TestUpsertInsertsThenDeduplicates(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	first, err := s.Upsert(sample(7200000), now)
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	if !first.IsNew {
		t.Error("first insert should report IsNew")
	}

	second, err := s.Upsert(sample(7200000), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if second.IsNew {
		t.Error("same external id should not be reported as new")
	}
	if second.ID != first.ID {
		t.Errorf("ID changed on re-upsert: %d then %d", first.ID, second.ID)
	}
	if second.PriceChanged {
		t.Error("unchanged price should not report a change")
	}

	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestUpsertRecordsPriceDrop(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	if _, err := s.Upsert(sample(7500000), now); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	res, err := s.Upsert(sample(7100000), now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !res.PriceChanged {
		t.Fatal("price drop should be reported")
	}
	if res.PreviousCents == nil || *res.PreviousCents != 7500000 {
		t.Errorf("PreviousCents = %v, want 7500000", res.PreviousCents)
	}

	_, points, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 price points, got %d", len(points))
	}
}

func TestPendingNotificationsOnlyReturnsUnnotifiedMatches(t *testing.T) {
	s := openTemp(t)
	now := time.Now()

	maybeListing := sample(7200000)
	maybeListing.ExternalID = "maybe1"
	maybeListing.Verdict = model.VerdictMaybe
	if _, err := s.Upsert(maybeListing, now); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	res, err := s.Upsert(sample(7200000), now)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != res.ID {
		t.Fatalf("expected only the match row, got %d rows", len(pending))
	}

	if err := s.MarkNotified(res.ID); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	pending, err = s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected no pending rows after marking, got %d", len(pending))
	}
}

func TestSetUserState(t *testing.T) {
	s := openTemp(t)
	res, err := s.Upsert(sample(7200000), time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := s.SetUserState(res.ID, "contacted"); err != nil {
		t.Fatalf("SetUserState: %v", err)
	}
	row, _, err := s.GetRow(res.ID)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.UserState != "contacted" {
		t.Errorf("UserState = %q, want contacted", row.UserState)
	}
}

func TestRecentRunCounts(t *testing.T) {
	s := openTemp(t)
	now := time.Now()
	for i, count := range []int{7, 9, 0} {
		start := now.Add(time.Duration(i) * time.Hour)
		if err := s.RecordRun("olx", start, start.Add(time.Minute), count, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	counts, err := s.RecentRunCounts("olx", 5)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	if len(counts) != 3 || counts[0] != 0 {
		t.Errorf("counts = %v, want most recent first starting with 0", counts)
	}
}
```

- [ ] **Step 3: Rodar o teste e confirmar a falha**

Run: `go test ./internal/store/ -v`
Expected: FAIL — pacote não compila.

- [ ] **Step 4: Implementar abertura, esquema e consultas**

`internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Row struct {
	ID              int64
	Source          string
	ExternalID      string
	URL             string
	Title           string
	Bike            string
	Variant         string
	Year            *int
	PriceCents      *int64
	Km              *int
	City            string
	State           string
	ImageURL        string
	Verdict         model.Verdict
	UserState       string
	Fingerprint     string
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	FirstPriceCents *int64
}

type PricePoint struct {
	PriceCents int64
	ObservedAt time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS listings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    raw_text TEXT NOT NULL DEFAULT '',
    bike TEXT NOT NULL,
    variant TEXT NOT NULL,
    year INTEGER,
    price_cents INTEGER,
    km INTEGER,
    city TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    image_url TEXT NOT NULL DEFAULT '',
    verdict TEXT NOT NULL,
    verdict_reason TEXT NOT NULL DEFAULT '{}',
    fingerprint TEXT NOT NULL DEFAULT '',
    user_state TEXT NOT NULL DEFAULT 'new',
    notified INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    first_seen_at TIMESTAMP NOT NULL,
    last_seen_at TIMESTAMP NOT NULL,
    UNIQUE (source, external_id)
);

CREATE INDEX IF NOT EXISTS idx_listings_verdict ON listings (verdict, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_listings_fingerprint ON listings (fingerprint);

CREATE TABLE IF NOT EXISTS price_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    listing_id INTEGER NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    price_cents INTEGER NOT NULL,
    observed_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_price_history_listing ON price_history (listing_id, observed_at);

CREATE TABLE IF NOT EXISTS source_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    started_at TIMESTAMP NOT NULL,
    finished_at TIMESTAMP NOT NULL,
    item_count INTEGER NOT NULL,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_source_runs_source ON source_runs (source, started_at DESC);
`

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

const rowColumns = `
    l.id, l.source, l.external_id, l.url, l.title, l.bike, l.variant, l.year,
    l.price_cents, l.km, l.city, l.state, l.image_url, l.verdict, l.user_state,
    l.fingerprint, l.first_seen_at, l.last_seen_at,
    (SELECT price_cents FROM price_history p WHERE p.listing_id = l.id ORDER BY p.observed_at ASC LIMIT 1)
`

func scanRow(scanner interface{ Scan(...any) error }) (Row, error) {
	var r Row
	err := scanner.Scan(&r.ID, &r.Source, &r.ExternalID, &r.URL, &r.Title, &r.Bike,
		&r.Variant, &r.Year, &r.PriceCents, &r.Km, &r.City, &r.State, &r.ImageURL,
		&r.Verdict, &r.UserState, &r.Fingerprint, &r.FirstSeenAt, &r.LastSeenAt,
		&r.FirstPriceCents)
	return r, err
}

func (s *Store) ListByVerdict(v model.Verdict) ([]Row, error) {
	query := "SELECT " + rowColumns + " FROM listings l WHERE l.verdict = ? ORDER BY l.first_seen_at DESC"
	rows, err := s.db.Query(query, string(v))
	if err != nil {
		return nil, fmt.Errorf("querying listings: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

func (s *Store) PendingNotifications(limit int) ([]Row, error) {
	query := "SELECT " + rowColumns + ` FROM listings l
        WHERE l.verdict = 'match' AND l.notified = 0 AND l.status = 'active'
        ORDER BY l.first_seen_at ASC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("querying pending notifications: %w", err)
	}
	defer rows.Close()
	return collectRows(rows)
}

func (s *Store) MarkNotified(id int64) error {
	if _, err := s.db.Exec("UPDATE listings SET notified = 1 WHERE id = ?", id); err != nil {
		return fmt.Errorf("marking listing as notified: %w", err)
	}
	return nil
}

func (s *Store) SetUserState(id int64, state string) error {
	if _, err := s.db.Exec("UPDATE listings SET user_state = ? WHERE id = ?", state, id); err != nil {
		return fmt.Errorf("updating user state: %w", err)
	}
	return nil
}

func (s *Store) GetRow(id int64) (Row, []PricePoint, error) {
	query := "SELECT " + rowColumns + " FROM listings l WHERE l.id = ?"
	row, err := scanRow(s.db.QueryRow(query, id))
	if err != nil {
		return Row{}, nil, fmt.Errorf("loading listing: %w", err)
	}

	rows, err := s.db.Query(
		"SELECT price_cents, observed_at FROM price_history WHERE listing_id = ? ORDER BY observed_at ASC", id)
	if err != nil {
		return Row{}, nil, fmt.Errorf("loading price history: %w", err)
	}
	defer rows.Close()

	var points []PricePoint
	for rows.Next() {
		var p PricePoint
		if err := rows.Scan(&p.PriceCents, &p.ObservedAt); err != nil {
			return Row{}, nil, fmt.Errorf("scanning price point: %w", err)
		}
		points = append(points, p)
	}
	return row, points, rows.Err()
}

func (s *Store) RecordRun(source string, started, finished time.Time, itemCount int, status, errMessage string) error {
	_, err := s.db.Exec(
		`INSERT INTO source_runs (source, started_at, finished_at, item_count, status, error)
         VALUES (?, ?, ?, ?, ?, ?)`,
		source, started, finished, itemCount, status, errMessage)
	if err != nil {
		return fmt.Errorf("recording source run: %w", err)
	}
	return nil
}

func (s *Store) RecentRunCounts(source string, limit int) ([]int, error) {
	rows, err := s.db.Query(
		"SELECT item_count FROM source_runs WHERE source = ? ORDER BY started_at DESC LIMIT ?", source, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recent runs: %w", err)
	}
	defer rows.Close()

	var counts []int
	for rows.Next() {
		var c int
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scanning run count: %w", err)
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
}

func collectRows(rows *sql.Rows) ([]Row, error) {
	var result []Row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning listing: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
```

- [ ] **Step 5: Implementar o upsert**

`internal/store/upsert.go`:

```go
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

type UpsertResult struct {
	ID            int64
	IsNew         bool
	PriceChanged  bool
	PreviousCents *int64
}

func (s *Store) Upsert(l model.Listing, now time.Time) (UpsertResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return UpsertResult{}, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var id int64
	var existingPrice *int64
	err = tx.QueryRow(
		"SELECT id, price_cents FROM listings WHERE source = ? AND external_id = ?",
		l.Source, l.ExternalID).Scan(&id, &existingPrice)

	reason, marshalErr := json.Marshal(l.VerdictReason)
	if marshalErr != nil {
		return UpsertResult{}, fmt.Errorf("encoding verdict reason: %w", marshalErr)
	}

	result := UpsertResult{}

	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, insertErr := tx.Exec(
			`INSERT INTO listings
             (source, external_id, url, title, raw_text, bike, variant, year, price_cents,
              km, city, state, image_url, verdict, verdict_reason, fingerprint,
              first_seen_at, last_seen_at)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			l.Source, l.ExternalID, l.URL, l.Title, l.RawText, l.Bike, l.Variant,
			l.Year, l.PriceCents, l.Km, l.City, l.State, l.ImageURL,
			string(l.Verdict), string(reason), l.Fingerprint, now, now)
		if insertErr != nil {
			return UpsertResult{}, fmt.Errorf("inserting listing: %w", insertErr)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return UpsertResult{}, fmt.Errorf("reading inserted id: %w", err)
		}
		result.IsNew = true

	case err != nil:
		return UpsertResult{}, fmt.Errorf("looking up listing: %w", err)

	default:
		if _, updateErr := tx.Exec(
			`UPDATE listings SET url = ?, title = ?, raw_text = ?, bike = ?, variant = ?,
             year = ?, price_cents = ?, km = ?, city = ?, state = ?, image_url = ?,
             verdict = ?, verdict_reason = ?, fingerprint = ?, status = 'active', last_seen_at = ?
             WHERE id = ?`,
			l.URL, l.Title, l.RawText, l.Bike, l.Variant, l.Year, l.PriceCents, l.Km,
			l.City, l.State, l.ImageURL, string(l.Verdict), string(reason),
			l.Fingerprint, now, id); updateErr != nil {
			return UpsertResult{}, fmt.Errorf("updating listing: %w", updateErr)
		}
		if l.PriceCents != nil && (existingPrice == nil || *existingPrice != *l.PriceCents) {
			result.PriceChanged = true
			result.PreviousCents = existingPrice
		}
	}

	if l.PriceCents != nil && (result.IsNew || result.PriceChanged) {
		if _, histErr := tx.Exec(
			"INSERT INTO price_history (listing_id, price_cents, observed_at) VALUES (?, ?, ?)",
			id, *l.PriceCents, now); histErr != nil {
			return UpsertResult{}, fmt.Errorf("recording price history: %w", histErr)
		}
	}

	if err := tx.Commit(); err != nil {
		return UpsertResult{}, fmt.Errorf("committing transaction: %w", err)
	}

	result.ID = id
	return result, nil
}
```

- [ ] **Step 6: Rodar os testes e confirmar que passam**

Run: `go test ./internal/store/ -v`
Expected: PASS nos quatro testes.

Se o scan de `first_seen_at` ou `last_seen_at` falhar com erro de conversão, o driver devolveu texto em vez de `time.Time`. Nesse caso abra a conexão com `sql.Open("sqlite", path+"?_time_format=sqlite")` e rode os testes de novo. Verifique antes de seguir: o histórico de preço depende dessas colunas.

O scan não falha: o `modernc.org/sqlite` grava `time.Time` como texto e lê de
volta o mesmo instante, com fuso preservado. O que quebra é a **ordenação**. O
texto gravado é `2026-08-09 18:00:00 -0300 BRT`, hora local seguida do offset,
e todo `ORDER BY` nessas colunas compara esse texto letra a letra. Duas
gravações do mesmo instante feitas em fusos diferentes ordenam pela hora de
parede, não pela linha do tempo — uma coleta às 21:00 UTC gravada como
`18:00:00 -0300` fica *antes* de outra às 20:00 UTC gravada como
`20:00:00 +0000`.

Não é hipotético: basta o processo rodar local (`-0300`) e depois em container
(`TZ=UTC`), ou vice-versa. O histórico de preço é o que mais sofre, porque é
exatamente a coluna que o painel lê em ordem. Com três observações — 7.500.000,
7.300.000 e 7.100.000 — gravadas com o fuso mudando no meio, `GetRow` devolvia
`[7300000 7500000 7100000]` e `FirstPriceCents` virava 7.300.000. A queda que o
painel anunciaria seria de R$ 2.000, não os R$ 4.000 reais: o sinal de
negociação sai menor do que é, que é a única direção de erro que importa aqui.

Trocar para `?_time_format=sqlite` **não resolve** — muda a pontuação
(`2026-08-09 18:00:00-03:00`) e mantém o offset, então a ordenação continua
lexicográfica sobre hora de parede. A correção é normalizar para UTC na
escrita: `now.UTC()` no `Upsert`, `started.UTC()`/`finished.UTC()` no
`RecordRun`. Todo texto gravado passa a terminar em `+00:00` e a ordem lexical
volta a ser a ordem cronológica.

Todo `ORDER BY` sobre timestamp no pacote ganha `id` como desempate, na mesma
direção da ordenação primária — as cinco consultas, não só as de
`price_history`. Empate é o caso normal, não a exceção: uma coleta passa o mesmo
`now` para todos os `Upsert` da rodada, então os anúncios daquele lote nascem com
`first_seen_at` idêntico por construção.

Sem o desempate, as consultas descendentes devolvem o empate ao contrário.
Medido com cinco anúncios do mesmo lote, `ListByVerdict` (`DESC`) devolvia
`[1 2 3 4 5]` em vez de `[5 4 3 2 1]`, e `RecentRunCounts` com `started_at`
igual devolvia `[10 20 30 40]` — o inverso de "mais recente primeiro", que é o
contrato da função. Não se perde dado: um anúncio empurrado para fora de uma
página de `PendingNotifications` continua com `notified = 0` e entra na rodada
seguinte. O que se perde é a ordem, justamente onde ela tinha acabado de ser
consertada.

`PRAGMA foreign_keys = ON` via `db.Exec` também não vale: pragma é por conexão,
e o `database/sql` mantém um pool. O comando pega a conexão que estiver livre
naquele instante e as outras nascem com a checagem desligada — em oito conexões
simultâneas, sete ficaram com `foreign_keys = 0`. O `ON DELETE CASCADE` do
`price_history` fica valendo só às vezes, o que é pior que não valer nunca. A
pragma vai no DSN (`path+"?_pragma=foreign_keys(1)"`), onde o driver a aplica a
cada conexão que abrir.

O mesmo DSN liga `journal_mode(WAL)` e `busy_timeout(5000)`. A coleta agendada e
o dashboard são processos distintos sobre o mesmo arquivo: sem WAL, uma escrita
bloqueia toda leitura, e sem `busy_timeout` a escrita concorrente recebe
`SQLITE_BUSY` de imediato em vez de esperar sua vez.

`_txlock=immediate` completa o par, e sem ele metade do ganho não existe.
`Upsert` abre a transação, faz o `SELECT` que decide entre inserir e atualizar,
e só então escreve. Uma transação que começa lendo segura um snapshot de
leitura, e a promoção de leitura para escrita é justamente o caso em que o
SQLite ignora o `busy_timeout` de propósito — esperar ali poderia travar os dois
lados. Medido: a escrita concorrente recebeu `SQLITE_BUSY` em 0s com 5000ms
configurados, e é exatamente a escrita que a coleta executa por anúncio. Com
`immediate`, o `BEGIN` toma o lock de escrita antes de qualquer leitura e o
handler volta a valer. Só o `Upsert` abre transação; as leituras seguem em
paralelo sob WAL.

O WAL cumpre o que promete — com uma transação de escrita aberta, a leitura do
outro processo retorna na hora. O `busy_timeout` sozinho **não**: ele não vale
para o `Upsert`, que é justamente a escrita que importa. O `Upsert` abre a
transação, faz o `SELECT` que procura o anúncio e só então grava. Uma transação
que começa lendo pega um snapshot de leitura, e subir de leitura para escrita
com outro escritor no caminho é a única situação em que o SQLite **não** chama o
busy handler: esperar ali poderia travar os dois lados, então ele devolve
`SQLITE_BUSY` na hora. Medido: o escritor concorrente falhava em 0s, com os
5000ms configurados sem efeito nenhum.

Por isso o DSN também leva `_txlock=immediate`, que faz o `BEGIN` já tomar o
lock de escrita, antes de qualquer leitura. Aí não há upgrade, o busy handler
entra e o escritor espera. Com a correção, o mesmo teste espera e conclui sem
erro quando o lock é liberado.

O alcance é pequeno de propósito: `db.Begin()` aparece num único lugar no
pacote, o `Upsert`. As consultas de leitura usam `db.Query`/`db.QueryRow` direto,
sem transação, então continuam entrando em paralelo pelo WAL. `busy_timeout`
segue necessário para as escritas avulsas — `MarkNotified`, `SetUserState`,
`RecordRun` — que são `db.Exec` sem transação e onde o busy handler já valia.

- [ ] **Step 7: Commit**

```bash
git add internal/store go.mod go.sum
git commit -m "feat: sqlite persistence with deduplication and price history"
```

---

### Task 9: Interface Source e coletor da OLX

**Files:**
- Create: `internal/source/source.go`, `internal/source/olx.go`
- Test: `internal/source/olx_test.go`, `internal/source/testdata/olx-search.html`

**Interfaces:**
- Consumes: `model.RawListing` da Task 2
- Produces:
  - `source.Source` interface com `Name() string` e `Fetch(ctx context.Context) ([]model.RawListing, error)`
  - `source.defaultUserAgent`, a string de User-Agent que todas as fontes HTTP usam
  - `source.NewOLX(fetcher source.PageFetcher, baseURLs []string) *source.OLX`
  - `source.PageFetcher` e `source.NewBrowserFetcher(devtoolsURL string) *source.BrowserFetcher`
  - `source.ParseOLX(body io.Reader) ([]model.RawListing, error)` — exportada para permitir teste sem rede

- [ ] **Step 1: Capturar a fixture real**

A OLX serve os resultados dentro de um bloco `<script id="__NEXT_DATA__" type="application/json">`. Baixe uma busca real e salve como fixture:

```bash
mkdir -p internal/source/testdata
curl -sL -A 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)' \
  'https://www.olx.com.br/autos-e-pecas/motos/estado-pr/regiao-de-curitiba-e-paranagua?q=harley%20street%20glide' \
  -o internal/source/testdata/olx-search.html
grep -c '__NEXT_DATA__' internal/source/testdata/olx-search.html
```

Expected: `1`. Se retornar `0`, a OLX serviu uma página de bloqueio; repita mais tarde ou pelo navegador, salvando o HTML da página de resultados. O parser depende dessa estrutura, então a fixture precisa ser real antes de escrever o teste.

- [ ] **Step 2: Escrever o teste que falha**

`internal/source/olx_test.go`:

```go
package source

import (
	"os"
	"testing"
)

func TestParseOLXExtractsListings(t *testing.T) {
	f, err := os.Open("testdata/olx-search.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseOLX(f)
	if err != nil {
		t.Fatalf("ParseOLX: %v", err)
	}
	if len(listings) == 0 {
		t.Fatal("expected at least one listing from the fixture")
	}

	for i, l := range listings {
		if l.Source != "olx" {
			t.Errorf("listing %d: Source = %q, want olx", i, l.Source)
		}
		if l.ExternalID == "" {
			t.Errorf("listing %d: ExternalID is empty", i)
		}
		if l.URL == "" {
			t.Errorf("listing %d: URL is empty", i)
		}
		if l.Title == "" {
			t.Errorf("listing %d: Title is empty", i)
		}
	}
}

func TestParseOLXReturnsErrorWhenBlocked(t *testing.T) {
	blocked := stringReader("<html><body>Acesso negado</body></html>")
	if _, err := ParseOLX(blocked); err == nil {
		t.Fatal("ParseOLX should return an error when the payload is missing")
	}
}
```

Adicione o auxiliar em `internal/source/source_test.go`:

```go
package source

import (
	"io"
	"strings"
)

func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}
```

O segundo teste é o que impede a falha silenciosa: página de bloqueio precisa virar erro visível, não lista vazia.

A mesma exigência vale no nível do campo, e não só no do contêiner. Se os cards
aparecem mas nenhum produz um anúncio — porque o seletor de título ou o padrão
de identificador deixou de casar — o resultado é uma lista vazia sem erro,
indistinguível de uma busca que legitimamente não achou nada. Como cinco dos
sete seletores já mudaram uma vez, esse é o modo de falha esperado no próximo
deploy da fonte. Card individual malformado continua sendo pulado em silêncio;
o que vira erro é a rodada inteira encontrar cards e não extrair nenhum.

A escolha do array de anúncios dentro do payload precisa ser por contexto, não
pela primeira ocorrência que decodificar. A página traz mais de uma lista com a
mesma chave — a de resultados e a da seleção VIP do topo — e hoje a de
resultados vem primeiro apenas por acidente de ordem. Se a OLX passar a popular
a seleção VIP numa busca legitimamente vazia, os itens dela seriam devolvidos
como se fossem o resultado, que é a falha silenciosa desta task ao contrário:
em vez de lista vazia onde havia anúncios, anúncios onde a busca não achou nada.

- [ ] **Step 3: Rodar o teste e confirmar a falha**

Run: `go test ./internal/source/ -v`
Expected: FAIL — `ParseOLX` não existe.

- [ ] **Step 4: Definir a interface**

`internal/source/source.go`:

```go
package source

import (
	"context"
	"fmt"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/chromedp/chromedp"
)

const (
	defaultDevtoolsURL = "http://127.0.0.1:9222"
	browserSettleDelay = 3 * time.Second
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"

type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]model.RawListing, error)
}

type PageFetcher interface {
	FetchPage(ctx context.Context, url string) (string, error)
}

type BrowserFetcher struct {
	devtoolsURL string
	settle      time.Duration
}

func NewBrowserFetcher(devtoolsURL string) *BrowserFetcher {
	if devtoolsURL == "" {
		devtoolsURL = defaultDevtoolsURL
	}
	return &BrowserFetcher{devtoolsURL: devtoolsURL, settle: browserSettleDelay}
}

func (b *BrowserFetcher) FetchPage(ctx context.Context, url string) (string, error) {
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, b.devtoolsURL)
	defer cancelAlloc()

	tabCtx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()

	var html string
	err := chromedp.Run(tabCtx,
		chromedp.Navigate(url),
		chromedp.Sleep(b.settle),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return "", fmt.Errorf("fetching %s through the browser: %w", url, err)
	}
	return html, nil
}
```

- [ ] **Step 5: Implementar o coletor**

```bash
go get github.com/PuerkitoBio/goquery
```

`internal/source/olx.go`:

```go
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andreabreu76/harley-hunter/internal/model"
)

type OLX struct {
	client   *http.Client
	baseURLs []string
}

func NewOLX(client *http.Client, baseURLs []string) *OLX {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &OLX{client: client, baseURLs: baseURLs}
}

func (o *OLX) Name() string { return "olx" }

func (o *OLX) Fetch(ctx context.Context) ([]model.RawListing, error) {
	var all []model.RawListing
	for _, url := range o.baseURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("building request for %s: %w", url, err)
		}
		req.Header.Set("User-Agent", defaultUserAgent)
		req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")

		resp, err := o.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", url, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("fetching %s: status %d", url, resp.StatusCode)
		}

		listings, err := ParseOLX(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", url, err)
		}
		all = append(all, listings...)

		select {
		case <-ctx.Done():
			return all, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return all, nil
}

type olxPayload struct {
	Props struct {
		PageProps struct {
			Ads []struct {
				ListID     json.Number `json:"listId"`
				Subject    string      `json:"subject"`
				Body       string      `json:"body"`
				URL        string      `json:"url"`
				Price      string      `json:"price"`
				Thumbnail  string      `json:"thumbnail"`
				LocationDetails struct {
					Municipality string `json:"municipality"`
					UF           string `json:"uf"`
				} `json:"locationDetails"`
				Properties []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"properties"`
			} `json:"ads"`
		} `json:"pageProps"`
	} `json:"props"`
}

func ParseOLX(body io.Reader) ([]model.RawListing, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("reading html: %w", err)
	}

	raw := doc.Find("script#__NEXT_DATA__").First().Text()
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("olx payload not found: page structure changed or request was blocked")
	}

	var payload olxPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("decoding olx payload: %w", err)
	}

	ads := payload.Props.PageProps.Ads
	listings := make([]model.RawListing, 0, len(ads))
	for _, ad := range ads {
		location := strings.TrimSpace(ad.LocationDetails.Municipality)
		if ad.LocationDetails.UF != "" {
			location += " - " + ad.LocationDetails.UF
		}
		listings = append(listings, model.RawListing{
			Source:       "olx",
			ExternalID:   ad.ListID.String(),
			URL:          ad.URL,
			Title:        ad.Subject,
			RawText:      ad.Body,
			PriceText:    ad.Price,
			YearText:     olxProperty(ad.Properties, "regdate"),
			KmText:       olxProperty(ad.Properties, "mileage"),
			LocationText: location,
			ImageURL:     ad.Thumbnail,
		})
	}
	return listings, nil
}

func olxProperty(props []struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}, name string) string {
	for _, p := range props {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}
```

- [ ] **Step 6: Rodar o teste e ajustar os caminhos do JSON**

Run: `go test ./internal/source/ -run TestParseOLX -v`

Se `TestParseOLXExtractsListings` falhar com zero anúncios, os caminhos do JSON mudaram. Inspecione a fixture para localizar a lista real:

```bash
python3 -c "
import json, re, sys
html = open('internal/source/testdata/olx-search.html', encoding='utf-8').read()
m = re.search(r'id=\"__NEXT_DATA__\"[^>]*>(.*?)</script>', html, re.S)
data = json.loads(m.group(1))
def walk(node, path=''):
    if isinstance(node, dict):
        for k, v in node.items():
            walk(v, path + '.' + k)
    elif isinstance(node, list) and node and isinstance(node[0], dict):
        print(path, len(node), sorted(node[0].keys())[:12])
walk(data)
"
```

Ajuste os nomes dos campos em `olxPayload` conforme a saída e rode o teste de novo até passar.

Expected ao final: PASS nos dois testes.

- [ ] **Step 7: Commit**

```bash
git add internal/source go.mod go.sum
git commit -m "feat: olx source with fixture-based parser tests"
```

---

### Task 10: Coletor do Mercado Livre

**Files:**
- Create: `internal/source/mercadolivre.go`
- Test: `internal/source/mercadolivre_test.go`, `internal/source/testdata/mercadolivre-search.html`

**Interfaces:**
- Consumes: `source.Source` e `source.defaultUserAgent` da Task 9
- Produces: `source.NewMercadoLivre(client *http.Client, baseURLs []string) *source.MercadoLivre` e `source.ParseMercadoLivre(body io.Reader) ([]model.RawListing, error)`

- [ ] **Step 1: Capturar a fixture real**

```bash
curl -sL -A 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)' \
  'https://lista.mercadolivre.com.br/harley-davidson-street-glide' \
  -o internal/source/testdata/mercadolivre-search.html
grep -c 'ui-search-layout__item' internal/source/testdata/mercadolivre-search.html
```

Expected: número maior que zero. Se der `0`, salve o HTML pelo navegador — a página é renderizada no servidor, então o HTML salvo contém os resultados.

- [ ] **Step 2: Escrever o teste que falha**

`internal/source/mercadolivre_test.go`:

```go
package source

import (
	"os"
	"testing"
)

func TestParseMercadoLivreExtractsListings(t *testing.T) {
	f, err := os.Open("testdata/mercadolivre-search.html")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	listings, err := ParseMercadoLivre(f)
	if err != nil {
		t.Fatalf("ParseMercadoLivre: %v", err)
	}
	if len(listings) == 0 {
		t.Fatal("expected at least one listing from the fixture")
	}

	for i, l := range listings {
		if l.Source != "mercadolivre" {
			t.Errorf("listing %d: Source = %q", i, l.Source)
		}
		if l.ExternalID == "" || l.URL == "" || l.Title == "" {
			t.Errorf("listing %d: incomplete listing %+v", i, l)
		}
	}
}

func TestParseMercadoLivreReturnsErrorWhenEmpty(t *testing.T) {
	if _, err := ParseMercadoLivre(stringReader("<html><body></body></html>")); err == nil {
		t.Fatal("ParseMercadoLivre should return an error when no result container exists")
	}
}
```

- [ ] **Step 3: Rodar o teste e confirmar a falha**

Run: `go test ./internal/source/ -run TestParseMercadoLivre -v`
Expected: FAIL — `ParseMercadoLivre` não existe.

- [ ] **Step 4: Implementar**

`internal/source/mercadolivre.go`:

```go
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andreabreu76/harley-hunter/internal/model"
)

type MercadoLivre struct {
	client   *http.Client
	baseURLs []string
}

func NewMercadoLivre(client *http.Client, baseURLs []string) *MercadoLivre {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &MercadoLivre{client: client, baseURLs: baseURLs}
}

func (m *MercadoLivre) Name() string { return "mercadolivre" }

func (m *MercadoLivre) Fetch(ctx context.Context) ([]model.RawListing, error) {
	var all []model.RawListing
	for _, url := range m.baseURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("building request for %s: %w", url, err)
		}
		req.Header.Set("User-Agent", defaultUserAgent)
		req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")

		resp, err := m.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", url, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("fetching %s: status %d", url, resp.StatusCode)
		}

		listings, err := ParseMercadoLivre(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", url, err)
		}
		all = append(all, listings...)

		select {
		case <-ctx.Done():
			return all, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return all, nil
}

var mlItemID = regexp.MustCompile(`(ML[A-Z]-?\d{6,})`)

func ParseMercadoLivre(body io.Reader) ([]model.RawListing, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, fmt.Errorf("reading html: %w", err)
	}

	items := doc.Find("li.ui-search-layout__item")
	if items.Length() == 0 {
		return nil, fmt.Errorf("mercadolivre result container not found: page structure changed or request was blocked")
	}

	var listings []model.RawListing
	items.Each(func(_ int, s *goquery.Selection) {
		link, _ := s.Find("a.ui-search-link, a.poly-component__title").First().Attr("href")
		title := strings.TrimSpace(s.Find("h2.ui-search-item__title, a.poly-component__title").First().Text())
		price := strings.TrimSpace(s.Find("span.andes-money-amount__fraction").First().Text())
		image, _ := s.Find("img").First().Attr("data-src")
		if image == "" {
			image, _ = s.Find("img").First().Attr("src")
		}
		location := strings.TrimSpace(s.Find(".ui-search-item__location, .poly-component__location").First().Text())

		var attributes []string
		s.Find(".ui-search-card-attributes__attribute, .poly-attributes-list__item").Each(func(_ int, a *goquery.Selection) {
			attributes = append(attributes, strings.TrimSpace(a.Text()))
		})

		if title == "" || link == "" {
			return
		}

		id := mlItemID.FindString(link)
		if id == "" {
			id = link
		}

		listings = append(listings, model.RawListing{
			Source:       "mercadolivre",
			ExternalID:   strings.ReplaceAll(id, "-", ""),
			URL:          link,
			Title:        title,
			RawText:      strings.Join(attributes, " "),
			PriceText:    "R$ " + price,
			YearText:     strings.Join(attributes, " "),
			KmText:       strings.Join(attributes, " "),
			LocationText: location,
			ImageURL:     image,
		})
	})

	return listings, nil
}
```

O Mercado Livre expõe ano e quilometragem como atributos livres do card, sem rótulo confiável, então ambos recebem o texto completo dos atributos e a extração fica com `ParseYear` e `ParseKm`, que já sabem descartar valores implausíveis.

- [ ] **Step 5: Rodar o teste e ajustar seletores**

Run: `go test ./internal/source/ -run TestParseMercadoLivre -v`

Se vier zero itens, inspecione as classes reais da fixture e ajuste os seletores:

```bash
grep -o 'class="[^"]*ui-search-layout[^"]*"' internal/source/testdata/mercadolivre-search.html | sort -u | head
grep -o 'class="[^"]*poly-component[^"]*"' internal/source/testdata/mercadolivre-search.html | sort -u | head
```

Expected ao final: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/source/mercadolivre.go internal/source/mercadolivre_test.go internal/source/testdata
git commit -m "feat: mercado livre source with fixture-based parser tests"
```

---

### Task 11: Orquestração da rodada e saúde das fontes

**Files:**
- Create: `internal/crawl/crawl.go`
- Modify: `cmd/hunter/main.go`, `config/config.yaml`, `internal/config/config.go`
- Test: `internal/crawl/crawl_test.go`

**Interfaces:**
- Consumes: `source.Source` (Task 9), `normalize.Normalize` (Task 6), `match.Evaluate` (Task 7), `store.Store` (Task 8), `config.Config` (Task 1)
- Produces:
  - `crawl.Run(ctx context.Context, sources []source.Source, s *store.Store, cfg config.Config) (crawl.Report, error)`
  - `crawl.Report` com `Results []crawl.SourceResult` e `NewMatches int`
  - `crawl.SourceResult` com `Source string`, `ItemCount int`, `Status string`, `Err error`
  - `crawl.HealthStatus(counts []int) string` devolvendo `"ok"`, `"suspect"` ou `"unknown"`

- [ ] **Step 1: Escrever o teste que falha**

`internal/crawl/crawl_test.go`:

```go
package crawl

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

type fakeSource struct {
	name  string
	items []model.RawListing
	err   error
}

func (f fakeSource) Name() string { return f.name }

func (f fakeSource) Fetch(ctx context.Context) ([]model.RawListing, error) {
	return f.items, f.err
}

func testConfig() config.Config {
	return config.Config{
		Match: config.MatchCriteria{
			Years:              []int{2014, 2015},
			MaybeYears:         []int{2013, 2016},
			MaxPriceCents:      7500000,
			MaybeMaxPriceCents: 8500000,
		},
		Crawl: config.CrawlSettings{TimeoutSeconds: 5, MaxConcurrent: 2, MaxSMSPerRun: 5},
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "crawl.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRunStoresMatchesAndSurvivesFailingSource(t *testing.T) {
	s := openStore(t)
	good := fakeSource{
		name: "good",
		items: []model.RawListing{{
			Source:       "good",
			ExternalID:   "1",
			URL:          "https://example.com/1",
			Title:        "Harley Davidson Street Glide Special",
			PriceText:    "R$ 72.000",
			YearText:     "2015",
			KmText:       "31.000 km",
			LocationText: "Curitiba - PR",
		}},
	}
	broken := fakeSource{name: "broken", err: errors.New("blocked")}

	report, err := Run(context.Background(), []Source{good, broken}, s, testConfig())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 source results, got %d", len(report.Results))
	}
	if report.NewMatches != 1 {
		t.Errorf("NewMatches = %d, want 1", report.NewMatches)
	}

	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 stored match, got %d", len(rows))
	}

	counts, err := s.RecentRunCounts("broken", 5)
	if err != nil {
		t.Fatalf("RecentRunCounts: %v", err)
	}
	if len(counts) != 1 {
		t.Errorf("failing source should still record a run, got %d", len(counts))
	}
}

func TestHealthStatus(t *testing.T) {
	cases := []struct {
		name   string
		counts []int
		want   string
	}{
		{"healthy", []int{8, 7, 9}, "ok"},
		{"two empty runs after productive history", []int{0, 0, 9, 8, 7}, "suspect"},
		{"one empty run is tolerated", []int{0, 9, 8, 7, 6}, "ok"},
		{"low volume source is not suspect", []int{0, 0, 1, 2, 0}, "ok"},
		{"no history", nil, "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HealthStatus(c.counts); got != c.want {
				t.Errorf("HealthStatus(%v) = %q, want %q", c.counts, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar a falha**

Run: `go test ./internal/crawl/ -v`
Expected: FAIL — pacote não compila.

- [ ] **Step 3: Implementar a orquestração**

`internal/crawl/crawl.go`:

```go
package crawl

import (
	"context"
	"sync"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/match"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

const (
	healthWindow       = 5
	emptyRunsForSuspect = 2
	minVolumeForSuspect = 3
)

type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]model.RawListing, error)
}

type SourceResult struct {
	Source    string
	ItemCount int
	Status    string
	Err       error
}

type Report struct {
	Results    []SourceResult
	NewMatches int
}

func Run(ctx context.Context, sources []Source, s *store.Store, cfg config.Config) (Report, error) {
	timeout := time.Duration(cfg.Crawl.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	concurrency := cfg.Crawl.MaxConcurrent
	if concurrency <= 0 {
		concurrency = 4
	}

	type fetched struct {
		result SourceResult
		items  []model.RawListing
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		gathered []fetched
		sem      = make(chan struct{}, concurrency)
	)

	for _, src := range sources {
		wg.Add(1)
		go func(src Source) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sourceCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			started := time.Now()
			items, err := src.Fetch(sourceCtx)
			finished := time.Now()

			result := SourceResult{Source: src.Name(), ItemCount: len(items), Status: "ok", Err: err}
			errMessage := ""
			if err != nil {
				result.Status = "error"
				errMessage = err.Error()
			}
			_ = s.RecordRun(src.Name(), started, finished, len(items), result.Status, errMessage)

			mu.Lock()
			gathered = append(gathered, fetched{result: result, items: items})
			mu.Unlock()
		}(src)
	}
	wg.Wait()

	report := Report{}
	now := time.Now()
	for _, f := range gathered {
		report.Results = append(report.Results, f.result)
		for _, raw := range f.items {
			listing := normalize.Normalize(raw)
			listing.Verdict, listing.VerdictReason = match.Evaluate(listing, cfg.Match)

			res, err := s.Upsert(listing, now)
			if err != nil {
				continue
			}
			if res.IsNew && listing.Verdict == model.VerdictMatch {
				report.NewMatches++
			}
		}
	}
	return report, nil
}

func HealthStatus(counts []int) string {
	if len(counts) == 0 {
		return "unknown"
	}
	window := counts
	if len(window) > healthWindow {
		window = window[:healthWindow]
	}

	leadingEmpty := 0
	for _, c := range window {
		if c != 0 {
			break
		}
		leadingEmpty++
	}
	if leadingEmpty < emptyRunsForSuspect {
		return "ok"
	}

	total, productive := 0, 0
	for _, c := range window {
		if c > 0 {
			total += c
			productive++
		}
	}
	if productive == 0 || total/productive < minVolumeForSuspect {
		return "ok"
	}
	return "suspect"
}
```

O dashboard pede um histórico bem maior que a janela de vazio. A janela decide
há quantas rodadas a fonte está sem trazer nada; o volume histórico decide se
ela já teve movimento. Se as duas lessem os mesmos cinco registros, uma fonte
quebrada limparia o próprio alarme: depois de cinco rodadas zeradas o histórico
visível seria só de zeros, a média de volume cairia abaixo do limiar e o estado
voltaria a `ok` — a fonte pareceria mais saudável quanto mais tempo ficasse
quebrada.

`HealthStatus` só acusa suspeita quando a fonte tinha volume relevante antes: uma fonte que normalmente traz um ou dois anúncios não deve disparar alarme ao passar uma rodada vazia.

- [ ] **Step 4: Rodar o teste e confirmar que passa**

Run: `go test ./internal/crawl/ -v`
Expected: PASS

- [ ] **Step 5: Ligar as fontes reais ao comando `crawl`**

Adicione ao `internal/config/config.go`, dentro de `Config`:

```go
	SourceURLs  map[string][]string `yaml:"source_urls"`
	DevtoolsURL string              `yaml:"devtools_url"`
```

Acrescente ao `config/config.yaml`:

```yaml
devtools_url: http://127.0.0.1:9222

source_urls:
  olx:
    - https://www.olx.com.br/autos-e-pecas/motos/estado-pr/regiao-de-curitiba-e-paranagua?q=harley%20street%20glide
    - https://www.olx.com.br/autos-e-pecas/motos/estado-sp/sao-paulo-e-regiao?q=harley%20street%20glide
    - https://www.olx.com.br/autos-e-pecas/motos/estado-rj/rio-de-janeiro-e-regiao?q=harley%20street%20glide
    - https://www.olx.com.br/autos-e-pecas/motos/estado-pr/regiao-de-curitiba-e-paranagua?q=harley%20road%20glide
    - https://www.olx.com.br/autos-e-pecas/motos/estado-sp/sao-paulo-e-regiao?q=harley%20road%20glide
    - https://www.olx.com.br/autos-e-pecas/motos/estado-rj/rio-de-janeiro-e-regiao?q=harley%20road%20glide
  mercadolivre:
    - https://lista.mercadolivre.com.br/harley-davidson-street-glide
    - https://lista.mercadolivre.com.br/harley-davidson-road-glide
```

Substitua o corpo do `case "crawl"` em `cmd/hunter/main.go`:

```go
	case "crawl":
		db, err := store.Open(cfg.DatabasePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer db.Close()

		var sources []crawl.Source
		for _, name := range cfg.Sources {
			switch name {
			case "olx":
				sources = append(sources, source.NewOLX(nil, cfg.SourceURLs["olx"]))
			case "mercadolivre":
				sources = append(sources, source.NewMercadoLivre(nil, cfg.SourceURLs["mercadolivre"]))
			default:
				fmt.Fprintf(os.Stderr, "unknown source in config: %s\n", name)
				os.Exit(1)
			}
		}

		report, err := crawl.Run(context.Background(), sources, db, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, r := range report.Results {
			if r.Err != nil {
				fmt.Printf("%-14s %s: %v\n", r.Source, r.Status, r.Err)
				continue
			}
			fmt.Printf("%-14s %s: %d items\n", r.Source, r.Status, r.ItemCount)
		}
		fmt.Printf("new matches: %d\n", report.NewMatches)
```

Ajuste os imports de `cmd/hunter/main.go` para incluir `context`, `github.com/andreabreu76/harley-hunter/internal/crawl`, `internal/source` e `internal/store`.

- [ ] **Step 6: Executar uma coleta real**

Run: `go run ./cmd/hunter crawl`
Expected: uma linha por fonte com a contagem de itens e a contagem de novos matches. Erro de rede em uma fonte não deve impedir a outra de reportar.

- [ ] **Step 7: Commit**

```bash
git add internal/crawl cmd/hunter internal/config config/config.yaml
git commit -m "feat: crawl orchestration with per-source isolation and health tracking"
```

---

### Task 12: Notificação por SMS

**Files:**
- Create: `internal/format/format.go`, `internal/notify/notify.go`, `internal/notify/twilio.go`
- Modify: `internal/crawl/crawl.go`, `cmd/hunter/main.go`
- Test: `internal/format/format_test.go`, `internal/notify/twilio_test.go`, `internal/crawl/notify_test.go`

**Interfaces:**
- Consumes: `store.Row` (Task 8), `crawl.Report` (Task 11)
- Produces:
  - `notify.Notifier` interface com `Send(ctx context.Context, message string) error`
  - `notify.NewTwilioFromEnv() (*notify.Twilio, error)` lendo `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_PHONE_NUMBER` (com `TWILIO_FROM` como alternativa) e `ALERT_TO`
  - `notify.FormatAlert(r store.Row) string`
  - `format.Thousands(value int64) string`, consumida pela Task 13
  - `crawl.Notify(ctx context.Context, s *store.Store, n notify.Notifier, limit int) (sent int, err error)`

- [ ] **Step 0: Criar o pacote de formatação compartilhado**

Separação de milhares é usada pelo SMS e pelo dashboard. Ela nasce em um pacote
próprio para não existir em duas cópias.

`internal/format/format_test.go`:

```go
package format

import "testing"

func TestThousands(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.000"},
		{72000, "72.000"},
		{4000, "4.000"},
		{1234567, "1.234.567"},
	}
	for _, c := range cases {
		if got := Thousands(c.in); got != c.want {
			t.Errorf("Thousands(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

`internal/format/format.go`:

```go
package format

import (
	"strconv"
	"strings"
)

func Thousands(value int64) string {
	digits := strconv.FormatInt(value, 10)
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)
	return strings.Join(parts, ".")
}
```

Run: `go test ./internal/format/ -v`
Expected: PASS

- [ ] **Step 1: Escrever o teste que falha**

`internal/notify/twilio_test.go`:

```go
package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	for _, want := range []string{"Street Glide", "2015", "72.000", "curitiba", "https://olx.com.br/abc"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q is missing %q", msg, want)
		}
	}
	if len(msg) > 320 {
		t.Errorf("message is %d chars, which spans too many SMS segments", len(msg))
	}
}

func TestTwilioSendPostsToAPI(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotBody = r.Form.Get("Body")
		if r.Form.Get("To") != "+5541999999999" {
			t.Errorf("To = %q", r.Form.Get("To"))
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"SM123"}`))
	}))
	defer server.Close()

	tw := &Twilio{
		endpoint: server.URL,
		from:     "+15550001111",
		to:       "+5541999999999",
		client:   &http.Client{Timeout: 5 * time.Second},
	}
	if err := tw.Send(context.Background(), "teste"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotBody != "teste" {
		t.Errorf("Body = %q, want %q", gotBody, "teste")
	}
}

func TestTwilioSendReportsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"authenticate"}`))
	}))
	defer server.Close()

	tw := &Twilio{endpoint: server.URL, from: "+1", to: "+2", client: server.Client()}
	if err := tw.Send(context.Background(), "teste"); err == nil {
		t.Fatal("Send should return an error on a non-2xx response")
	}
}
```

`internal/crawl/notify_test.go`:

```go
package crawl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

type recordingNotifier struct {
	messages []string
	failAt   int
}

func (r *recordingNotifier) Send(ctx context.Context, message string) error {
	if r.failAt > 0 && len(r.messages) == r.failAt-1 {
		return errors.New("twilio unavailable")
	}
	r.messages = append(r.messages, message)
	return nil
}

func storeWithMatches(t *testing.T, count int) *store.Store {
	t.Helper()
	s := openStore(t)
	year := 2015
	cents := int64(7200000)
	for i := 0; i < count; i++ {
		l := model.Listing{
			Source:     "olx",
			ExternalID: string(rune('a' + i)),
			URL:        "https://example.com",
			Title:      "Harley Street Glide",
			Bike:       model.BikeStreetGlide,
			Year:       &year,
			PriceCents: &cents,
			City:       "curitiba",
			State:      "PR",
			Verdict:    model.VerdictMatch,
		}
		if _, err := s.Upsert(l, time.Now()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	return s
}

func TestNotifyRespectsPerRunCap(t *testing.T) {
	s := storeWithMatches(t, 12)
	n := &recordingNotifier{}

	sent, err := Notify(context.Background(), s, n, 5)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 5 {
		t.Errorf("sent = %d, want 5", sent)
	}
	if len(n.messages) != 5 {
		t.Errorf("delivered %d messages, want 5", len(n.messages))
	}
}

func TestNotifyDoesNotMarkOnFailure(t *testing.T) {
	s := storeWithMatches(t, 2)
	n := &recordingNotifier{failAt: 1}

	if _, err := Notify(context.Background(), s, n, 5); err == nil {
		t.Fatal("Notify should surface the delivery error")
	}

	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("failed delivery must leave rows pending, got %d", len(pending))
	}
}
```

Adicione o import de `store` no topo de `internal/crawl/notify_test.go`:

```go
	"github.com/andreabreu76/harley-hunter/internal/store"
```

- [ ] **Step 2: Rodar os testes e confirmar a falha**

Run: `go test ./internal/notify/ ./internal/crawl/ -v`
Expected: FAIL — `Twilio` e `Notify` não existem.

- [ ] **Step 3: Implementar o notificador**

`internal/notify/notify.go`:

```go
package notify

import "context"

type Notifier interface {
	Send(ctx context.Context, message string) error
}
```

`internal/notify/twilio.go`:

```go
package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/format"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

type Twilio struct {
	endpoint string
	sid      string
	token    string
	from     string
	to       string
	client   *http.Client
}

func NewTwilioFromEnv() (*Twilio, error) {
	sid := os.Getenv("TWILIO_ACCOUNT_SID")
	token := os.Getenv("TWILIO_AUTH_TOKEN")
	from := os.Getenv("TWILIO_PHONE_NUMBER")
	if from == "" {
		from = os.Getenv("TWILIO_FROM")
	}
	to := os.Getenv("ALERT_TO")

	var missing []string
	for name, value := range map[string]string{
		"TWILIO_ACCOUNT_SID": sid,
		"TWILIO_AUTH_TOKEN":  token,
		"TWILIO_PHONE_NUMBER": from,
		"ALERT_TO":           to,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing environment variables: %s", strings.Join(missing, ", "))
	}

	return &Twilio{
		endpoint: fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", sid),
		sid:      sid,
		token:    token,
		from:     from,
		to:       to,
		client:   &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func (t *Twilio) Send(ctx context.Context, message string) error {
	form := url.Values{}
	form.Set("From", t.from)
	form.Set("To", t.to)
	form.Set("Body", message)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("building twilio request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if t.sid != "" {
		req.SetBasicAuth(t.sid, t.token)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("calling twilio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("twilio returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func FormatAlert(r store.Row) string {
	price := "preço não informado"
	if r.PriceCents != nil {
		price = "R$ " + format.Thousands(*r.PriceCents/100)
	}
	year := ""
	if r.Year != nil {
		year = fmt.Sprintf(" %d", *r.Year)
	}
	location := r.City
	if r.State != "" {
		location += "/" + r.State
	}
	return fmt.Sprintf("%s%s - %s - %s [%s] %s",
		strings.TrimSpace(r.Title), year, price, location, r.Source, r.URL)
}
```

- [ ] **Step 4: Implementar o envio na rodada**

Acrescente a `internal/crawl/crawl.go`:

```go
func Notify(ctx context.Context, s *store.Store, n notify.Notifier, limit int) (int, error) {
	if limit <= 0 {
		limit = 5
	}
	pending, err := s.PendingNotifications(limit)
	if err != nil {
		return 0, err
	}

	sent := 0
	seen := make(map[string]bool, len(pending))
	for _, row := range pending {
		if row.Fingerprint != "" && row.Km != nil && seen[row.Fingerprint] {
			if err := s.MarkNotified(row.ID); err != nil {
				return sent, err
			}
			continue
		}
		if err := n.Send(ctx, notify.FormatAlert(row)); err != nil {
			return sent, fmt.Errorf("sending alert for listing %d: %w", row.ID, err)
		}
		if err := s.MarkNotified(row.ID); err != nil {
			return sent, err
		}
		if row.Fingerprint != "" && row.Km != nil {
			seen[row.Fingerprint] = true
		}
		sent++
	}
	return sent, nil
}
```

Adicione `fmt` e `github.com/andreabreu76/harley-hunter/internal/notify` aos imports do pacote.

`MarkNotified` só roda depois de o envio ter sucesso — é isso que garante o reenvio na rodada seguinte quando a Twilio falha.

O envio deduplica por impressão digital dentro da rodada, mas SOMENTE quando a
quilometragem existe. A impressão usa modelo, ano, faixa de km e cidade; sem km
a faixa vira `?` e ela deixa de distinguir motos diferentes — numa coleta real,
duas Street Glide 2014 de Curitiba (R$ 72.000 e R$ 75.000, ambas sem km)
compartilharam a impressão, e a deduplicação cega silenciaria uma delas para
sempre. Com km presente ela funciona como deve: a mesma FLHX 2014 com 90.195 km
apareceu em OLX e Mercado Livre com impressão idêntica, e um só SMS basta. Na
dúvida, dois SMS para a mesma moto é melhor que zero para uma moto real.

- [ ] **Step 5: Ligar ao comando `crawl`**

Acrescente ao final do `case "crawl"` em `cmd/hunter/main.go`:

```go
		notifier, err := notify.NewTwilioFromEnv()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sms disabled: %v\n", err)
			return
		}
		sent, err := crawl.Notify(context.Background(), db, notifier, cfg.Crawl.MaxSMSPerRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sms delivery failed after %d messages: %v\n", sent, err)
			return
		}
		fmt.Printf("sms sent: %d\n", sent)
```

Ausência de credencial degrada para aviso, não para falha: a coleta já foi gravada e o dashboard continua útil.

- [ ] **Step 6: Rodar os testes e confirmar que passam**

Run: `go test ./... -v`
Expected: PASS em todos os pacotes.

- [ ] **Step 7: Teste real de envio**

```bash
set -a && source .env && set +a
go run ./cmd/hunter crawl
```

Expected: `sms sent: N` e chegada da mensagem no aparelho.

As credenciais já existem em `.env` na raiz do projeto, que o `.gitignore` cobre na primeira linha. O arquivo traz `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_PHONE_NUMBER` e `TWILIO_WHATSAPP_FROM`. Falta `ALERT_TO` com o número de destino no formato E.164 (`+5541999999999`); sem ele o comando degrada para `sms disabled` e a coleta segue normalmente.

- [ ] **Step 8: Commit**

```bash
git add internal/format internal/notify internal/crawl cmd/hunter
git commit -m "feat: sms alerts with per-run cap and retry on delivery failure"
```

---

### Task 13: Dashboard

**Files:**
- Create: `internal/web/server.go`, `internal/web/templates/layout.html`, `internal/web/templates/list.html`, `internal/web/templates/detail.html`, `internal/web/templates/health.html`
- Modify: `cmd/hunter/main.go`
- Test: `internal/web/server_test.go`

**Interfaces:**
- Consumes: `store.Store`, `store.Row`, `store.PricePoint` (Task 8); `crawl.HealthStatus` (Task 11); `format.Thousands` (Task 12)
- Produces: `web.NewServer(s *store.Store, sources []string) http.Handler`

- [ ] **Step 1: Escrever o teste que falha**

`internal/web/server_test.go`:

```go
package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

func seededStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	year := 2015
	cents := int64(7200000)
	matched := model.Listing{
		Source: "olx", ExternalID: "m1", URL: "https://example.com/m1",
		Title: "Harley Street Glide Special", Bike: model.BikeStreetGlide,
		Year: &year, PriceCents: &cents, City: "curitiba", State: "PR",
		Verdict: model.VerdictMatch,
	}
	if _, err := s.Upsert(matched, time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	maybeCents := int64(8100000)
	maybe := matched
	maybe.ExternalID = "m2"
	maybe.Title = "Harley Road Glide"
	maybe.PriceCents = &maybeCents
	maybe.Verdict = model.VerdictMaybe
	if _, err := s.Upsert(maybe, time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	return s
}

func TestRoutesRenderExpectedListings(t *testing.T) {
	srv := NewServer(seededStore(t), []string{"olx"})

	cases := []struct {
		path        string
		wantPresent string
		wantAbsent  string
	}{
		{"/", "Street Glide Special", "Road Glide"},
		{"/maybe", "Road Glide", "Street Glide Special"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, c.wantPresent) {
				t.Errorf("%s should contain %q", c.path, c.wantPresent)
			}
			if strings.Contains(body, c.wantAbsent) {
				t.Errorf("%s should not contain %q", c.path, c.wantAbsent)
			}
		})
	}
}

func TestHealthRouteRenders(t *testing.T) {
	srv := NewServer(seededStore(t), []string{"olx"})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "olx") {
		t.Error("health page should list the configured source")
	}
}

func TestSetUserStateUpdatesRow(t *testing.T) {
	s := seededStore(t)
	srv := NewServer(s, []string{"olx"})

	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil || len(rows) == 0 {
		t.Fatalf("seed failed: %v", err)
	}
	id := rows[0].ID

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/listing/"+strconv.FormatInt(id, 10)+"/state?value=contacted", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	row, _, err := s.GetRow(id)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.UserState != "contacted" {
		t.Errorf("UserState = %q, want contacted", row.UserState)
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar a falha**

Run: `go test ./internal/web/ -v`
Expected: FAIL — pacote não compila.

- [ ] **Step 3: Criar os templates**

`internal/web/templates/layout.html`:

```html
{{define "layout"}}
<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Harley Hunter</title>
<style>
:root { color-scheme: light dark; }
body { font-family: -apple-system, system-ui, sans-serif; margin: 0; padding: 1.5rem; max-width: 60rem; }
nav a { margin-right: 1rem; text-decoration: none; }
.card { display: flex; gap: 1rem; border: 1px solid #8884; border-radius: 8px; padding: 1rem; margin-bottom: 1rem; }
.card img { width: 140px; height: 105px; object-fit: cover; border-radius: 4px; }
.price { font-size: 1.25rem; font-weight: 600; }
.drop { color: #1a7f37; font-weight: 600; }
.meta { color: #888; font-size: 0.9rem; }
.suspect { color: #b42318; font-weight: 600; }
table { border-collapse: collapse; width: 100%; }
th, td { text-align: left; padding: 0.5rem; border-bottom: 1px solid #8884; }
</style>
</head>
<body>
<nav>
  <a href="/">Match</a>
  <a href="/maybe">Talvez</a>
  <a href="/rejected">Descartados</a>
  <a href="/health">Saúde</a>
</nav>
<h1>{{.Title}}</h1>
{{template "content" .}}
</body>
</html>
{{end}}
```

`internal/web/templates/list.html`:

```html
{{define "content"}}
{{if not .Rows}}<p>Nenhum anúncio nesta categoria.</p>{{end}}
{{range .Rows}}
<div class="card">
  {{if .ImageURL}}<img src="{{.ImageURL}}" alt="">{{end}}
  <div>
    <a href="/listing/{{.ID}}"><strong>{{.Title}}</strong></a>
    <div class="price">
      {{if .PriceCents}}R$ {{money .PriceCents}}{{else}}preço não informado{{end}}
      {{with priceDrop .}}<span class="drop">{{.}}</span>{{end}}
    </div>
    <div class="meta">
      {{if .Year}}{{.Year}}{{end}}
      {{if .Km}} · {{.Km}} km{{end}}
      · {{.City}}{{if .State}}/{{.State}}{{end}}
      · {{.Source}}
      · {{.UserState}}
    </div>
    <div class="meta"><a href="{{.URL}}" target="_blank" rel="noreferrer">abrir anúncio</a></div>
  </div>
</div>
{{end}}
{{end}}
```

`internal/web/templates/detail.html`:

```html
{{define "content"}}
<div class="card">
  {{if .Row.ImageURL}}<img src="{{.Row.ImageURL}}" alt="">{{end}}
  <div>
    <div class="price">{{if .Row.PriceCents}}R$ {{money .Row.PriceCents}}{{else}}preço não informado{{end}}</div>
    <div class="meta">
      {{if .Row.Year}}{{.Row.Year}}{{end}}
      {{if .Row.Km}} · {{.Row.Km}} km{{end}}
      · {{.Row.City}}{{if .Row.State}}/{{.Row.State}}{{end}}
      · {{.Row.Source}}
    </div>
    <p><a href="{{.Row.URL}}" target="_blank" rel="noreferrer">abrir anúncio original</a></p>
    <form method="post" action="/listing/{{.Row.ID}}/state?value=contacted">
      <button type="submit">marcar como contatado</button>
    </form>
    <form method="post" action="/listing/{{.Row.ID}}/state?value=dismissed">
      <button type="submit">descartar</button>
    </form>
  </div>
</div>

<h2>Histórico de preço</h2>
<table>
  <tr><th>Data</th><th>Preço</th></tr>
  {{range .Points}}
  <tr><td>{{.ObservedAt.Format "02/01/2006 15:04"}}</td><td>R$ {{moneyCents .PriceCents}}</td></tr>
  {{end}}
</table>

<h2>Texto do anúncio</h2>
<p class="meta">{{.Row.Title}}</p>
{{end}}
```

`internal/web/templates/health.html`:

```html
{{define "content"}}
<table>
  <tr><th>Fonte</th><th>Estado</th><th>Últimas coletas</th></tr>
  {{range .Sources}}
  <tr>
    <td>{{.Name}}</td>
    <td {{if eq .Status "suspect"}}class="suspect"{{end}}>{{.Status}}</td>
    <td>{{.Counts}}</td>
  </tr>
  {{end}}
</table>
{{end}}
```

- [ ] **Step 4: Implementar o servidor**

`internal/web/server.go`:

```go
package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/andreabreu76/harley-hunter/internal/crawl"
	"github.com/andreabreu76/harley-hunter/internal/format"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

type server struct {
	store     *store.Store
	sources   []string
	listTmpl  *template.Template
	detailTmpl *template.Template
	healthTmpl *template.Template
}

const healthHistoryRuns = 30

var regionPriority = map[string]int{"RJ": 0, "SP": 1, "PR": 2}

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
```

As listas saem ordenadas por prioridade de região — Rio de Janeiro primeiro,
depois São Paulo, depois Curitiba — preservando a ordem por data dentro de cada
uma. O Rio é a região de maior interesse e a de estoque mais escasso: numa
coleta real, todas as Street Glide cariocas eram de 2017 em diante, fora do
alvo. Quando um anúncio no alvo finalmente aparecer por lá, ele precisa estar no
topo, não perdido entre os de Curitiba. A ordenação é estável, então nada além
da região muda de posição.

- [ ] **Step 5: Ligar ao comando `serve`**

Substitua o `case "serve"` em `cmd/hunter/main.go`:

```go
	case "serve":
		db, err := store.Open(cfg.DatabasePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer db.Close()

		addr := "127.0.0.1:8080"
		fmt.Printf("dashboard: http://%s\n", addr)
		if err := http.ListenAndServe(addr, web.NewServer(db, cfg.Sources)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
```

Adicione `net/http` e `internal/web` aos imports.

- [ ] **Step 6: Rodar os testes e abrir o dashboard**

Run: `go test ./... && go run ./cmd/hunter serve`
Expected: testes passam e `http://127.0.0.1:8080` mostra as abas com os anúncios coletados.

- [ ] **Step 7: Commit**

```bash
git add internal/web cmd/hunter
git commit -m "feat: local dashboard with match, maybe, rejected and health views"
```

---

### Task 14: Agendamento no launchd

**Files:**
- Create: `deploy/com.andreabreu.harleyhunter.plist`, `deploy/hunter-crawl.sh`, `README.md`
- Test: verificação manual documentada abaixo

**Interfaces:**
- Consumes: binário `hunter` compilado
- Produces: agendamento periódico da coleta

- [ ] **Step 1: Compilar e instalar o binário**

```bash
go build -o ~/bin/hunter ./cmd/hunter
~/bin/hunter -config ~/src/github.com/andreabreu76/harley-hunter/config/config.yaml crawl
```

Expected: a coleta roda e imprime o resumo por fonte.

- [ ] **Step 2: Criar o script de execução**

`deploy/hunter-crawl.sh`:

```bash
#!/bin/bash
set -euo pipefail

PROJECT_DIR="$HOME/src/github.com/andreabreu76/harley-hunter"
ENV_FILE="$PROJECT_DIR/.env"
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
CHROME_PROFILE="$HOME/Library/Application Support/harley-hunter-chrome"
DEVTOOLS_PORT=9222

if [ -f "$ENV_FILE" ]; then
  set -a
  source "$ENV_FILE"
  set +a
fi

if ! curl -sf -o /dev/null "http://127.0.0.1:${DEVTOOLS_PORT}/json/version"; then
  "$CHROME" \
    --remote-debugging-port="${DEVTOOLS_PORT}" \
    --user-data-dir="$CHROME_PROFILE" \
    --no-first-run \
    --no-default-browser-check \
    --window-position=-32000,-32000 \
    --window-size=1280,900 \
    about:blank >/dev/null 2>&1 &

  for _ in $(seq 1 30); do
    curl -sf -o /dev/null "http://127.0.0.1:${DEVTOOLS_PORT}/json/version" && break
    sleep 1
  done
fi

cd "$PROJECT_DIR"
exec "$HOME/bin/hunter" -config config/config.yaml crawl
```

O Chrome da coleta é uma instância DEDICADA, com `--user-data-dir` próprio, e
nunca o navegador de uso diário. São duas razões independentes. A primeira é
segurança de sessão: a coleta abre e fecha abas por conta própria, e durante o
desenvolvimento houve um episódio, não reproduzido, em que todas as abas do
navegador sumiram — não vale arriscar as abas de trabalho de alguém por causa de
um robô que roda a cada duas horas. A segunda é que a fase 2 vai precisar de uma
sessão logada em Instagram e Facebook, e essa sessão deve viver num perfil
separado do pessoal.

A janela é posicionada fora da tela em vez de rodar em modo headless: o
Cloudflare bloqueia headless, que foi justamente o que motivou usar o navegador
real. O script não encerra o Chrome ao terminar — deixá-lo vivo evita pagar a
inicialização a cada coleta, e o `curl` no início reaproveita a instância que já
estiver de pé.

```bash
chmod +x deploy/hunter-crawl.sh
```

O script carrega as credenciais de um arquivo fora do repositório, porque o `launchd` não herda o ambiente do seu shell.

- [ ] **Step 3: Criar o agendamento**

`deploy/com.andreabreu.harleyhunter.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.andreabreu.harleyhunter</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>/Users/andreabreu/src/github.com/andreabreu76/harley-hunter/deploy/hunter-crawl.sh</string>
    </array>
    <key>StartInterval</key>
    <integer>7200</integer>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/andreabreu/Library/Logs/harley-hunter.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/andreabreu/Library/Logs/harley-hunter.error.log</string>
</dict>
</plist>
```

- [ ] **Step 4: Carregar e verificar**

```bash
cp deploy/com.andreabreu.harleyhunter.plist ~/Library/LaunchAgents/
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.andreabreu.harleyhunter.plist
launchctl list | grep harleyhunter
tail -20 ~/Library/Logs/harley-hunter.log
```

Expected: `launchctl list` mostra o serviço e o log traz o resumo da coleta. Para desligar:
`launchctl bootout gui/$(id -u)/com.andreabreu.harleyhunter`.

- [ ] **Step 5: Escrever o README**

`README.md` deve cobrir, em texto curto: o que o projeto faz, como rodar `crawl` e `serve`, quais variáveis de ambiente o SMS exige e onde ficam os logs. Sem seção de arquitetura — ela está no spec.

- [ ] **Step 6: Commit**

```bash
git add deploy README.md
git commit -m "feat: launchd scheduling and usage documentation"
```

---

## Verificação final da fase

- [ ] `go test ./...` passa inteiro
- [ ] `go vet ./...` limpo
- [ ] `hunter crawl` grava anúncios reais vindos de OLX e Mercado Livre
- [ ] `hunter serve` mostra as quatro abas com dados reais
- [ ] Um SMS real chegou ao aparelho
- [ ] `launchctl list` mostra o agendamento ativo

## Fase 2 (plano separado)

Escrito depois que esta fase estiver rodando: coletores de Webmotors e iCarros sobre a mesma interface `Source`; coletores de Instagram e Facebook Marketplace via `chromedp` acoplado ao Chrome real, com filtro prévio de sinal de venda, interrupção diante de checkpoint e limite de volume por rodada; marcação de reanúncio por impressão digital no dashboard; e expiração para `status = 'gone'` após três rodadas sem reaparecer.
