# Harley Hunter — Plano de Implementação (Fase 2: novas fontes)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ampliar a cobertura de fontes — Webmotors, iCarros, Instagram e Facebook Marketplace — e completar o ciclo de vida do anúncio com expiração e marcação de reanúncio.

**Architecture:** Cada fonte nova implementa a interface `source.Source` existente sobre o seam `PageFetcher`. O pipeline (normalize → match → store → notify) não muda. A fase acrescenta expiração (`status = gone`) e reanúncio visível no dashboard.

**Tech Stack:** o da fase 1, sem adições: Go 1.26, chromedp sobre Chrome dedicado, SQLite via modernc, goquery.

**Spec:** `docs/superpowers/specs/2026-08-09-harley-hunter-design.md` (seções de fontes Meta, dedup/histórico e riscos assumidos).

## Método desta fase (diferente da fase 1, com motivo)

A fase 1 provou que markup prescrito de véspera nasce errado: cinco dos sete
seletores do Mercado Livre estavam errados, a OLX tinha trocado `__NEXT_DATA__`
por flight payload, e as URLs planejadas estavam mortas. O que funcionou foi o
protocolo fixture-primeiro. Esta fase o adota como regra:

1. **Fixture antes de parser.** Baixar a página real logo no Step 1. Se o
   download falhar ou vier bloqueio, reportar NEEDS_CONTEXT com o que veio —
   nunca inventar HTML nem escrever parser contra estrutura imaginada.
2. **Transporte por evidência.** Testar HTTP simples primeiro; só usar o
   `BrowserFetcher` se a fonte bloquear de fato. Reportar a evidência.
3. **Falha barulhenta nos dois níveis.** Contêiner ausente → erro. Contêiner
   presente e zero anúncios extraídos → erro. Card individual malformado →
   pulado em silêncio. (Padrão dos coletores da fase 1; copiar a semântica.)
4. **Sanity check obrigatório.** Rodar os anúncios parseados por
   `normalize` + `match` e reportar contagens por eixo. Foi assim que a fase 1
   achou "São Paulo Zona Sul" rejeitando a capital inteira.
5. **Todo texto extraído é hipótese.** Formatos reais de preço, local e ano
   surpreenderam em TODAS as tasks da fase 1. Na dúvida, probe com corpus real
   e reporte antes de mudar comportamento especificado.

## Global Constraints (idênticas à fase 1)

- Go module `github.com/andreabreu76/harley-hunter`, Go 1.26, sem cgo.
- Código e identificadores em inglês. Sem comentários no código.
- Commits sem menção a IA, sem `Co-Authored-By`, sem link de sessão.
- Testes nunca acessam a rede; fixtures em `testdata/`.
- Valores ausentes são ponteiros nulos; dinheiro é int64 em centavos.
- `.env` nunca é versionado nem ecoado. Stage por caminho explícito.
- Nas fontes Meta: diante de tela de login, checkpoint ou captcha, ABORTAR a
  fonte imediatamente com erro descritivo — nunca retentar na mesma rodada.
  Volume baixo e intervalos aleatórios entre requisições (o spec assume o risco
  de bloqueio de forma controlada, não cega).

## Pré-requisito operacional (ação do dono, antes das Tasks 4 e 5)

O perfil dedicado do Chrome (`~/Library/Application Support/harley-hunter-chrome`)
precisa de sessão logada no Instagram e no Facebook. Login manual, uma vez, com
a conta que o dono decidir arriscar. As Tasks 1–3 não dependem disso.

---

### Task 1: Coletor Webmotors

**Files:**
- Create: `internal/source/webmotors.go`, `internal/source/webmotors_test.go`, fixtures em `internal/source/testdata/`
- Modify: `config/config.yaml` (URLs de busca em RJ/SP/PR para Street Glide e Road Glide), `cmd/hunter/main.go` (case `webmotors`)

**Interfaces:**
- Consumes: `source.Source`, `source.PageFetcher`, `model.RawListing`
- Produces: `source.NewWebmotors(fetcher PageFetcher, urls []string) *Webmotors`, `source.ParseWebmotors(body io.Reader) ([]model.RawListing, error)`

**Contrato de teste (mínimo):** fixture real com resultados; fixture/caso de
busca vazia legítima distinguida de página quebrada; teste de card individual
malformado pulado; teste de rodada-com-cards-e-zero-extraídos → erro; IDs
únicos; sanity check por normalize+match reportado.

A Webmotors historicamente expõe um endpoint JSON de busca — se a fixture
confirmar, parsear o JSON direto é mais estável que HTML. Verificar, não
assumir.

- [ ] Fixture real baixada e verificada (resultados presentes, não bloqueio)
- [ ] Transporte decidido por evidência e reportado
- [ ] TDD: testes vermelhos → parser → verdes
- [ ] Sanity check normalize+match com contagens no relatório
- [ ] Config e main.go ligados; commit

### Task 2: Coletor iCarros

Mesma estrutura, mesmo contrato de teste e mesmos passos da Task 1, para
`icarros`. Files/Interfaces análogos (`NewICarros`, `ParseICarros`).

- [ ] Fixture real; transporte por evidência; TDD; sanity check; ligado; commit

### Task 3: Extrair o laço de coleta compartilhado

O laço `Fetch`/`fetchOne` está duplicado verbatim entre OLX e Mercado Livre
(deferido na fase 1 até existir a terceira fonte — ela agora existe).

**Files:**
- Create: `internal/source/fetchloop.go` (+ teste)
- Modify: `olx.go`, `mercadolivre.go`, `webmotors.go`, `icarros.go` para consumi-lo

**Interfaces:**
- Produces: `source.fetchPages(ctx, fetcher, urls, delay, parse func(io.Reader) ([]model.RawListing, error)) ([]model.RawListing, error)` — preservando: checagem de ctx antes da primeira busca, delay entre buscas, resultados parciais retornados junto com o erro.

**Regra:** refactor puro — os testes existentes dos quatro coletores passam sem
alteração de expectativa. Qualquer mudança de comportamento é defeito.

- [ ] TDD sobre o helper; quatro coletores migrados; suíte integral verde; commit

### Task 4: Coletor Instagram (hashtags)

**Files:**
- Create: `internal/source/meta/instagram.go` (+ teste + fixtures)
- Modify: `config/config.yaml` (`source_urls.instagram` com URLs de hashtag), `cmd/hunter/main.go`

**Interfaces:**
- Consumes: `source.PageFetcher` (sempre browser — Instagram não tem caminho HTTP)
- Produces: `meta.NewInstagram(fetcher, urls)`, `meta.ParseInstagram(...)` conforme o formato que a fixture real revelar (HTML renderizado via CDP)

**Hashtags iniciais (config, dono pode editar):** `#streetglide`,
`#streetglidebrasil`, `#roadglide`, `#harleyusada`, `#harleydavidsonbrasil`.

**Regras específicas:**
- Filtro prévio de sinal de venda ANTES do matcher: legenda precisa conter
  preço ou termo de venda (`vendo`, `à venda`, `disponível`, `aceito troca`).
  Post sem sinal é descartado antes de virar RawListing. (Spec: sem isso o
  dashboard fica inutilizável.)
- `RawListing` de Instagram carrega só `RawText` (legenda), `URL` (permalink),
  `ExternalID` (shortcode) e `ImageURL` — os parsers de texto livre da fase 1
  extraem o resto; foram endurecidos exatamente para isso.
- Tela de login/checkpoint → abortar com erro nomeando a causa. O painel de
  saúde já exibe a falha por fonte.
- Máximo de posts por hashtag por rodada: 12 (volume baixo deliberado).
- Delay aleatório 8–20s entre hashtags.

**Contrato de teste:** fixture real de página de hashtag; posts sem sinal de
venda filtrados; checkpoint → erro; zero posts com página válida ≠ erro (feed
de hashtag pode ser legitimamente pobre — distinguir pela presença do contêiner
de grid); sanity check.

- [ ] Sessão logada confirmada no perfil dedicado (pré-requisito do dono)
- [ ] Fixture real via CDP; TDD; filtro de venda; abort de checkpoint; sanity check; commit

### Task 5: Coletor Facebook Marketplace

**Files:**
- Create: `internal/source/meta/marketplace.go` (+ teste + fixtures)
- Modify: `config/config.yaml` (URLs de busca do Marketplace por cidade/raio), `cmd/hunter/main.go`

**Interfaces:** análogas à Task 4 (`meta.NewMarketplace`, `meta.ParseMarketplace`).

**Regras específicas:** mesmas da Task 4 (abort em login/checkpoint, volume
baixo, delays aleatórios). Marketplace tem busca por cidade com raio — usar as
três regiões-alvo. Cards do Marketplace têm preço e local estruturados no HTML
renderizado; título e descrição vão para os parsers de texto livre.

**Contrato de teste:** o mesmo da Task 4, adaptado.

- [ ] Fixture real via CDP; TDD; abort de checkpoint; sanity check; commit

### Task 6: Ciclo de vida — expiração e reanúncio visível

**Files:**
- Modify: `internal/store/` (query de expiração), `internal/crawl/crawl.go` (passo pós-rodada), `internal/web/` (badge de reanúncio no card e no detalhe)

**Interfaces:**
- Produces: `(*Store).ExpireUnseen(since time.Time, rounds int) (int, error)` — marca `status = gone` em anúncios ativos não vistos há N rodadas; `(*Store).RepostsOf(fingerprint string, excludeID int64) ([]Row, error)` para o dashboard.

**Regras:**
- `gone` após 3 rodadas sem reaparecer (spec). Anúncio `gone` que reaparece na
  mesma `(source, external_id)` volta a `active` pelo Upsert existente.
- Reanúncio: no card e no detalhe, quando outra linha ativa ou gone compartilha
  o fingerprint (com km presente — regra da fase 1), badge "possível reanúncio"
  com link cruzado. Reanúncio mais barato que o anterior ganha destaque
  (`--laranja-hd`): é o sinal de vendedor com pressa que o spec quer à vista.
- Cobrir com teste o filtro `status = 'active'` de `PendingNotifications`
  (deferido da fase 1 — agora o estado `gone` existe de verdade e o teste é
  honesto).

- [ ] TDD: expiração; reaparecimento reativa; badge; teste do filtro active; commit

---

## Verificação final da fase

- [ ] `go test ./... -race` integral verde; vet e gofmt limpos
- [ ] Coleta real com as 6 fontes ativas: contagens por fonte no relatório
- [ ] Painel de saúde exibindo as 6 fontes, com as da Meta abortando de forma
      legível quando sem sessão
- [ ] Review final da branch antes do merge

## Riscos aceitos (herdados do spec)

Instagram e Marketplace violam ToS da Meta; risco de bloqueio da conta usada é
decisão registrada do dono. Mitigações: navegador real com perfil dedicado,
volume baixo, delays aleatórios, abort imediato em checkpoint. Fontes da Meta
são frágeis por natureza — o config permite desligá-las sem recompilar.
