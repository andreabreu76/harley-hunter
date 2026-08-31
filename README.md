# harley-hunter

Robô que vigia anúncios de Harley-Davidson Street Glide e Road Glide em seis
marketplaces brasileiros, guarda o que encontra num SQLite e avisa por
notificação da área de trabalho quando aparece um anúncio dentro do alvo. Roda
em macOS, Linux e Windows.

## Como funciona

A cada rodada o hunter abre as URLs de busca configuradas para cada fonte, lê os
anúncios, normaliza o que veio (preço em centavos, ano, quilometragem, cidade) e
classifica cada um em três baldes, segundo os critérios do config:

- **match** — ano e preço dentro do alvo;
- **maybe** — ano ou preço na faixa de tolerância;
- **rejeitado** — fora, ou nem é a moto certa.

Anúncio novo em match, e queda de preço abaixo do menor valor já comunicado,
viram notificação na área de trabalho. Anúncio que some das buscas é marcado como
encerrado. Reanúncios da mesma moto são agrupados por um fingerprint grosseiro
(moto, ano, faixa de quilometragem, cidade), para que a mesma máquina reanunciada
não avise duas vezes.

Cinco das seis fontes bloqueiam HTTP puro e são lidas por um navegador de
verdade, controlado por CDP. O hunter encontra, sobe e derruba esse navegador
sozinho — veja [O navegador](#o-navegador).

## Pré-requisitos

- **Go 1.26+** para compilar. O SQLite é implementação pura em Go: não precisa de
  cgo nem de toolchain C, e é por isso que o `make cross` compila para os três
  sistemas a partir de qualquer um deles.
- **Google Chrome, Chromium ou Microsoft Edge** instalado. Sem nenhum dos três, a
  rodada falha dizendo exatamente isso.
- Notificação, conforme o sistema:
  - **macOS** — `terminal-notifier` (`brew install terminal-notifier`). Sem ele o
    alerta cai no `osascript` e o clique no banner deixa de abrir o anúncio.
  - **Linux** — `notify-send` (`libnotify-bin` no Debian e Ubuntu, `libnotify` no
    Fedora e Arch). Sem ele o alerta falha e fica pendente para a próxima rodada.
  - **Windows** — nada a instalar; o toast sai por PowerShell.

## Instalação

```bash
go build -o ~/bin/hunter ./cmd/hunter
```

ou `make build`, que faz o mesmo. Para gerar os binários dos três sistemas em
`dist/`:

```bash
make cross
```

## Onde ficam os arquivos

Nada mais mora no diretório do repositório. Config, banco, perfil do navegador e
log vivem num diretório por usuário:

| Sistema | Diretório |
|---|---|
| macOS | `~/Library/Application Support/harley-hunter` |
| Linux | `$XDG_DATA_HOME/harley-hunter`, ou `~/.local/share/harley-hunter` |
| Windows | `%LOCALAPPDATA%\harley-hunter` |

Dentro dele: `config.yaml`, `hunter.db`, `chrome-profile/` e `logs/hunter.log`.

Não é preciso decorar nada disso — o comando `paths` imprime os cinco caminhos:

```bash
hunter paths
```

```
directory: /Users/andreabreu/Library/Application Support/harley-hunter
config:    /Users/andreabreu/Library/Application Support/harley-hunter/config.yaml
database:  /Users/andreabreu/Library/Application Support/harley-hunter/hunter.db
profile:   /Users/andreabreu/Library/Application Support/harley-hunter/chrome-profile
log:       /Users/andreabreu/Library/Application Support/harley-hunter/logs/hunter.log
```

Na primeira execução de qualquer comando, se ainda não houver `config.yaml`, o
hunter escreve um esqueleto ali — com as seis fontes listadas, sem URLs e sem
critérios. Um config incompleto assim não derruba o daemon: ele sobe, serve o
dashboard e não coleta, registrando no log o que falta, até o arquivo ganhar
URLs, anos e preço máximo. Se o config quebrar depois, com o daemon já
coletando, é essa mesma linha que conta o que aconteceu — uma vez, e de novo só
quando o motivo mudar.

Dois desvios possíveis:

- `HARLEY_HUNTER_HOME=/outro/lugar` troca o diretório inteiro, em qualquer
  sistema. Serve para testar sem encostar na instalação de verdade.
- `hunter -config /caminho/config.yaml <comando>` aponta para um config fora do
  diretório. O banco então segue o `database_path` daquele arquivo — que, quando
  relativo, é resolvido a partir do diretório do próprio config, e não do
  diretório onde o comando foi chamado.

## Uso

### `serve` — o daemon

```bash
hunter serve
```

Sobe e fica de pé. É ele quem agenda a própria coleta: a cada minuto olha quando
a última rodada começou e, se já passou o intervalo, coleta. Não existe mais
script de shell nem relógio do sistema operacional no caminho — quem manda é o
`interval_hours` do config.

Junto sobe o dashboard em <http://127.0.0.1:8080>, com as abas de match, maybe,
rejeitados e a saúde de cada fonte, além das fontes ativas naquele momento.
`Ctrl+C` encerra o daemon e o dashboard.

O config é relido a quente. Mudar `interval_hours`, as fontes, as URLs ou a faixa
de preço passa a valer na checagem seguinte, sem reiniciar. Se o arquivo for
salvo quebrado, o daemon avisa no log e segue com a última versão boa.

Enquanto o `serve` está no ar, tudo que ele imprime vai também para o arquivo de
log.

### `crawl` — uma rodada só

```bash
hunter crawl
```

Coleta uma vez e sai. Serve para testar o config ou forçar uma rodada fora de
hora. Ao contrário do `serve`, imprime só no terminal.

Não rode o `crawl` com o daemon no ar: os dois disputam o mesmo perfil do Chrome.

### `repair-silenced`

Devolve à fila de alerta os anúncios que o dedup antigo calou sem avisar:

```bash
hunter repair-silenced
```

Ele lista os ids que vai liberar antes de gravar e, no fim, quantos voltaram para
a fila. A próxima rodada avisa sobre eles, respeitando o teto de alertas.

Rode **uma única vez**, num momento em que dê para acompanhar a fila — nunca no
agendamento. O comando se rearma: depois que um anúncio liberado já foi avisado,
rodar de novo o coloca outra vez na fila e gera alerta duplicado.

### `export.json`

Com o dashboard no ar, `GET /export.json` devolve o banco em JSON, para entregar
a um agente. Traz match e maybe por padrão; `?verdict=match`, `?verdict=maybe` e
`?verdict=all` estreitam ou ampliam o recorte. Cada anúncio vai com a referência
FIPE e o gap em percentual, a queda desde o primeiro preço visto, o histórico
completo e os reanúncios irmãos. O envelope leva ainda a contagem de todo o banco
e a saúde de cada fonte. `make export` grava o arquivo, e `make export VERDICT=all`
traz os descartados junto.

## Agendamento: `interval_hours`

O intervalo entre rodadas é uma linha do config, e nada mais:

```yaml
crawl:
  interval_hours: 12
```

Ausente ou zero, o padrão é 12 horas. A contagem parte do início da última rodada
gravada no banco, e não do momento em que o processo subiu: reiniciar o daemon
não zera o relógio nem provoca uma coleta extra. Num banco que nunca rodou, a
primeira coleta acontece na primeira checagem.

Como o config é relido a quente, mudar o intervalo não exige reiniciar nada.

## Autostart no macOS

O agente do `launchd` deixou de ser o relógio. O plist não tem mais
`StartInterval`: ele apenas sobe `hunter serve` no login e, com `KeepAlive`, o
levanta de novo se o processo cair.

```bash
make agent-install    # compila, copia o plist e recarrega o agente
make agent-status     # diz se o agente está carregado e se o dashboard responde
make logs             # acompanha o log do daemon
make agent-uninstall  # descarrega o agente
```

O plist também declara o `PATH` do agente, com `/opt/homebrew/bin` e
`/usr/local/bin` na frente dos diretórios do sistema. Sem isso o `launchd` entrega
um `PATH` mínimo, o `terminal-notifier` some e o clique no banner deixa de abrir o
anúncio — veja [Logs](#logs).

Depois de editar o plist é preciso repetir o `make agent-install` — o `launchd`
não relê o arquivo sozinho. Mudar o intervalo das rodadas, porém, não passa mais
por aqui: é o `interval_hours` do config.

O autostart equivalente para Linux (`systemd --user`) e Windows é da fase 9. Nos
dois, por ora, o daemon sobe à mão com `hunter serve`.

## O navegador

Com `devtools_url` vazio no config — que é o caso normal —, o hunter cuida do
navegador sozinho: procura Chrome, Chromium e Edge nos caminhos de instalação e
no `PATH`, sobe uma instância com o perfil dedicado (`chrome-profile`, dentro do
diretório do app) numa porta livre escolhida na hora, usa e derruba no fim da
rodada. A janela nasce fora da tela; no Linux sem `DISPLAY` nem `WAYLAND_DISPLAY`
ele entra em `--headless=new`.

Com `devtools_url` preenchido, o sentido se inverte: significa "eu mesmo cuido do
navegador". O hunter não sobe nem derruba nada — só confere se alguém responde
naquele endereço e usa o que estiver lá. Se ninguém responder, ele falha e pede
para limpar o campo. Use isso quando quiser um Chrome seu, aberto na tela, com a
sessão logada à vista.

O `config/config.yaml` versionado no repositório não traz mais `devtools_url`:
quem copiar esse arquivo cai no modo em que o hunter cuida do navegador sozinho.
Um config herdado da versão anterior ainda pode trazer a linha — apague-a para
voltar a esse modo.

## Instagram e Facebook Marketplace exigem sessão logada

Estas duas fontes não devolvem nada para quem não está logado — e o problema não
é a falha em si, é onde o motivo dela aparece.

**A fonte falha com nome e sobrenome, mas só o log conta o porquê.** Quando a
sessão falta ou expirou, o Instagram responde com a tela de login e a rodada
daquela fonte morre com `instagram answered with the login screen: the browser
profile is not signed in`. O Facebook faz o mesmo, com `facebook answered with
the login screen instead of marketplace results`. A frase sai no resumo da
rodada, vai para o `logs/hunter.log` e fica gravada junto com a coleta falha.

O painel de saúde, porém, não mostra esse texto. Ele lista o nome da fonte, o
estado, a última coleta e a contagem de itens por rodada — e nada mais. Como uma
rodada que falha conta como zero itens, o que se vê no painel é uma fonte que
parou de produzir: ela vira `suspect` depois de duas rodadas vazias seguidas,
desde que já viesse trazendo volume, e numa instalação em que ela nunca produziu
nada continua em `ok` com zeros. Em nenhum dos casos o painel diz por quê — e o
`export.json` carrega os mesmos campos, sem a mensagem. O log é o único lugar
onde a frase aparece.

Ou seja: quando o Instagram ou o Marketplace secam, a resposta está no log, e
quase sempre é a sessão. O conserto é logar de novo no perfil dedicado.

A sessão mora no perfil dedicado do Chrome — aquele que o `hunter paths` mostra
na linha `profile:`. Como é o mesmo perfil em toda rodada, basta logar uma vez: a
sessão sobrevive a reinício do daemon e da máquina, e expira sozinha de tempos em
tempos, quando é preciso logar de novo.

### Como logar, nesta fase

Nesta fase o login é feito à mão, abrindo o Chrome contra aquele perfil. Com o
daemon parado, porque dois processos não compartilham o mesmo perfil:

1. Pare o daemon (`Ctrl+C`, ou `make agent-uninstall` no macOS).
2. Descubra o caminho do perfil com `hunter paths`, linha `profile:`.
3. Abra o Chrome com ele, apontando o `--user-data-dir` para esse caminho:

```bash
# macOS
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --user-data-dir="$HOME/Library/Application Support/harley-hunter/chrome-profile"

# Linux
google-chrome --user-data-dir="$HOME/.local/share/harley-hunter/chrome-profile"
```

```powershell
# Windows
& "$env:ProgramFiles\Google\Chrome\Application\chrome.exe" `
  --user-data-dir="$env:LOCALAPPDATA\harley-hunter\chrome-profile"
```

Nessa janela, entre em `instagram.com` e em `facebook.com` e logue até ver o feed
de cada um. Depois feche a janela e suba o daemon de novo.

A tela que faz esse login por botão, sem linha de comando, é da **fase 8**. Até
lá é assim — não adianta procurar um botão que ainda não existe.

Se o seu config usa `devtools_url`, a sessão que conta é a do navegador que você
mantém no ar, não a do perfil dedicado: logue nele.

## Fontes e transporte

| Fonte | Transporte | Depende do navegador | Exige sessão logada |
|---|---|---|---|
| olx | CDP | sim | não |
| mercadolivre | CDP | sim | não |
| webmotors | CDP — o PerimeterX responde 403 a HTTP puro | sim | não |
| mobiauto | HTTP puro | **não** | não |
| instagram | CDP | sim | **sim** |
| marketplace | CDP | sim | **sim** |

A Mobiauto continua coletando com o navegador fora do ar: ela fala HTTP direto,
com User-Agent de navegador e timeout próprio. Numa rodada em que o Chrome não
sobe, as outras cinco falham e a Mobiauto entrega normalmente. O painel de saúde
mostra quais pararam de produzir, fonte a fonte; o motivo de cada falha, esse sai
no log.

Fonte que responde algo diferente de `200`, ou que devolve página de desafio no
lugar do payload esperado, falha alto e vira erro da rodada; não passa em branco.
O Instagram e o Marketplace sem sessão entram nessa mesma regra: a tela de login
é tratada como falha, com mensagem própria — que aparece no log, e não no painel.

## Logs

O log do daemon é o `logs/hunter.log` dentro do diretório do app — resumo por
fonte, contagem de novos matches, alertas mostrados e os erros da rodada. Ele
rotaciona ao passar de 5 MB, guardando o anterior como `hunter.log.1`. Quem roda
`hunter serve` no terminal vê o mesmo texto na tela; o `hunter crawl` só imprime
no terminal, sem escrever no arquivo.

No macOS, o agente do `launchd` manda o `stderr` do processo para
`logs/launchd.error.log`, ao lado — é lá que caem as falhas anteriores à abertura
do log, como um config ilegível ou um diretório sem permissão de escrita.

Duas linhas que aparecem no log e merecem tradução:

- `ERROR: unhandled node event ... dom.Event` é ruído do chromedp conversando com
  o DevTools, não do nosso código.
- O aviso de que o `terminal-notifier` não foi encontrado no `PATH` quer dizer que
  o alerta caiu no `osascript` — o banner ainda aparece, mas o clique deixou de
  abrir o anúncio. É uma falha silenciosa, e a causa costuma ser uma só: o `PATH`
  que o `launchd` entrega a um agente é o mínimo (`/usr/bin:/bin:/usr/sbin:/sbin`),
  onde o Homebrew não está. Por isso o plist deste repositório já declara o `PATH`
  explicitamente:

```xml
<key>EnvironmentVariables</key>
<dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
</dict>
```

  Se o aviso aparecer mesmo assim, o `terminal-notifier` está fora desses
  diretórios: acrescente o dele à lista e refaça o `make agent-install`.

## Migração do banco existente

Só vale para quem já rodava a versão anterior, com o banco na raiz do repositório.
Com o agente parado, o banco e o config vão para o diretório novo:

```bash
launchctl bootout gui/$(id -u)/com.andreabreu.harleyhunter
mkdir -p "$HOME/Library/Application Support/harley-hunter"
cp hunter.db hunter.db-wal hunter.db-shm "$HOME/Library/Application Support/harley-hunter/"
cp config/config.yaml "$HOME/Library/Application Support/harley-hunter/"
```

São os **três** arquivos do banco, e não só o `.db`: o SQLite roda em modo WAL, e
copiar apenas o primeiro descarta as transações que ainda não foram integradas —
ou seja, joga fora o histórico recente de preços e de alertas já enviados, que é
justamente o que impede o hunter de reavisar tudo de novo.

O agente fica para o fim, e é de propósito: com `RunAtLoad` e `KeepAlive` ele
começa a coletar no instante em que é carregado, e um config que ainda aponte
para fora do diretório do app o levaria a abrir um banco vazio — perdendo as
âncoras de preço e o histórico de alertas. Antes de carregá-lo, abra o config
novo e confira que não sobrou nenhuma destas duas linhas. O
`config/config.yaml` do repositório já vem sem as duas, mas uma cópia editada à
mão pode trazê-las:

- `database_path`, que apontaria para o banco antigo, na raiz do repositório;
  sem ela o hunter usa o `hunter.db` ao lado do config, que é o que você acabou
  de copiar;
- `devtools_url`, herança do Chrome que se subia à mão; sem ela o hunter sobe e
  derruba o navegador sozinho.

Confira o resultado com `hunter paths`: a linha `database:` tem que apontar para
o banco dentro do diretório do app. Só então carregue o agente:

```bash
make agent-install
```

## Documentação

As especificações e os planos de cada fase estão em
`docs/superpowers/specs/` e `docs/superpowers/plans/`.
