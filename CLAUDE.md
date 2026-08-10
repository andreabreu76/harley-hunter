# CLAUDE.md — harley-hunter

Projeto pessoal do dono: caça um modelo específico de Harley-Davidson em seis
marketplaces brasileiros e avisa por notificação do macOS. Um alerta perdido é
uma moto perdida — é esse o critério que decide dúvidas de design.

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

- `.env` e `*.db` — `hunter.db` é o banco de produção e vive na raiz do repo.
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

- launchd `com.andreabreu.harleyhunter` a cada 7200s, via
  `deploy/hunter-crawl.sh`.
- Binário em `~/bin/hunter`. **Editar o repo não muda produção** — só
  `go build -o ~/bin/hunter ./cmd/hunter` muda.
- Chrome dedicado na porta 9222, perfil em
  `~/Library/Application Support/harley-hunter-chrome`. Instagram logado;
  Facebook funciona deslogado.
- Logs em `~/Library/Logs/harley-hunter.{log,error.log}`. Os
  `ERROR: unhandled node event *dom.Event...` são ruído do DevTools, não do
  nosso código.
- `Makefile` na raiz tem os atalhos (`build`, `crawl`, `serve`, `agent-install`,
  `logs`).

### Mexer em produção

Nunca rodar verificação contra `hunter.db` direto — o launchd pode disparar no
meio. Copiar o banco (**inclusive `-wal` e `-shm`**, porque roda em WAL), gerar
um config apontando para a cópia, e trabalhar ali.

`database_path` relativo é resolvido em relação ao **diretório do arquivo de
config**, não ao cwd. Por isso `../hunter.db` em `config/config.yaml` aponta
para a raiz do repo.

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
