# Harley Hunter — Fase 3: nenhum anúncio bom passa batido

Data: 2026-08-10. Escopo fechado com o dono: itens 1 e 3 do backlog pós-fase 2.
FIPE, Instagram e as dívidas de abas/dashboard ficam para a fase 4.

## Objetivo

A fase 3 fecha os dois furos que fazem o sistema **perder um alerta**. Ao fim
dela vale a garantia: todo anúncio que casa com o critério, e toda queda de
preço num anúncio já conhecido, chega ao dono — mesmo que o cap da rodada
estoure, o envio falhe ou duas motos parecidas apareçam juntas.

Nada mais entra. Qualidade de sinal (FIPE), ruído de fonte (Instagram) e
conforto de operação (abas, paginação) não são objetivo desta fase.

## Furo 1 — dedup silencia motos distintas

### Estado atual

`crawl.Notify` (`internal/crawl/crawl.go:169`) monta `seen map[string]bool`
sobre o fingerprint das linhas pendentes da rodada. A segunda linha de um
fingerprint repetido recebe `MarkNotified` **sem envio** — silenciada para
sempre, porque `notified` é irreversível.

`Fingerprint` (`internal/normalize/normalize.go:106`) tem seed
`bike|year|km/5000|city`. O par real 237/251 (webmotors, 53.000 e 53.118 km)
cai no mesmo balde, mesma cidade, mesmo ano: duas motos diferentes com o mesmo
fingerprint. Uma delas nunca foi alertada.

Endurecer o seed não resolve este par — nenhum eixo disponível separa os dois.
O que resolve é mudar **quando** a dedup tem permissão de silenciar.

### Regra nova

Dois anúncios só são a mesma moto, para efeito de silenciar alerta, quando o
contexto é plausível:

- **fontes diferentes** — cross-post do mesmo vendedor em dois sites;
- **o outro lado já está `gone`** — reanúncio.

Dois anúncios **ativos na mesma fonte** são duas motos. Ambos alertam.

Implementação em `Notify`: por fingerprint, contar quantos anúncios de cada
fonte já foram percorridos (inclusive os silenciados) e quantos alertas aquele
fingerprint já produziu. Uma linha da fonte `S` com fingerprint `F` silencia
quando a contagem de `S` é **menor ou igual** ao número de alertas de `F`;
senão alerta, e a contagem de alertas sobe.

Em outras palavras, o número de alertas de um fingerprint é o maior número de
anúncios que uma única fonte tem dele — que é o piso de quantas motos distintas
aquele fingerprint representa.

| Anúncios na ordem percorrida | Alertas | Por quê |
|---|---|---|
| `wm(237)`, `wm(251)` | 2 | mesma fonte, ambos ativos → duas motos |
| `olx(X)`, `ml(X)` | 1 | fontes diferentes → cross-post da mesma moto |
| `olx(X)`, `ml(X)`, `wm(X)` | 1 | cross-post triplo, ainda uma moto |
| `olx(X)`, `wm(X)`, `wm(Y)` | 2 | webmotors tem dois anúncios → há uma segunda moto |

A última linha é a razão de contar em vez de guardar um booleano por fonte. Com
um booleano, `wm(Y)` seria lida como cross-post — webmotors não constaria como
"já alertou", porque foi a olx que alertou a moto X — e a moto Y seria
silenciada para sempre. É a mesma classe do bug 237/251, e só aparece com três
anúncios. Encontrado durante a implementação da fase, não na triagem.

O seed do fingerprint **não muda**, logo não há backfill nem risco com as
linhas `gone` que congelaram o seed antigo.

`dedupKey` mantém a exigência de `Km != nil`: sem km o fingerprint não
discrimina, e sem discriminar não se silencia nada.

### Reanúncio continua alertando

Quando o lado antigo está `gone`, ele não está na fila de pendentes, então a
linha nova alerta normalmente. Isso é desejado: a moto voltou ao mercado, com
preço possivelmente novo. A dedup existe só para o cross-post simultâneo. O
selo de reanúncio do dashboard é agrupamento visual e não se mistura com a
decisão de alerta.

### Reparo das vítimas já no banco

As linhas silenciadas indevidamente no passado têm `notified = 1` e nunca vão
alertar. Um comando one-shot (`hunter repair-silenced`) devolve à fila as
linhas que compartilham fingerprint **e** fonte com outra linha ativa,
mantendo a mais antiga do grupo como já notificada:

```sql
UPDATE listings SET notified = 0
WHERE status = 'active' AND verdict = 'match' AND notified = 1
  AND km IS NOT NULL AND fingerprint <> ''
  AND id NOT IN (
      SELECT MIN(id) FROM listings
      WHERE status = 'active' AND verdict = 'match'
        AND km IS NOT NULL AND fingerprint <> ''
      GROUP BY fingerprint, source);
```

Grupo de uma só linha é o próprio `MIN(id)` e sai pelo `NOT IN`, então não
precisa de `HAVING COUNT(*) > 1`.

Comando explícito, não migração automática: reenfileirar alerta é efeito
visível e o dono decide quando.

## Furo 2 — quedas de preço não sobrevivem à rodada

### Estado atual

`Report.Drops` vive só em memória e existe apenas para alimentar
`crawl.Notify` (`cmd/hunter/main.go:72`). Matches novos têm fila durável
(`listings.notified = 0`); quedas não têm nada. Se o cap de 5 estourar antes
(`crawl.go:205`) ou o envio falhar, a queda desaparece — e como o preço já foi
gravado, ela nunca mais é detectada.

### Estado persistido

Coluna nova em `listings`:

```
notified_price_cents INTEGER
```

Semântica: **o preço que já foi comunicado ao dono** para aquele anúncio.

- ao enviar alerta de match novo: grava o preço atual;
- ao enviar alerta de queda: grava o preço atual;
- **queda pendente** é derivada, não enfileirada:

```sql
verdict = 'match' AND status = 'active' AND notified = 1
  AND price_cents IS NOT NULL AND notified_price_cents IS NOT NULL
  AND price_cents < notified_price_cents
```

Consequências desejadas do estado derivado:

- cap estourado ou envio falho → a queda continua pendente na rodada seguinte,
  sem nenhuma limpeza de fila;
- duas quedas antes do envio → **um** alerta, com a queda acumulada, em vez de
  dois alertas defasados;
- **subida de preço não reancora.** `notified_price_cents` fica no menor preço
  já comunicado. Preço que sobe e volta ao patamar antigo não é notícia e não
  alerta. Só uma queda abaixo do menor já comunicado alerta.

### Migração das linhas existentes

Linhas com `notified = 1` anteriores à coluna ficariam com NULL e nenhuma
queda detectável. O seed âncora-as no preço atual:

```sql
UPDATE listings SET notified_price_cents = price_cents
WHERE notified = 1 AND price_cents IS NOT NULL
```

O seed roda **uma única vez**, no mesmo passo condicional que cria a coluna
(`addMissingColumns` só executa quando a coluna falta). Rodar de novo
apagaria quedas pendentes ao reancorar no preço já caído — por isso o struct de
`addedColumns` ganha um campo `seed` executado junto do ALTER, e não uma
migração idempotente solta.

## Fila única ordenada por urgência

`Notify` perde o parâmetro `drops` e passa a drenar uma fila só, vinda do
store: matches novos e quedas juntos, cortados no `max_alerts_per_run`. O que
não couber fica pendente por construção (`notified = 0` ou queda derivada).
`Report.Drops` é removido.

**Urgência** = desconto percentual do preço atual contra a melhor referência
disponível, nesta ordem:

1. o último preço notificado, quando houver queda →
   `(notified_price_cents - price_cents) / notified_price_cents`;
2. a FIPE, quando existir → `(fipe_cents - price_cents) / fipe_cents`;
3. nenhuma das duas → urgência 0.

Desempate: `first_seen_at ASC`, preservando a ordem atual.

### A dedup vale para a fila inteira

A regra fingerprint+fonte se aplica à fila unificada, não só aos matches novos.
Sem isso, o cross-post que sobrevive à dedup no match volta a duplicar quando
os dois lados baixam o preço na mesma rodada.

Ao silenciar por cross-post, `MarkNotified` também grava
`notified_price_cents`. Assim uma queda no anúncio silenciado continua sendo
detectável — a queda é notícia legítima mesmo que o match dele nunca tenha
gerado alerta, e a dedup da fila evita que os dois lados alertem juntos.

Consequência aceita explicitamente: **match novo sem FIPE entra com urgência
0** e fica atrás de qualquer queda medida. Hoje isso afeta sobretudo Special e
CVO, que não têm linha na FIPE — exatamente o que o item 2 da fase 4 conserta.
No volume atual (cap 5, ~14 matches) a fila drena em poucas rodadas e nada se
perde; o custo é atraso de uma rodada, não alerta perdido.

## Arquivos afetados

| Arquivo | Mudança |
|---|---|
| `internal/store/migrate.go` | coluna `notified_price_cents` + campo `seed` no struct de colunas |
| `internal/store/store.go` | `PendingAlerts` (fila única ordenada); `MarkNotified` grava o preço comunicado |
| `internal/store/lifecycle.go` | consulta do reparo one-shot |
| `internal/crawl/crawl.go` | dedup por fingerprint+fonte; `Notify` sem `drops`; `Report.Drops` removido |
| `cmd/hunter/main.go` | `sendAlerts` sem drops; comando `repair-silenced` |

Nenhum arquivo passa de ~300 linhas com a mudança; não há necessidade de
quebrar módulo nesta fase.

## Testes (red-first, um por furo real)

1. duas pendentes, mesma fonte, mesmo fingerprint → **2** envios;
2. duas pendentes, fontes diferentes, mesmo fingerprint → 1 envio, a outra
   marcada notificada;
3. queda que não cabe no cap → continua pendente na rodada seguinte;
4. envio de queda falha → não marca, segue pendente;
5. duas quedas antes do envio → 1 alerta com a queda acumulada;
6. preço sobe → nenhum alerta e sem reancoragem;
7. queda até o patamar já comunicado → nenhum alerta; abaixo dele → alerta;
8. migração ancora linhas `notified = 1` no preço atual, e o seed não roda
   duas vezes;
9. fila ordena queda de 10% acima de match novo com 3% abaixo da FIPE;
10. `repair-silenced` reenfileira a vítima do par mesma-fonte e preserva a
    linha mais antiga do grupo;
11. cross-post cujos dois lados caem de preço na mesma rodada → 1 alerta.

## Fora de escopo

FIPE (fallback special/cvo→base, cache negativo, Electra ancorada na Ultra),
oscilação do Instagram, snapshot de abas, `/health` sem `source_runs.error`,
paginação de `/rejected`, selo de reanúncio com dois lados ativos,
re-normalize em massa. Todos permanecem no backlog da fase 4.

## Convenções da casa

Código sem comentários. TDD red-first. Dinheiro em centavos `int64`. Ausente =
ponteiro nil. Commits sem menção a IA ou sessão. `.env` e `*.db` nunca staged.
