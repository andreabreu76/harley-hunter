# harley-hunter

Robô que vigia anúncios de Harley-Davidson Street Glide e Road Glide na OLX e no
Mercado Livre, guarda o que encontra em SQLite e avisa por notificação nativa do
macOS quando aparece um anúncio dentro do alvo. O clique no banner abre o anúncio
no navegador.

## Pré-requisitos

- Go 1.26+
- Google Chrome instalado (OLX, Mercado Livre e Webmotors bloqueiam HTTP puro e são lidas por CDP; a Mobiauto não precisa dele)
- `terminal-notifier` (`brew install terminal-notifier`), necessário para o clique no banner abrir o anúncio

## Instalação

```bash
go build -o ~/bin/hunter ./cmd/hunter
```

## Uso

Coleta uma rodada e dispara os alertas pendentes:

```bash
deploy/hunter-crawl.sh
```

O script sobe, se ainda não estiver de pé, um Chrome dedicado na porta 9222 com
perfil próprio em `~/Library/Application Support/harley-hunter-chrome`, fora da
tela e sem `--headless` (o Cloudflare bloqueia headless). Ele deixa o navegador
vivo entre as rodadas. Para chamar o binário direto, com o Chrome já rodando:

```bash
~/bin/hunter -config config/config.yaml crawl
```

Dashboard local, sob demanda — não entra no agendamento:

```bash
~/bin/hunter -config config/config.yaml serve
```

Sobe em <http://127.0.0.1:8080> com as abas de match, maybe, rejeitados e saúde
das fontes. Encerra com Ctrl+C.

Com o dashboard no ar, `GET /export.json` devolve o banco em JSON, para entregar
a um agente. Traz match e maybe por padrão; `?verdict=match`, `?verdict=maybe` e
`?verdict=all` estreitam ou ampliam o recorte. Cada anúncio vai com a referência
FIPE e o gap em percentual, a queda desde o primeiro preço visto, o histórico
completo e os reanúncios irmãos. O envelope leva ainda a contagem de todo o
banco e a saúde de cada fonte. `make export` grava o arquivo, e `make export
VERDICT=all` traz os descartados junto.

Devolve à fila de alerta os anúncios que o dedup antigo calou sem avisar:

```bash
~/bin/hunter -config config/config.yaml repair-silenced
```

Ele lista os ids que vai liberar antes de gravar e, no fim, quantos voltaram para
a fila. O próximo `crawl` avisa sobre eles, respeitando o teto de alertas por
rodada.

Rode **uma única vez**, num momento em que dê para acompanhar a fila — nunca no
agendamento. O comando se rearma: depois que um anúncio liberado já foi avisado,
rodar de novo o coloca outra vez na fila e gera alerta duplicado.

## Agendamento

A coleta roda sozinha a cada 2 horas por um agente do `launchd`.

```bash
cp deploy/com.andreabreu.harleyhunter.plist ~/Library/LaunchAgents/
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.andreabreu.harleyhunter.plist
launchctl list | grep harleyhunter
```

Para desligar:

```bash
launchctl bootout gui/$(id -u)/com.andreabreu.harleyhunter
```

Depois de editar o plist, é preciso copiar de novo para `~/Library/LaunchAgents`
e refazer o par `bootout` + `bootstrap` — o `launchd` não relê o arquivo sozinho.

## Logs

- `~/Library/Logs/harley-hunter.log` — resumo por fonte, contagem de novos matches e de alertas mostrados
- `~/Library/Logs/harley-hunter.error.log` — erros; o ruído `unhandled node event` vem do chromedp e é inofensivo

Se o log de erro trouxer o aviso de que o `terminal-notifier` não foi encontrado
no `PATH`, o alerta caiu no fallback de `osascript` e o clique no banner deixa de
abrir o anúncio. A causa é o `PATH` mínimo que o `launchd` entrega; o
`deploy/hunter-crawl.sh` corrige isso exportando `/opt/homebrew/bin` na primeira
linha.

## Fontes e transporte

| Fonte | Transporte | Depende do Chrome |
|---|---|---|
| olx | CDP (`BrowserFetcher`) | sim |
| mercadolivre | CDP (`BrowserFetcher`) | sim |
| webmotors | CDP (`BrowserFetcher`) — PerimeterX responde 403 a HTTP puro | sim |
| mobiauto | HTTP puro (`HTTPFetcher`) | **não** |

A Mobiauto continua coletando com o Chrome fora do ar: ela fala HTTP direto, com
o User-Agent de navegador e timeout próprio. Numa rodada em que o Chrome não
sobe, as três primeiras fontes falham e a Mobiauto entrega normalmente — o painel
de saúde mostra exatamente isso, fonte a fonte.

Fonte que responde algo diferente de `200`, ou que devolve página de desafio no
lugar do payload esperado, falha alto e vira erro da rodada; não passa em branco.

## Fase 2

O plano das próximas fontes (Webmotors, Mobiauto, Instagram e Facebook
Marketplace), da marcação de reanúncio e da expiração de anúncios sumidos está em
`docs/superpowers/plans/2026-08-09-harley-hunter-fase2.md`.
