# harley-hunter

Robô que vigia anúncios de Harley-Davidson Street Glide e Road Glide na OLX e no
Mercado Livre, guarda o que encontra em SQLite e avisa por notificação nativa do
macOS quando aparece um anúncio dentro do alvo. O clique no banner abre o anúncio
no navegador.

## Pré-requisitos

- Go 1.24+
- Google Chrome instalado (a coleta lê as páginas por CDP; as duas fontes bloqueiam HTTP puro)
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

## Fase 2

O plano das próximas fontes (Webmotors, iCarros, Instagram e Facebook
Marketplace), da marcação de reanúncio e da expiração de anúncios sumidos está em
`docs/superpowers/plans/2026-08-09-harley-hunter-fase2.md`.
