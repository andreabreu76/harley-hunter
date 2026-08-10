# Harley Hunter — configuração visual no dashboard

Data: 2026-08-10. Decidido com o dono: tela de config no dashboard, gravando no
próprio `config.yaml`, mais o intervalo de checagem saindo do plist do launchd.

O gráfico de FIPE de 6 meses foi **descartado** — ver "Fora de escopo".

## Objetivo

Hoje toda mudança de critério exige abrir o `config.yaml` no editor, e o
intervalo de checagem exige editar o plist e recarregar o launchd. A tela troca
isso por um formulário em `/config`, sem tirar do arquivo a condição de fonte de
verdade: quem edita o yaml à mão continua sendo obedecido.

Não é um painel de administração. É um formulário para um único usuário, num
dashboard que só escuta em `127.0.0.1`.

## O que a tela edita

| Campo | Origem hoje | Observação |
|---|---|---|
| Anos que contam como match | `match.years` | lista de inteiros |
| Anos que contam como maybe | `match.maybe_years` | lista de inteiros |
| Preço máximo de match | `match.max_price_cents` | exibido em reais, gravado em centavos |
| Preço máximo de maybe | `match.maybe_max_price_cents` | idem |
| Veredito por modelo | **hardcoded em `match.go`** | ver abaixo |
| Fontes ativas | `sources` | checkbox por fonte |
| Alertas por rodada | `crawl.max_alerts_per_run` | inteiro |
| Timeout por fonte | `crawl.timeout_seconds` | inteiro |
| Concorrência | `crawl.max_concurrent` | inteiro |
| Intervalo de checagem | **plist do launchd** | ver abaixo |

**Somente leitura na tela:** as URLs de `source_urls`. São longas e específicas
por fonte — a do Webmotors é uma URL de API com query inteira encodada — e um
campo de texto livre ali é a forma mais fácil de quebrar a coleta sem perceber.
A tela mostra quantas URLs cada fonte tem e permite ligar ou desligar a fonte;
editar URL continua sendo trabalho de arquivo.

`database_path` e `devtools_url` também ficam de fora: mudar o caminho do banco
por um formulário é como trocar o chão embaixo dos pés do processo.

## Veredito por modelo

Hoje `match.evaluateBike` (`internal/match/match.go:22`) é uma tabela fixa:
Street Glide e Road Glide são match; Electra Glide, Ultra e touring não
identificada são maybe; o resto é reject.

Passa a vir do config:

```yaml
match:
  bikes: [street_glide, road_glide]
  maybe_bikes: [electra_glide, ultra, touring_unknown]
```

Na tela, cada um dos cinco modelos conhecidos ganha um seletor de três posições:
**match**, **maybe** ou **ignorar**. `other` não aparece — é a categoria de
"não é uma touring da Harley" e permanece reject sempre.

Compatibilidade: quando as duas chaves estão ausentes do yaml, `evaluateBike`
usa exatamente a tabela de hoje. Um config antigo continua funcionando sem
edição, e o comportamento não muda por baixo de quem não pediu.

Consequência que o dono precisa saber: mudar o veredito de um modelo **não
reclassifica o que já está no banco**. O veredito é gravado na linha em
`Upsert`, então só anúncios revistos numa rodada seguinte adotam a regra nova.
Ligar um modelo novo não faz aparecerem, de imediato, os anúncios que já foram
rejeitados por ele.

## Intervalo de checagem

Hoje o intervalo é `StartInterval` de 7200s no plist
`com.andreabreu.harleyhunter`. Alterar pela tela exigiria o dashboard reescrever
`~/Library/LaunchAgents` e chamar `launchctl` — permissão e diagnóstico que um
formulário não deveria carregar.

O intervalo vira dado, e o launchd vira relógio fixo:

1. `crawl.interval_hours` entra no yaml (padrão 12).
2. O plist passa a acordar **de hora em hora** e nunca mais muda.
3. Um subcomando novo, `hunter due`, responde por código de saída se já passou o
   intervalo desde a última rodada registrada.
4. `deploy/hunter-crawl.sh` só sobe o Chrome e coleta quando `due` autoriza.

Granularidade de uma hora é o piso real: o launchd acorda de hora em hora, então
`interval_hours` como inteiro descreve honestamente o que o sistema pode cumprir.
Não haverá campo em minutos prometendo precisão que não existe.

### Contrato do `hunter due`

| Situação | Exit | Motivo |
|---|---|---|
| Passou `interval_hours` desde a última rodada | 0 | está na hora |
| Não passou | 1 | ainda não, como o `grep` que não achou nada |
| Nunca houve rodada | 0 | primeira coleta |
| Erro ao ler o banco | 0, com mensagem no stderr | erra para o lado de coletar |

A última linha é deliberada e segue o princípio da fase 3: entre perder uma
coleta em silêncio e fazer uma coleta a mais, o projeto escolhe coletar. Uma
falha transitória de banco não deve parar a caça.

"Última rodada" é `MAX(started_at)` em `source_runs`, sem filtrar por status —
uma rodada que falhou ainda consumiu o ciclo, e insistir de hora em hora contra
um site que está bloqueando não ajuda.

O subcomando recebe o mesmo `-config` que os outros (`hunter -config <path>
due`), porque precisa do `interval_hours` do yaml e do `database_path` para
achar o banco. Nome novo no store para não colidir com o `LastRunAt(source)` que
o `/health` já usa: `LastRunStartedAt() (time.Time, bool, error)`, onde o
booleano distingue "nunca houve rodada" de "houve às zero horas".

## Como o formulário grava

`POST /config` → valida → grava → recarrega em memória → redireciona para
`GET /config` com o resultado.

**Validação antes de gravar.** `config.Load` hoje valida depois de ler o
arquivo (`internal/config/config.go:35`): exige `database_path`, ao menos uma
fonte, ao menos um ano de match, preço máximo positivo, e URLs para toda fonte
ativa. Essa validação é extraída para `config.Validate(Config) error` e passa a
ser usada nos dois caminhos — no `Load` e no handler. Um formulário inválido
volta com a mensagem e **não toca o arquivo**: o crawl nunca pode ficar sem
poder subir por causa de um clique no dashboard.

**Escrita atômica.** `os.CreateTemp` no mesmo diretório do config, escreve o
YAML, `fsync`, `os.Rename` sobre o original. O rename é atômico no mesmo
filesystem, então um crawl que esteja lendo o arquivo nesse instante lê a versão
antiga inteira ou a nova inteira, nunca metade.

Custo aceito: o arquivo reescrito por `yaml.Marshal` perde comentários e a ordem
original das chaves. O `config.yaml` atual não tem comentários, então a perda é
de formatação, não de informação.

**Recarga em memória.** O dashboard é um processo longo e hoje recebe
`cfg.Sources` uma única vez (`web.NewServer`, `internal/web/server.go:103`).
Depois de gravar, o servidor precisa passar a servir os valores novos, senão a
tela mostra o que acabou de ser substituído. `NewServer` passa a receber a
`Config` inteira e o caminho do arquivo, e guarda os dois atrás de um mutex de
leitura/escrita — o handler de gravação é o único escritor, e as páginas de
listagem que hoje leem `sources` passam a ler pelo mesmo acessor.

## Arquivos afetados

| Arquivo | Mudança |
|---|---|
| `internal/config/config.go` | `Validate`, `CrawlSettings.IntervalHours`, `MatchCriteria.Bikes`/`MaybeBikes`, e uma função de escrita atômica |
| `internal/match/match.go` | `evaluateBike` passa a consultar o config, com a tabela de hoje como padrão |
| `internal/store/store.go` | `LastRunStartedAt()`, o `MAX(started_at)` global |
| `internal/web/server.go` | `NewServer` recebe `Config` + caminho; rotas `GET /config` e `POST /config`; config guardado sob mutex |
| `cmd/hunter/main.go` | subcomando `due`; `runServe` passa o config e o caminho |
| `deploy/hunter-crawl.sh` | consulta `due` antes de subir o Chrome |
| `deploy/com.andreabreu.harleyhunter.plist` | `StartInterval` de 7200 para 3600 |
| `config/config.yaml` | `interval_hours: 12`, `bikes`, `maybe_bikes` |
| `README.md` | a tela, o `due`, e que o intervalo agora é do yaml |

`internal/web/server.go` já tem 515 linhas e vai crescer com o formulário. O
handler de config, o parse do formulário e o template ganham arquivo próprio
(`internal/web/config.go`), em vez de engordar o `server.go`.

## Testes

1. `Validate` rejeita: sem fonte, sem ano de match, preço zero, fonte ativa sem
   URL — e o handler devolve o erro sem escrever o arquivo.
2. Escrita atômica: o arquivo resultante recarrega com `Load` e produz o mesmo
   `Config`; nenhum arquivo temporário sobra no diretório.
3. `POST /config` válido grava e o `GET /config` seguinte mostra os valores
   novos, sem reiniciar o processo.
4. `POST /config` inválido: arquivo intacto byte a byte, mensagem na tela.
5. `evaluateBike` com config vazio reproduz a tabela de hoje, modelo a modelo.
6. `evaluateBike` com um modelo movido de maybe para match muda só aquele eixo.
7. `due` com a última rodada dentro do intervalo → exit 1; fora → exit 0;
   banco vazio → exit 0.
8. `due` com erro de banco → exit 0 e mensagem no stderr.
9. `LastRunAt()` devolve o maior `started_at` entre todas as fontes, incluindo
   rodadas com status de erro.
10. O formulário preenche os campos com os valores em vigor (uma tela que abre
    vazia convida a salvar um config zerado).

## Fora de escopo

**Gráfico de 6 meses do valor FIPE — descartado, com motivo registrado.**
Três achados durante a investigação: (a) `SaveFipeReference` faz upsert em
`UNIQUE(bike,variant,year)`, então cada refresh sobrescreve o mês anterior e não
existe histórico no banco; (b) a API pública devolve apenas três meses para trás
— testado em 2026-08-10: agosto R$ 69.207, julho R$ 68.476, junho R$ 69.308 na
Street Glide 2014, e de maio para trás responde *"apenas assinantes pagos podem
acessar o histórico de preços estendido"*; (c) a cobertura é irregular — das 32
combinações de modelo, variante e ano, só 14 existem, Street Glide base só tem
2013-14, Special e CVO só 2015-16, e Road Glide não existe em nenhum ano de
2013-16. Um gráfico honesto nasceria com três pontos, para algumas combinações
só, e completaria seis meses em três meses de acumulação.

Também fora: edição das URLs de busca, edição de `database_path` e
`devtools_url`, autenticação (o dashboard segue em `127.0.0.1`), reclassificação
retroativa do banco ao mudar o veredito de um modelo, e qualquer forma de
histórico de alterações do config.

## Convenções da casa

Código sem comentários. TDD red-first. Dinheiro em centavos `int64` no arquivo e
no código, convertido para reais só na exibição. Ausente = ponteiro nil. Commits
em inglês, sem menção a IA ou sessão. `.env` e `*.db` nunca staged.
