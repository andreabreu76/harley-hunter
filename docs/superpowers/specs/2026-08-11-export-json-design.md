# Export JSON do banco

## Problema

O dono quer entregar o conteúdo do banco a um agente para que ele analise os
match e os talvez — comparar preço com FIPE, notar reanúncio, decidir onde vale
ligar. Hoje o único acesso é o dashboard HTML, que serve olho humano e não
consumo por programa.

O que o agente precisa não é um dump de colunas. Um agente que recebe
`price_cents: 6890000` sem a referência FIPE ao lado não sabe se o preço é bom,
e teria que cruzar `fipe_refs` por conta própria. O JSON leva o julgamento que o
dashboard já calcula.

## Escopo

Um endpoint novo no servidor que já roda (`hunter serve`, 127.0.0.1:8080).
Nada de autenticação — o bind é local, como o resto do dashboard.

Fora de escopo: escrita pelo endpoint, paginação, formato CSV, exportação para
arquivo em disco.

## Rota

```
GET /export.json[?verdict=match|maybe|all]
```

- Sem parâmetro: `match` + `maybe` (77 anúncios hoje).
- `?verdict=match` ou `?verdict=maybe`: só aquele veredito.
- `?verdict=all`: os três vereditos, incluindo os 580 descartados.
- Qualquer outro valor: 400, mesmo caminho do `setState` (`fail`).

O sufixo `.json` é intencional: deixa óbvio no navegador e no `curl` que aquilo
não é uma página.

Anúncios encerrados (`status = "gone"`) entram em todos os recortes. O histórico
de preço de um anúncio que saiu do ar é a base de comparação de quanto o modelo
costuma valer — descartá-lo empobrece a análise.

`Content-Type: application/json; charset=utf-8`. Resposta indentada com dois
espaços: o volume é pequeno e alguém vai abrir isso no navegador.

## Formato

```json
{
  "generated_at": "2026-08-11T18:30:00Z",
  "counts": {"match": 14, "maybe": 63, "reject": 580},
  "sources": [
    {
      "name": "olx",
      "status": "ok",
      "last_run_at": "2026-08-11T15:05:00Z",
      "recent_counts": [31, 29, 30]
    }
  ],
  "fipe": [
    {
      "label": "HD FAT BOB 114",
      "bike": "fat bob",
      "variant": "114",
      "year": 2019,
      "price_cents": 7420000,
      "month": "agosto de 2026"
    }
  ],
  "listings": [
    {
      "id": 412,
      "source": "olx",
      "external_id": "1298374651",
      "url": "https://...",
      "title": "Harley-Davidson Fat Bob 114",
      "bike": "fat bob",
      "variant": "114",
      "year": 2019,
      "km": 18400,
      "price_cents": 6890000,
      "city": "Rio de Janeiro",
      "state": "RJ",
      "phone": "21999998888",
      "verdict": "match",
      "user_state": "new",
      "status": "active",
      "fingerprint": "fat bob|2019|3|rio de janeiro",
      "notified": true,
      "notified_price_cents": 7190000,
      "published_at": null,
      "first_seen_at": "2026-07-02T11:04:00Z",
      "last_seen_at": "2026-08-11T15:05:00Z",
      "fipe": {
        "label": "HD FAT BOB 114",
        "year": 2019,
        "price_cents": 7420000,
        "gap_percent": 7.1,
        "below_fipe": true,
        "base_variant": false
      },
      "price_drop_cents": 300000,
      "price_history": [
        {"price_cents": 7190000, "at": "2026-07-02T11:04:00Z"},
        {"price_cents": 6890000, "at": "2026-08-05T09:12:00Z"}
      ],
      "reposts": [
        {
          "id": 88,
          "source": "mercadolivre",
          "verdict": "match",
          "price_cents": 7190000,
          "km": 18300,
          "first_seen_at": "2026-06-11T08:20:00Z"
        }
      ]
    }
  ]
}
```

### Contagens

`counts` conta o banco inteiro, não o recorte devolvido. O agente que pede
`?verdict=match` precisa saber que existem 580 descartados — senão conclui que
a caçada está parada quando ela só está filtrando bem.

### Fontes

Um item por fonte configurada, na ordem do config. `status` vem de
`crawl.HealthStatus` sobre as últimas 30 rodadas, o mesmo valor que a página
`/health` mostra. `recent_counts` são as contagens dessas rodadas, da mais
recente para a mais antiga. Fonte que nunca rodou: `last_run_at: null` e
`recent_counts: []`.

Sem isso o agente não distingue "nenhum match novo" de "o crawler da OLX está
mudo há três rodadas" — e essas duas situações pedem conclusões opostas.

Erro na leitura da saúde de uma fonte derruba a resposta com 500. O dashboard
tolera (`lights()` devolve nil e a página renderiza sem os sinais), mas um JSON
que omite silenciosamente uma fonte mente para o consumidor.

### FIPE

`fipe` no envelope é a tabela vigente inteira (14 linhas), com o mês de
referência de cada linha.

`fipe` dentro do anúncio é a referência que casou com aquela moto, via
`fipe.Table.Lookup(bike, variant, year)` — o mesmo caminho do dashboard.

- `gap_percent`: distância percentual entre o preço pedido e a FIPE, valor
  absoluto com uma casa decimal, calculado sobre a FIPE
  (`|ref - price| * 100 / ref`).
- `below_fipe`: `true` quando o pedido está abaixo da FIPE. O sinal fica aqui e
  não no `gap_percent` porque um número assinado se lê nos dois sentidos.
- `base_variant`: `true` quando a variante do anúncio é desconhecida e o lookup
  caiu na variante base — a comparação vale menos e o agente merece saber.
- Anúncio sem ano ou sem casamento na tabela: `fipe: null`.
- Anúncio que casa na tabela mas não tem preço pedido: o objeto vem com
  `label`, `year`, `price_cents` e `base_variant`, e `gap_percent`/`below_fipe`
  nulos. A referência sozinha já informa, e omiti-la perderia o casamento.

O limiar de 5% que o dashboard usa para marcar pechincha (`bargainGapPercent`)
não é replicado. Ele é decisão de apresentação; o agente recebe o número e
decide sozinho.

### Queda de preço

`price_drop_cents` é `first_price_cents - price_cents` quando positivo, e `null`
caso contrário — nunca `0`, que se confundiria com "não caiu" e "não sei". Usa
o `FirstPriceCents` que o `store.Row` já traz (primeiro ponto do histórico).

### Histórico

`price_history` é a série completa daquele anúncio, ordenada por
`observed_at`, `id`. Anúncio sem histórico: array vazio.

### Reanúncios

`reposts` traz os irmãos de fingerprint, pelo mesmo critério do dashboard
(`store.RepostGroups`: mesmo fingerprint, `km` não nulo, fingerprint não vazio).

Cada irmão vem com dados próprios, não só o `id`, porque o irmão pode estar fora
do recorte exportado — um `id` órfão obrigaria o agente a uma segunda chamada
para descobrir do que se trata. Sem irmãos: array vazio.

O `reposts` não repete a rotulagem do dashboard ("possível reanúncio" versus
"anunciada também em X"). O agente compara `source` e chega lá sozinho.

### Nulos

Campo ausente sai como `null`, nunca como zero ou string vazia. A regra do repo
— ausente é ponteiro nil, nil nunca vira zero em silêncio — vale no JSON. Um
anúncio sem preço não é um anúncio de graça, e uma moto sem km rodado não é uma
moto zero.

Campos que o schema garante não nulos (`city`, `state`, `image_url`,
`fingerprint`) saem como string, vazia quando vazios.

`raw_text` e `verdict_reason` ficam de fora: o `store.Row` não os carrega, o
primeiro é o HTML bruto do anúncio e o segundo é o rastro interno do
classificador. Nenhum dos dois ajuda a decidir sobre uma moto.

### Datas

Todas em UTC, RFC 3339. O dashboard converte para hora local porque quem lê é
gente; um consumidor de JSON prefere o instante sem ambiguidade de fuso.

## Arquitetura

Arquivo novo `internal/web/export.go`, no pacote que já existe.

```
handler exportJSON(w, r)
  ├── lê verdict da query, valida
  ├── store.ListByVerdict(v) por veredito pedido
  ├── store.CountByVerdict()          [novo]
  ├── store.RepostGroups()
  ├── store.FipeReferences()
  ├── store.PriceHistoryFor(ids)      [novo]
  ├── store.RecentRunCounts + LastRunAt por fonte
  └── buildExport(...) → envelope → json.Encoder
```

`buildExport` é função pura: recebe linhas, grupos de reanúncio, histórico,
tabela FIPE, saúde das fontes e o instante, devolve o envelope. Testa-se sem
subir HTTP e sem banco.

Um pacote `internal/export` separado foi considerado e descartado: não haveria
outro consumidor, e o handler continuaria fazendo as mesmas leituras do store.

### Adições ao store

**`PriceHistoryFor(ids []int64) (map[int64][]PricePoint, error)`** — o
`GetRow` existente lê o histórico de um anúncio por vez. Com `?verdict=all`
seriam 657 idas ao banco. Uma query com `WHERE listing_id IN (...)` agrupada em
memória resolve. Lista de ids vazia devolve mapa vazio sem tocar o banco.

**`CountByVerdict() (map[string]int, error)`** — `GROUP BY verdict` sobre
`listings`, espelhando o `CountByState` que já existe. Veredito sem nenhuma
linha simplesmente não aparece no mapa; o build preenche `0` para os três
vereditos conhecidos, para que `counts` tenha sempre a mesma forma.

Nenhuma mudança de schema. Nenhuma migration.

### Structs

Structs de saída próprias, com tags `json`, em `export.go`. Serializar
`store.Row` direto acoplaria o contrato do JSON ao formato de armazenamento —
uma coluna renomeada quebraria o consumidor sem aviso.

## Testes

TDD red-first, teste falhando antes da implementação, no padrão de
`server_test.go` (que já monta store temporário e chama o handler via
`httptest`).

Em `internal/web/export_test.go`:

- O recorte padrão traz match e maybe, e não traz reject.
- `?verdict=match` traz só match; `?verdict=all` traz os três.
- `?verdict=lixo` responde 400.
- Anúncio encerrado (`status = "gone"`) aparece no recorte.
- `counts` reflete o banco inteiro, não o recorte — um pedido de
  `?verdict=match` num banco com rejects mostra os rejects em `counts`.
- Preço abaixo da FIPE: `gap_percent` correto e `below_fipe: true`. Acima:
  `below_fipe: false`.
- Variante desconhecida que casa na base: `base_variant: true`.
- Anúncio sem ano e anúncio sem casamento na tabela: `fipe: null`.
- Anúncio sem preço com referência: objeto FIPE presente, `gap_percent: null`.
- Preço nulo sai como `null` no JSON, não como `0` — asserção sobre o texto
  cru da resposta, não sobre a struct desserializada, porque é o texto que o
  agente lê.
- Queda de preço presente vira `price_drop_cents`; preço estável ou em alta
  vira `null`.
- `price_history` sai ordenado e completo; anúncio sem histórico vira `[]`.
- Irmão de fingerprint aparece em `reposts` com seus próprios campos, mesmo
  quando o irmão está fora do recorte pedido (irmão `reject` num export de
  `match`).
- Anúncio sem irmão: `reposts: []`.
- Fonte que nunca rodou: `last_run_at: null`, `recent_counts: []`.
- Banco vazio: `listings: []`, `counts` com os três vereditos em zero, status
  200.

Em `internal/store/store_test.go`:

- `PriceHistoryFor` agrupa por anúncio e ordena os pontos.
- `PriceHistoryFor` com lista vazia devolve mapa vazio.
- `CountByVerdict` conta os três vereditos.

## Fora do endpoint

O `Makefile` ganha um alvo `export`, `curl` no endpoint com saída em arquivo,
para quem preferir o terminal ao navegador. O `README` ganha a rota na lista de
páginas do dashboard.
