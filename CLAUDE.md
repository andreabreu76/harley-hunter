# CLAUDE.md — harley-hunter

Projeto pessoal do dono: caça um modelo específico de Harley-Davidson em seis
marketplaces brasileiros e avisa por notificação da área de trabalho — macOS,
Linux e Windows, cada um pelo mecanismo nativo. Um alerta perdido é uma moto
perdida — é esse o critério que decide dúvidas de design.

## CÓDIGO

- Sem comentários. Nomes carregam a intenção.
- TDD red-first sempre: teste que falha primeiro, rodado para ver falhar pelo
  motivo esperado, e só então a implementação.
- Dinheiro em centavos `int64`. Nunca float para valor monetário.
- Ausente = ponteiro nil. Nil nunca vira zero em silêncio.
- Mensagens de commit em inglês, descrevendo o comportamento e não o diff
  (`fix:`, `feat:`, `test:`, `docs:`).
- Documentação e conteúdo voltado ao dono (README, handoff) em português.

## NUNCA COMMITAR

- `.env` e `*.db`. O banco de produção passou a viver no diretório por usuário
  (`hunter paths`); o `hunter.db` na raiz do repo é o legado, que a migração
  descrita no README leva para lá.
- Arquivos de scratch dos agentes (`zz_*`, `*.bak`) — conferir `git status`
  antes de commitar.

## FLUXO DE TRABALHO

O repo **não tem remote**. Não existe PR.

- Trabalho de fase acontece em branch local (`fase-N`), criada de `main`.
- Worktree nativo não serve: ele ramifica de `origin/<default>` e não há origin.
- O merge em `main` é `--no-ff`, com mensagem `merge: fase N do harley-hunter`.
- Antes de qualquer merge: `go build ./... && go vet ./... && go test ./...`.

## PRODUÇÃO

Roda na máquina do dono, não em servidor.

- `hunter serve` é um daemon: fica de pé e agenda a própria coleta. O relógio é o
  `interval_hours` da seção `crawl` do config (padrão 12h), lido a quente —
  mudar o intervalo não exige reiniciar nada nem mexer no plist.
- O agente launchd `com.andreabreu.harleyhunter` é só autostart: `RunAtLoad` +
  `KeepAlive` sobre `~/bin/hunter serve`, sem `StartInterval`. Ele declara o
  `PATH` com `/opt/homebrew/bin` na frente, senão o `terminal-notifier` some e o
  alerta cai no `osascript`, cujo banner não abre o anúncio no clique. Editar o
  plist só vale depois de `make agent-install`.
- Binário em `~/bin/hunter`. **Editar o repo não muda produção** — só
  `go build -o ~/bin/hunter ./cmd/hunter` muda.
- Config, banco, perfil do Chrome e log ficam no diretório por usuário, que
  `hunter paths` imprime (`~/Library/Application Support/harley-hunter` no
  macOS). `HARLEY_HUNTER_HOME` troca esse diretório inteiro.
- O hunter acha e sobe o próprio Chrome, numa porta livre escolhida na hora, com
  o perfil dedicado `chrome-profile` dentro daquele diretório, e o derruba no fim
  da rodada. `devtools_url` preenchido inverte isso: passa a significar navegador
  externo, que o hunter só consome.
- Instagram e Facebook Marketplace exigem sessão logada nesse perfil. Sem ela não
  dão erro: voltam vazios, o que no painel parece "não tinha nada à venda". O
  login é manual nesta fase (Chrome aberto contra o perfil, com o daemon parado);
  a tela com botão é da fase 8.
- Log em `<diretório>/logs/hunter.log`, com rotação em 5 MB (`hunter.log.1`).
  Escrito pelo `serve`; o `crawl` só imprime no terminal. O `stderr` do agente
  cai em `logs/launchd.error.log`, ao lado. Os
  `ERROR: unhandled node event *dom.Event...` são ruído do DevTools, não do
  nosso código.
- `Makefile` na raiz tem os atalhos (`build`, `crawl`, `serve`, `cross`,
  `agent-install`, `agent-status`, `logs`). Nenhum deles passa `-config`: o
  binário resolve o diretório por usuário sozinho.

### Mexer em produção

Nunca rodar verificação contra o banco de produção direto — com `KeepAlive` o
daemon está sempre de pé e pode coletar no meio. Copiar o banco (**inclusive
`-wal` e `-shm`**, porque roda em WAL) e trabalhar na cópia, por um dos dois
caminhos: `HARLEY_HUNTER_HOME=/tmp/hunter-scratch`, que troca o diretório
inteiro, ou `-config` apontando para um config próprio.

`database_path` relativo é resolvido em relação ao **diretório do arquivo de
config**, não ao cwd. O `config/config.yaml` versionado no repo ainda traz
`../hunter.db` e `devtools_url: http://127.0.0.1:9222`, herança da versão
anterior; ele não é mais usado por nenhum alvo do Makefile e não é o config de
produção.

### `repair-silenced`

Comando one-shot, **nunca em cron**. Ele se rearma: depois que uma linha
devolvida à fila é alertada, rodar de novo a devolve outra vez e gera alerta
duplicado. Rodar uma vez, com o dashboard aberto.

## DECISÕES QUE JÁ FORAM TOMADAS (não relitigar)

- **Fixture-primeiro para qualquer fonte externa.** Toda premissa de markup
  escrita de véspera, sem olhar o HTML real, estava errada. Buscar a página,
  salvar a fixture, e só então escrever o parser.
- **O fingerprint é grosseiro de propósito** (`bike|year|km/5000|city`), para
  que um reanúncio com km atualizado ainda case. Endurecer o seed não é a
  solução para colisão — a regra de *quando* silenciar é.
- **A dedup conta anúncios por fonte**, não guarda booleano por fonte. A forma
  booleana silencia uma moto real no caso de três anúncios. O total de alertas
  de um fingerprint é a maior contagem por fonte, e isso não depende da ordem.
- **Queda de preço é estado derivado**, não fila enfileirada: preço atual abaixo
  de `notified_price_cents` (o menor preço já comunicado). Cap estourado e envio
  falho não perdem nada, sem código de compensação.
- **Na dúvida, alertar.** Entre um alerta duplicado e uma queda real morrendo em
  silêncio, o projeto escolhe o duplicado.

## MÉTODO

As fases 2 e 3 usaram subagent-driven-development: um implementer por tarefa,
review por tarefa, review da branch inteira no fim. Vale registrar que
implementers acharam lacunas reais nos planos — um call site não listado e uma
permutação de teste pedida errada, que teria produzido teste sem dentes. Tratar
o plano como falível.

Especificações e planos ficam em `docs/superpowers/{specs,plans}/`. Handoffs de
fase em `~/Documents/Claude/harley-hunter/`.
