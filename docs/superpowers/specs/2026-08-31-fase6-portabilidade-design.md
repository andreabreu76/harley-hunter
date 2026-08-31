# Harley Hunter — fase 6: portabilidade do runtime

Data: 2026-08-31. Primeira das quatro fases que transformam o projeto de
ferramenta pessoal em macOS num programa que outra pessoa instala no computador
dela. A decomposição das quatro fases está em
`~/Documents/Claude/harley-hunter/2026-08-31-decomposicao-multiplataforma.md`.

## Objetivo

Tirar o macOS de dentro do código sem mudar nada do que o projeto faz. Ao fim
desta fase, o mesmo binário compila e roda em macOS, Linux e Windows, coletando
as mesmas seis fontes, com as mesmas regras de veredito, dedup e alerta.

O que a fase **não** faz: escolher moto por tela, gerar URL de busca, instalar
nada e falar com o GitHub. Isso é das fases 7, 8 e 9.

## O que muda

| Hoje | Depois |
|---|---|
| `launchd` dispara `hunter crawl` a cada 12h | `hunter serve` fica no ar e dispara a coleta sozinho |
| `deploy/hunter-crawl.sh` sobe o Chrome com caminho fixo do macOS | `internal/browser` acha e sobe o Chrome nos três sistemas |
| Alerta por `terminal-notifier`/`osascript` | `notify.New()` escolhe a implementação por `runtime.GOOS` |
| `config.yaml` e `hunter.db` na pasta do repositório | diretório do aplicativo, por sistema operacional |
| `config.Load` recusa config incompleto | `Load` lê, `Validate` julga: o daemon sobe, mas não coleta |
| `devtools_url` com padrão `127.0.0.1:9222` | vazio significa "o daemon gerencia o Chrome" |

Não muda: o esquema do banco, as regras de `match`, o fingerprint de dedup, a
âncora de preço, o dashboard, o formato do alerta, nem os parsers das fontes.

## O daemon

`hunter serve` deixa de ser só o dashboard. Passa a rodar duas coisas no mesmo
processo: o servidor HTTP em `127.0.0.1:8080` e um agendador.

O agendador **não** é um `time.Ticker` de doze horas. O daemon morre e renasce a
cada logout, e um ticker ingênuo faria uma coleta a cada login ou perderia a
janela inteira. Ele acorda a cada minuto e toma uma decisão pura a partir do
estado no banco:

| Situação | Decisão |
|---|---|
| `Validate` falha no config atual | não coleta |
| Nunca houve rodada | coleta |
| Passou `interval_hours` desde a última rodada | coleta |
| Não passou | não coleta |
| Erro ao ler a última rodada | coleta, com aviso no stderr |

O intervalo deixa de ser o `StartInterval` do plist e vira dado: `interval_hours`
entra em `crawl`, com padrão de doze horas, como o spec de 2026-08-10 desenhou.
Inteiro em horas, e não minutos, porque é o que o sistema consegue cumprir com
honestidade.

A última linha repete o princípio já firmado no `due` do spec de 2026-08-10:
entre perder uma coleta em silêncio e fazer uma coleta a mais, o projeto
coleta. "Última rodada" é `MAX(started_at)` em `source_runs`, sem filtrar por
status — uma rodada que falhou consumiu o ciclo, e insistir de minuto em minuto
contra um site que está bloqueando não ajuda ninguém.

O subcomando `due`, desenhado no spec de 2026-08-10, **não será implementado**.
Ele existia para um shell script decidir por código de saída se valia subir o
Chrome. Com o agendador em processo, a decisão é uma função em Go, testável com
relógio injetado, e o script deixa de existir.

Acordar de minuto em minuto tem uma segunda consequência boa: o agendador relê
o config a cada decisão. Editar o arquivo à mão passa a valer sem reiniciar o
daemon, e a fase 8 ganha isso de graça — a tela grava o arquivo e o agendador
obedece no minuto seguinte, sem nenhum canal entre os dois.

O dashboard lê o config pelo mesmo caminho: `NewServer` deixa de receber
`cfg.Sources` no boot e passa a consultar o config quando renderiza. Sem isso, a
tela mostraria para sempre a lista de fontes que existia quando o processo
subiu, e a fase 8 precisaria de um canal entre o handler que grava e a página
que lê. O arquivo é a fonte de verdade; ninguém guarda cópia longa em memória.

Duas coletas nunca correm juntas: o agendador segura uma flag enquanto coleta e
simplesmente pula os ticks que caem no meio de uma rodada longa. `SIGINT` e
`SIGTERM` (e o `os.Interrupt` do Windows) fecham o Chrome que o daemon subiu e
o banco antes de sair.

Dashboard e coleta dividem o mesmo `*store.Store`. Isso já é seguro: o DSN em
`store.go:121` abre em WAL com `busy_timeout(5000)`, então a leitura do
dashboard convive com a gravação da rodada.

`crawl` e `repair-silenced` continuam como comandos one-shot, para depuração e
para a operação manual que o `repair-silenced` sempre exigiu.

## Onde as coisas moram

Um diretório por usuário guarda tudo — config, banco e perfil do Chrome:

| Sistema | Diretório |
|---|---|
| macOS | `~/Library/Application Support/harley-hunter` |
| Linux | `$XDG_DATA_HOME/harley-hunter`, ou `~/.local/share/harley-hunter` |
| Windows | `%LOCALAPPDATA%\harley-hunter` |

Um diretório só, e não a divisão canônica entre config, dados e cache. Três
arquivos relacionados, um usuário, uma pasta: quem for dar suporte a um amigo
por telefone precisa de um caminho para pedir, não de três.

No Windows é `LOCALAPPDATA` e não `APPDATA` de propósito: `APPDATA` é o perfil
que sincroniza entre máquinas em ambiente corporativo, e um SQLite em WAL
sincronizado por rede é corrupção esperando acontecer.

A variável `HARLEY_HUNTER_HOME` sobrepõe tudo. Serve aos testes e ao autor, que
tem um banco com histórico fora desse caminho.

Resolução do config, em ordem: a flag `-config`, depois
`$HARLEY_HUNTER_HOME/config.yaml`, depois `<diretório do app>/config.yaml`. Se
o arquivo não existir, o daemon escreve um esqueleto com as seis fontes ligadas,
sem alvo e sem URLs, e sobe assim mesmo. Um `database_path` ausente vira
`hunter.db` ao lado do config, e a regra de resolver caminho relativo contra o
diretório do arquivo continua valendo, como sempre valeu.

`hunter paths` entra como subcomando novo: imprime onde estão config, banco,
perfil do Chrome e logs. É a primeira pergunta de qualquer suporte, e a resposta
não pode depender de o usuário saber o que é `LOCALAPPDATA`.

## Logs

Hoje quem redireciona a saída para `~/Library/Logs/harley-hunter.log` é o plist
do `launchd`. Um daemon que roda em três sistemas não pode depender disso.

O processo escreve em `stdout` e `stderr`, como já escreve, e passa a duplicar
em `<diretório do app>/logs/hunter.log`, com rotação por tamanho. Quem roda no
terminal continua vendo tudo; quem roda pelo autostart tem onde procurar. É o
caminho que `hunter paths` imprime, e é o que se pede a alguém por telefone.

## `Load` lê, `Validate` julga

Hoje `config.Load` (`internal/config/config.go:35`) recusa um arquivo sem
fontes, sem anos de match ou com preço zero. Numa máquina recém-instalada isso
travaria o daemon antes de ele servir a tela — e sem tela não há como
configurar. A validação vira função própria, como o spec de 2026-08-10 já
previa, e os dois caminhos passam a divergir de propósito:

- `Load` lê o arquivo, aplica os padrões e resolve caminhos. Só falha em YAML
  quebrado ou arquivo ilegível.
- `Validate` responde se este config permite coletar. O agendador chama antes de
  cada rodada; `crawl` one-shot chama e recusa na cara do usuário; a fase 8
  transforma o erro em painel de pendências.

O daemon sobe com config incompleto, serve o dashboard e não coleta. É
exatamente o estado de uma instalação nova.

## `internal/browser`

O pacote novo substitui o `deploy/hunter-crawl.sh` inteiro, com duas
responsabilidades e nada mais.

**`Locate`** procura o executável, em ordem: Google Chrome, Chromium, Microsoft
Edge. No macOS e no Windows, nos caminhos conhecidos de instalação; no Linux, no
`PATH` (`google-chrome`, `google-chrome-stable`, `chromium`,
`chromium-browser`, `microsoft-edge`). Não achar nada é um erro que diz o que
instalar, não um `exec: not found`.

**`Launch`** sobe o processo com o perfil dedicado, espera `/json/version`
responder dentro de um timeout e devolve um handle que sabe se encerrar.

Três regras governam o ciclo de vida:

1. **O perfil nunca é descartado.** Vive em `<diretório do app>/chrome-profile`
   e é ele que guarda a sessão do Instagram entre rodadas. Apagar o perfil é
   deslogar o usuário.
2. **O daemon só mata o que subiu.** Se `devtools_url` está preenchido no config
   e o endereço responde, ele reaproveita e não encerra nada ao fim — que é o
   modo de operação do autor hoje. Vazio significa "gerencie para mim".
3. **A porta é livre, não 9222.** O daemon pede uma porta ao sistema no momento
   de subir. A porta fixa colidiria com um Chrome de depuração que o próprio
   usuário tenha aberto.

O navegador é verificado no início de cada rodada, não uma vez na vida do
daemon: um Chrome que o usuário fechou à mão, ou que morreu sozinho, é
relançado na coleta seguinte em vez de derrubar todas as fontes com o mesmo
erro de conexão.

Uma divergência de plataforma fica registrada: hoje a janela é escondida em
`--window-position=-32000,-32000`, o que exige sessão gráfica. Sem `DISPLAY`
nem `WAYLAND_DISPLAY`, o Launch cai para `--headless=new`. Instagram e Facebook
detectam headless com mais facilidade que uma janela real, então num Linux sem
interface essas duas fontes podem render menos que num desktop. É degradação
conhecida, não bug.

## Notificação por sistema

`notify.New()` escolhe a implementação por `runtime.GOOS`, e todas usam a mesma
injeção de executor que o `MacOS` já usa hoje — os testes verificam comando e
argumentos sem disparar notificação nenhuma.

| Sistema | Mecanismo | Clique abre o anúncio |
|---|---|---|
| macOS | `terminal-notifier`, com `osascript` de reserva | sim, como hoje |
| Linux | `notify-send`, com a URL no corpo | não prometido |
| Windows | toast por PowerShell e WinRT | não nesta fase |

No Linux a ação de clique depende do daemon de notificação do ambiente e varia
entre GNOME e KDE; prometer isso seria prometer o que não se controla. No
Windows, sem um `AppUserModelID` registrado, o toast aparece como se viesse do
PowerShell e não tem ativação. A fase 9 corrige o Windows criando um atalho no
Menu Iniciar com AppID próprio, o que dá nome ao remetente e permite o clique.

A biblioteca `beeep` foi considerada e descartada: uniformizar as três
plataformas numa implementação só custaria o clique também no macOS, que é onde
ele funciona hoje e onde o autor usa o programa.

**Falha de notificação não perde anúncio, e isso não é código novo.**
`crawl.Notify` (`internal/crawl/crawl.go:192`) aborta quando `Send` devolve
erro, **sem** chamar `MarkNotified` — a linha continua pendente e sai na rodada
seguinte. Por isso os notifiers novos devem falhar alto: um Linux sem
`libnotify` acumula a fila em vez de esvaziá-la em silêncio, e quando o usuário
instalar o pacote os alertas saem, limitados por `max_alerts_per_run`.

## O banco do autor

O `hunter.db` na raiz do repositório é o único banco com histórico real: âncoras
de preço, o que já foi notificado, o ciclo de vida de centenas de anúncios.

Não haverá detecção automática de banco legado. Uma heurística que procura
`hunter.db` no diretório atual erra em silêncio, e o preço do erro é re-alertar
seiscentos anúncios de uma vez ou, pior, começar do zero e perder as âncoras.

Vira procedimento no README, em três passos: descarregar o agente `launchd`,
copiar `hunter.db`, `hunter.db-wal` e `hunter.db-shm` para o diretório do
aplicativo, subir o daemon. Os três arquivos, não só o primeiro — o banco roda
em WAL e copiar apenas o `.db` descarta as transações que ainda não foram
integradas.

## Arquivos afetados

| Arquivo | Mudança |
|---|---|
| `internal/paths/paths.go` | novo: diretório do app por sistema, com `HARLEY_HUNTER_HOME` |
| `internal/browser/locate.go` | novo: descoberta do executável por sistema |
| `internal/browser/launch.go` | novo: subir, esperar o endpoint, encerrar o que subiu |
| `internal/notify/linux.go` | novo: `notify-send` |
| `internal/notify/windows.go` | novo: toast por PowerShell |
| `internal/notify/notify.go` | `New()` seleciona por `runtime.GOOS` |
| `internal/config/config.go` | `Validate` separado de `Load`; `IntervalHours`; padrões de `database_path` e `devtools_url`; escrita do esqueleto |
| `internal/schedule/schedule.go` | novo: a decisão de coletar, com relógio injetado |
| `internal/store/store.go` | `LastRunStartedAt()`, o `MAX(started_at)` global |
| `cmd/hunter/main.go` | `serve` vira daemon; `paths`; resolução do config; encerramento por sinal |
| `internal/web/server.go` | `NewServer` lê o config sob demanda em vez de receber `Sources` no boot |
| `deploy/hunter-crawl.sh` | removido |
| `deploy/com.andreabreu.harleyhunter.plist` | vira autostart do daemon, sem `StartInterval`; o autostart de Linux e Windows é da fase 9 |
| `internal/logging/logging.go` | novo: saída duplicada em arquivo, com rotação por tamanho |
| `Makefile` | alvos que não presumem `~/Library` nem `launchctl`; build cruzado |
| `README.md` | daemon, diretórios por sistema, migração do banco |

## Testes

Todos red-first, e nenhum dispara notificação, sobe navegador ou toca o banco de
produção.

1. `paths.AppDir` devolve o caminho certo em cada sistema, com o ambiente
   injetado, e `HARLEY_HUNTER_HOME` sobrepõe os três.
2. `Load` aceita um arquivo sem fontes, sem anos e sem preço, e ainda assim
   devolve um `Config` utilizável.
3. `Validate` recusa: sem fonte ativa, sem ano de match, preço zero, fonte ativa
   sem URL. Uma mensagem por caso.
4. `Load` aplica os padrões: `database_path` ausente vira `hunter.db` ao lado do
   config; caminho relativo resolve contra o diretório do arquivo.
5. O esqueleto escrito numa pasta vazia recarrega com `Load` sem erro.
6. Agendador com relógio injetado, caso a caso da tabela do daemon: nunca
   rodou, venceu, não venceu, config inválido, erro de leitura.
7. O agendador pula o tick que cai durante uma coleta em andamento.
8. `browser.Locate` com candidatos e `PATH` injetados, um caso por sistema;
   nenhum encontrado devolve erro que nomeia o que instalar.
9. `browser.Launch` com executor injetado: as flags certas, a espera pelo
   endpoint, `Close` encerra o processo que subiu.
10. `Launch` com endpoint já respondendo não sobe processo nenhum, e `Close` não
    encerra o navegador de terceiro.
11. Sem `DISPLAY` nem `WAYLAND_DISPLAY` em Linux, as flags incluem
    `--headless=new`.
12. Notificadores de Linux e Windows: comando e argumentos exatos, com executor
    injetado.
13. Erro do executor vira erro retornado pelo `Send`, para que `crawl.Notify`
    não marque a linha como notificada.
14. `notify.New` escolhe a implementação de cada sistema.
15. `LastRunStartedAt` devolve o maior `started_at` entre todas as fontes,
    incluindo rodadas com status de erro, e distingue "nunca houve rodada" de
    "houve à meia-noite".

16. O arquivo de log recebe o mesmo texto que o `stdout`, e o arquivo é rotado
    quando passa do tamanho máximo, sem perder a linha que estourou o limite.

Verificação de plataforma, além dos testes: `GOOS=windows`, `GOOS=linux` e
`GOOS=darwin`, em `amd64` e `arm64`, compilando no `make cross`. Compilar não é
rodar, e a fase termina com uma execução real em Linux e outra em Windows.

## Riscos

- **Headless no Linux muda o que Instagram e Facebook devolvem.** Conhecido e
  aceito; a fase 8 precisa mostrar quando uma fonte volta vazia.
- **Toast do Windows sem clique e com remetente errado.** Some na fase 9.
- **Chrome ausente na máquina do usuário.** A mensagem de erro precisa dizer o
  que instalar; a fase 8 transforma isso em pendência visível.
- **O padrão de `devtools_url` muda de sentido.** O config do autor tem a chave
  preenchida, então ele continua no modo de navegador externo até apagar a
  linha. Quem tiver copiado o config antigo herda o mesmo comportamento.
- **A cópia do banco do autor.** Manual, documentada, e com os três arquivos.

## Fora de escopo

Catálogo de modelos, gerador de URL de busca e regiões parametrizadas são da
fase 7. Tela de configuração, painel de pendências e login social pela interface
são da fase 8. Instalador, GoReleaser, CI e o README voltado a terceiros são da
fase 9 — nesta fase o README só ganha o que mudou de lugar. O backlog técnico
das fases 4 e 5 (FIPE sem fallback, colisão de fingerprint entre fontes, dedup
entre rodadas) segue no handoff de 2026-08-10 e não entra aqui.

## Convenções da casa

Código sem comentários. TDD red-first: o teste falha primeiro, pelo motivo
esperado, e só então a implementação. Dinheiro em centavos `int64`. Ausente é
ponteiro nil. Commits em inglês, descrevendo o comportamento e não o diff.
Documentação voltada ao dono em português. `.env` e `*.db` nunca entram em
commit. O merge em `main` é `--no-ff`, com `merge: fase 6 do harley-hunter`,
depois de `go build ./... && go vet ./... && go test ./...`.
