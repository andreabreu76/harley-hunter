# Harley Hunter — Design

Data: 2026-08-09
Status: aprovado, pronto para plano de implementação

## Objetivo

Monitorar continuamente anúncios de Harley-Davidson Street Glide e Road Glide,
ano 2014-2015, até R$ 75.000, nas regiões do Rio de Janeiro, São Paulo e
Curitiba. Encontrar o anúncio cedo é o que define a compra: as motos bem
precificadas somem em horas, e revisar seis sites na mão todo dia não é
sustentável.

O sistema coleta de marketplaces e das redes da Meta, classifica cada anúncio,
guarda histórico de preço e avisa por SMS quando aparece algo dentro do alvo.

## Escopo do alvo

| Critério | Valor |
|---|---|
| Modelos | Street Glide (FLHX), Street Glide Special (FLHXS), CVO Street Glide (FLHXSE), Road Glide (FLTRX), Road Glide Special (FLTRXS), CVO Road Glide (FLTRXSE) |
| Anos | 2014 e 2015 (ano-modelo) |
| Preço-teto | R$ 75.000 |
| Regiões | Rio de Janeiro, São Paulo e Curitiba, incluindo regiões metropolitanas |

Nota de mercado: a Road Glide saiu de linha após 2013 e retornou em 2015 com o
Project Rushmore. Na prática quase nenhuma unidade 2014 existe, e nenhuma regra
especial é necessária — o eixo de ano já trata o caso.

## Decisões tomadas

| Decisão | Escolha |
|---|---|
| Cobertura de fontes | Marketplaces + Facebook Marketplace + Instagram |
| Descoberta no Instagram | Hashtags e busca aberta |
| Entrega | Dashboard web local, com SMS para os melhores casos |
| Rigor do filtro | Estrito, com zona cinza preservada em aba separada |
| Ambiente | Mac local, agendado via `launchd` |
| Arquitetura | Binário Go único, SQLite, Chrome real acoplado por CDP |
| Canal de SMS | Twilio, conta existente informada pelo usuário |

## Arquitetura

Binário Go único com dois subcomandos. `hunter crawl` é disparado pelo `launchd`
em intervalo configurável (padrão 2 horas) e executa uma rodada de coleta.
`hunter serve` sobe o dashboard em `localhost` lendo o mesmo banco.

```
harley-hunter/
  cmd/hunter/main.go          # subcomandos: crawl | serve
  internal/source/
    source.go                 # interface Source
    olx.go  mercadolivre.go  webmotors.go  icarros.go
    meta/instagram.go  meta/marketplace.go
  internal/normalize/         # RawListing -> Listing
  internal/match/             # classificação em quatro eixos
  internal/store/             # SQLite
  internal/notify/            # Notifier + twilio.go
  internal/web/               # handlers e templates do dashboard
  config/config.yaml
```

Fluxo de uma rodada:

```
launchd -> hunter crawl
             |
             +-- sources (paralelo, concorrência limitada, timeout por fonte)
                   |
                   v
             RawListing -> normalize -> Listing -> match -> verdict
                                                              |
                                                              v
                                                    store (dedup + histórico)
                                                              |
                                              novos com verdict=match
                                                              |
                                                              v
                                                       notify (SMS)
```

### Fronteira entre componentes

Uma fonte sabe apenas buscar e devolver dados crus. Ela não conhece preço-teto,
ano-alvo nem cidade; quem decide é o matcher. Isso mantém cada fonte pequena e
substituível, o que é essencial porque OLX e Instagram vão quebrar em momentos
diferentes e cada conserto precisa ser local.

```go
type Source interface {
    Name() string
    Fetch(ctx context.Context) ([]RawListing, error)
}
```

`RawListing` carrega o texto bruto, a URL, o identificador na plataforma e os
campos estruturados que a fonte conseguiu oferecer. Campo que a fonte não tem
fica ausente, nunca preenchido por suposição.

### Fontes HTTP e fontes de navegador

OLX, Mercado Livre, Webmotors e iCarros respondem a requisições HTTP com HTML ou
JSON previsível e são implementadas com `net/http` e parsing direto.

Instagram e Facebook Marketplace exigem sessão autenticada e detectam
automação. São acessadas por `chromedp` conectado via CDP a uma instância do
Chrome do próprio usuário, já aberta e logada, em vez de um navegador headless.
Navegador real, perfil real e IP residencial formam a configuração menos
detectável disponível sem proxy pago.

## Modelo de dados

SQLite, arquivo único no diretório de dados do usuário.

**listings** — um registro por anúncio conhecido.

| Coluna | Tipo | Observação |
|---|---|---|
| `id` | INTEGER PK | |
| `source` | TEXT | `olx`, `mercadolivre`, `webmotors`, `icarros`, `instagram`, `marketplace` |
| `external_id` | TEXT | identificador na plataforma; shortcode no Instagram |
| `url`, `title`, `raw_text` | TEXT | `raw_text` guarda a legenda ou descrição íntegra |
| `model` | TEXT | `street_glide`, `road_glide`, `unknown` |
| `variant` | TEXT | `base`, `special`, `cvo`, `unknown` |
| `year` | INTEGER NULL | nulo quando não extraído |
| `price_cents` | INTEGER NULL | nulo quando ausente ou "a combinar" |
| `km` | INTEGER NULL | |
| `city`, `state` | TEXT | |
| `image_url` | TEXT | primeira imagem, exibida no dashboard |
| `verdict` | TEXT | `match`, `maybe`, `reject` |
| `verdict_reason` | TEXT | JSON com o resultado de cada eixo |
| `fingerprint` | TEXT | impressão digital aproximada, para detectar reanúncio |
| `user_state` | TEXT | `new`, `contacted`, `dismissed` |
| `notified` | INTEGER | 0/1 |
| `first_seen_at`, `last_seen_at` | TIMESTAMP | |
| `status` | TEXT | `active` ou `gone` |

Restrição de unicidade em `(source, external_id)`.

**price_history** — `listing_id`, `price_cents`, `observed_at`. Uma linha por
mudança de preço observada.

**source_runs** — `source`, `started_at`, `finished_at`, `item_count`,
`status`, `error`. Base do painel de saúde.

## Normalização

Converte o caos dos anúncios em campos comparáveis.

Preço aceita `"R$ 74.900"`, `"74.900,00"`, `"74,9 mil"` e devolve centavos.
`"a combinar"`, `"consulte"` e variantes resultam em ausente, nunca em zero.

Ano lida com `"2014/2015"` usando o ano-modelo, e com ano embutido no título
(`"15/15"`, `"mod. 2015"`).

Quilometragem, cidade e estado seguem a mesma regra: extraído quando existe,
ausente quando não. Cidades são resolvidas contra uma tabela das três regiões
metropolitanas para distinguir `match` de `maybe` no eixo de local.

Fontes estruturadas preenchem a maior parte dos campos diretamente. Instagram e
Marketplace extraem tudo de texto livre e frequentemente deixam campos ausentes
— comportamento esperado, tratado pelo matcher.

## Matching

Quatro eixos independentes, cada um devolvendo `match`, `maybe` ou `reject`:

| Eixo | match | maybe | reject |
|---|---|---|---|
| Modelo | Street Glide ou Road Glide, qualquer variante | "Harley touring" sem modelo identificável | Electra Glide, Ultra, outro modelo, outra marca |
| Ano | 2014, 2015 | 2013, 2016, ou ausente | demais |
| Preço | ≤ R$ 75.000 | R$ 75.001–85.000, ou ausente | > R$ 85.000 |
| Local | RJ, SP e Curitiba com regiões metropolitanas | resto de RJ, SP e PR | demais estados |

Combinação: qualquer `reject` derruba o anúncio; sem `reject` e com ao menos um
`maybe`, o veredito é `maybe`; `match` exige os quatro eixos em `match`.

A decisão deliberada é que **ano e preço ausentes caem em `maybe`, não em
`reject`**. Anúncio mal preenchido é justamente onde costuma estar a barganha, e
descartá-lo silenciosamente anularia boa parte do valor do sistema.

O reconhecimento de modelo trabalha sobre texto normalizado (minúsculas, sem
acento) e cobre grafias reais: `street glide`, `streetglide`, `st glide`,
`road glide`, `roadglide`, `flhx`, `flhxs`, `flhxse`, `fltrx`, `fltrxs`,
`fltrxse`. A distinção entre Road Glide e Electra Glide é explícita, já que
ambos contêm "glide" e apenas o primeiro é alvo.

### Filtro prévio do Instagram

Antes do matcher, um post do Instagram precisa apresentar sinal de venda na
legenda: preço, `vendo`, `à venda`, `disponível`, `aceito troca` ou equivalente.
Sem isso é descartado. Perfis de loja publicam muita foto de moto sem intenção
de venda daquela unidade, e sem esse filtro o dashboard fica inutilizável.

## Deduplicação e histórico

A chave primária de identidade é `(source, external_id)`. Reencontrar o mesmo
anúncio atualiza `last_seen_at` e, se o preço mudou, grava linha em
`price_history`.

Reanúncio da mesma moto com identificador novo é detectado por impressão digital
aproximada sobre modelo, ano, quilometragem e cidade. O registro não é ocultado:
aparece marcado como possível reanúncio, porque republicação após queda de preço
indica vendedor com pressa — informação de negociação, não ruído.

Anúncio que deixa de aparecer por três rodadas consecutivas recebe
`status = gone`, mas permanece no banco. O histórico do que já saiu do ar é o
que permite avaliar se um preço pedido é razoável.

## Notificação

Interface `Notifier` com implementação Twilio, configurada exclusivamente por
variáveis de ambiente: `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`,
`TWILIO_FROM`, `ALERT_TO`. Nenhuma credencial no repositório ou no
`config.yaml`.

O SMS dispara apenas para anúncio novo com veredito `match`. Vereditos `maybe`
nunca notificam — ficam para revisão no dashboard.

Dois limites protegem contra falha em massa. Há um teto de 5 SMS por rodada: se
um parser quebrar e classificar mal em volume, chegam 5 mensagens e um aviso, e
não centenas. E falha de envio não descarta alerta — o anúncio permanece com
`notified = false` e é reenviado na rodada seguinte, de modo que indisponibilidade
da Twilio adia a notificação em vez de perdê-la.

## Dashboard

Servidor Go com `html/template`, sem framework de frontend.

| Rota | Função |
|---|---|
| `GET /` | anúncios com veredito `match` |
| `GET /maybe` | zona cinza, para revisão manual |
| `GET /rejected` | descartados, para auditar o matcher |
| `GET /listing/{id}` | detalhe, com histórico de preço |
| `GET /health` | painel de saúde por fonte |
| `POST /listing/{id}/state` | marca `contacted` ou `dismissed` |

Cada item mostra foto, título, preço com variação desde a primeira observação,
ano, quilometragem, cidade, fonte e link para o anúncio original. A aba
`rejected` existe para calibração: quando uma moto boa é descartada por engano,
é ali que o erro aparece.

## Falhas e resiliência

Cada fonte roda isolada, com timeout próprio. Falha em uma não interrompe as
demais nem aborta a rodada. Toda execução é registrada em `source_runs`.

O modo de falha mais perigoso é o silêncio: quando um site muda o HTML, o parser
passa a devolver zero itens sem erro, e a ausência de anúncios parece resultado
legítimo. Contra isso, cada fonte mantém a média de itens das últimas cinco
rodadas; se retornar zero em duas rodadas consecutivas tendo média igual ou
superior a três itens, é marcada como provavelmente quebrada e destacada no
painel de saúde.

Nas fontes da Meta o cuidado é de exposição: intervalos aleatórios entre
requisições e volume baixo por rodada. Diante de tela de login, checkpoint ou
captcha, a fonte aborta imediatamente e sinaliza no dashboard, sem nova
tentativa. Insistir é o caminho mais curto para o bloqueio da conta.

## Testes

Fixtures em `testdata/` com HTML e JSON reais de cada fonte; parsers testados sem
acesso à rede. Testes que dependem de rede ficam atrás de build tag separada.

Normalizador e matcher são cobertos por tabela de casos com títulos reais
bagunçados, entre eles `"HD STREET GLIDE ESPECIAL 15/15 IMPECÁVEL"`,
`"Harley 1690 Rushmore aceito troca"`, `"Road Glide Special 2015 - valor a
combinar"` e `"Electra Glide Ultra Limited 2015"` (que deve ser rejeitado).

Quando um parser quebrar em produção, o HTML novo entra em `testdata/` como caso
de regressão e o teste falha antes do conserto.

## Riscos assumidos

O acesso automatizado ao Instagram e ao Facebook Marketplace contraria os termos
de uso da Meta e implica risco de bloqueio da conta utilizada. O risco foi
apresentado e aceito de forma explícita; as mitigações são o uso do navegador
real, volume baixo, intervalos aleatórios e interrupção imediata diante de
checkpoint.

O envio de SMS usará a conta Twilio já existente indicada pelo usuário, por
decisão explícita dele. As credenciais entram apenas por variáveis de ambiente e
não são versionadas.

Fontes da Meta são frágeis por natureza e devem quebrar com frequência. O
`config.yaml` permite desativá-las sem recompilar, e o painel de saúde mostra
quando isso acontece.

## Fora de escopo

Consulta a tabela FIPE, avaliação automática de preço justo, negociação
automatizada, contato com vendedor, aplicativo móvel e implantação em servidor
remoto. O sistema encontra e organiza; a decisão e o contato são manuais.
