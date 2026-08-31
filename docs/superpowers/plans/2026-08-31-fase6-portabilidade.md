# Fase 6 — Portabilidade do runtime: plano de implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** O mesmo binário do harley-hunter roda em macOS, Linux e Windows, com o processo `serve` virando um daemon que agenda a própria coleta, acha e gerencia o Chrome sozinho, notifica pelo mecanismo nativo de cada sistema e guarda config, banco e perfil num diretório por usuário.

**Architecture:** Cinco pacotes novos e pequenos (`paths`, `logging`, `schedule`, `browser`, e dois notificadores dentro de `notify`) removem do `cmd/hunter` e do shell script tudo que presumia macOS. O `serve` passa a rodar servidor HTTP e agendador no mesmo processo, decidindo a cada minuto se coleta, a partir do estado no banco e do config lido do disco. Nada do domínio muda: parsers, veredito, dedup, âncora de preço e dashboard ficam como estão.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` em WAL, `chromedp` sobre CDP, `gopkg.in/yaml.v3`, biblioteca padrão para processo, sinal e arquivo. Nenhuma dependência nova.

**Spec:** `docs/superpowers/specs/2026-08-31-fase6-portabilidade-design.md`

## Global Constraints

- Código sem comentários. Nomes carregam a intenção.
- TDD red-first: o teste falha primeiro, rodado para ver falhar pelo motivo esperado, e só então a implementação.
- Dinheiro em centavos `int64`. Ausente é ponteiro nil.
- Commits em inglês, descrevendo o comportamento e não o diff. Sem menção a IA, sem `Co-Authored-By`, sem link de sessão.
- Documentação voltada ao dono em português, com acentuação correta.
- `.env` e `*.db` nunca entram em commit. Conferir `git status` antes de cada commit.
- Trabalho na branch `fase-6`, que já existe. O merge em `main` é `--no-ff` com `merge: fase 6 do harley-hunter`, depois de `go build ./... && go vet ./... && go test ./...`.
- Nenhum teste sobe navegador, dispara notificação ou toca `hunter.db`. Executor, relógio e ambiente são sempre injetados.
- Nome do diretório do aplicativo: `harley-hunter`. Variável de override: `HARLEY_HUNTER_HOME`.
- O intervalo padrão de coleta é 12 horas; o tick do agendador é de 1 minuto.

## Estrutura de arquivos

| Arquivo | Responsabilidade |
|---|---|
| `internal/paths/paths.go` | Onde ficam config, banco, perfil do Chrome e log, por sistema |
| `internal/logging/logging.go` | Arquivo de log com rotação por tamanho |
| `internal/config/config.go` | Ler, aplicar padrões e validar; escrever o esqueleto |
| `internal/config/watcher.go` | Reler o arquivo quando o `mtime` muda, servindo agendador e dashboard |
| `internal/schedule/schedule.go` | Decidir se está na hora de coletar, e o laço que executa |
| `internal/browser/locate.go` | Achar Chrome, Chromium ou Edge |
| `internal/browser/launch.go` | Subir com perfil dedicado, esperar o endpoint, encerrar o que subiu |
| `internal/notify/linux.go` | `notify-send` |
| `internal/notify/windows.go` | Toast por PowerShell e WinRT |
| `internal/notify/notify.go` | `New()` escolhe por `runtime.GOOS` |
| `internal/store/store.go` | `LastRunStartedAt()` |
| `internal/web/server.go` | Lista de fontes lida sob demanda |
| `cmd/hunter/main.go` | Comandos, resolução do config, daemon, sinais |

---

### Task 1: Diretório do aplicativo

**Files:**
- Create: `internal/paths/paths.go`
- Test: `internal/paths/paths_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `paths.AppDir() (string, error)`, `paths.ConfigFile(dir string) string`, `paths.DatabaseFile(dir string) string`, `paths.ChromeProfile(dir string) string`, `paths.LogFile(dir string) string`.

- [ ] **Step 1: Escrever o teste que falha**

```go
package paths

import (
	"path/filepath"
	"testing"
)

func envOf(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func TestAppDirPerOperatingSystem(t *testing.T) {
	cases := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "macos uses application support",
			goos: "darwin",
			env:  map[string]string{"HOME": "/Users/rider"},
			want: filepath.Join("/Users/rider", "Library", "Application Support", "harley-hunter"),
		},
		{
			name: "windows uses local appdata and not roaming",
			goos: "windows",
			env:  map[string]string{"LOCALAPPDATA": `C:\Users\rider\AppData\Local`, "APPDATA": `C:\Users\rider\AppData\Roaming`},
			want: filepath.Join(`C:\Users\rider\AppData\Local`, "harley-hunter"),
		},
		{
			name: "linux honours xdg data home",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/rider", "XDG_DATA_HOME": "/home/rider/.data"},
			want: filepath.Join("/home/rider/.data", "harley-hunter"),
		},
		{
			name: "linux falls back to local share",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/rider"},
			want: filepath.Join("/home/rider", ".local", "share", "harley-hunter"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := appDir(c.goos, envOf(c.env))
			if err != nil {
				t.Fatalf("appDir: %v", err)
			}
			if got != c.want {
				t.Errorf("appDir = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAppDirOverrideWinsOnEveryOperatingSystem(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		env := envOf(map[string]string{
			"HARLEY_HUNTER_HOME": "/tmp/hunter-test",
			"HOME":               "/Users/rider",
			"LOCALAPPDATA":       `C:\Users\rider\AppData\Local`,
		})
		got, err := appDir(goos, env)
		if err != nil {
			t.Fatalf("appDir on %s: %v", goos, err)
		}
		if got != "/tmp/hunter-test" {
			t.Errorf("appDir on %s = %q, want the override", goos, got)
		}
	}
}

func TestAppDirFailsWhenTheSystemHasNoHome(t *testing.T) {
	if _, err := appDir("darwin", envOf(nil)); err == nil {
		t.Fatal("appDir with no HOME returned no error")
	}
	if _, err := appDir("windows", envOf(nil)); err == nil {
		t.Fatal("appDir with no LOCALAPPDATA returned no error")
	}
}

func TestFileHelpersHangOffTheDirectory(t *testing.T) {
	dir := filepath.Join("/tmp", "hunter")
	if got, want := ConfigFile(dir), filepath.Join(dir, "config.yaml"); got != want {
		t.Errorf("ConfigFile = %q, want %q", got, want)
	}
	if got, want := DatabaseFile(dir), filepath.Join(dir, "hunter.db"); got != want {
		t.Errorf("DatabaseFile = %q, want %q", got, want)
	}
	if got, want := ChromeProfile(dir), filepath.Join(dir, "chrome-profile"); got != want {
		t.Errorf("ChromeProfile = %q, want %q", got, want)
	}
	if got, want := LogFile(dir), filepath.Join(dir, "logs", "hunter.log"); got != want {
		t.Errorf("LogFile = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/paths/ -run TestAppDir -v`
Expected: FAIL na compilação, `undefined: appDir`.

- [ ] **Step 3: Implementar o mínimo**

```go
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "harley-hunter"

const overrideEnv = "HARLEY_HUNTER_HOME"

func AppDir() (string, error) {
	return appDir(runtime.GOOS, os.Getenv)
}

func appDir(goos string, env func(string) string) (string, error) {
	if custom := env(overrideEnv); custom != "" {
		return custom, nil
	}
	switch goos {
	case "darwin":
		home := env("HOME")
		if home == "" {
			return "", fmt.Errorf("no HOME in the environment: cannot tell where %s should keep its files", appName)
		}
		return filepath.Join(home, "Library", "Application Support", appName), nil
	case "windows":
		base := env("LOCALAPPDATA")
		if base == "" {
			return "", fmt.Errorf("no LOCALAPPDATA in the environment: cannot tell where %s should keep its files", appName)
		}
		return filepath.Join(base, appName), nil
	default:
		if base := env("XDG_DATA_HOME"); base != "" {
			return filepath.Join(base, appName), nil
		}
		home := env("HOME")
		if home == "" {
			return "", fmt.Errorf("no HOME in the environment: cannot tell where %s should keep its files", appName)
		}
		return filepath.Join(home, ".local", "share", appName), nil
	}
}

func ConfigFile(dir string) string { return filepath.Join(dir, "config.yaml") }

func DatabaseFile(dir string) string { return filepath.Join(dir, "hunter.db") }

func ChromeProfile(dir string) string { return filepath.Join(dir, "chrome-profile") }

func LogFile(dir string) string { return filepath.Join(dir, "logs", "hunter.log") }
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/paths/ -v`
Expected: PASS em todos.

- [ ] **Step 5: Commit**

```bash
git add internal/paths
git commit -m "feat: the hunter knows where its files live on each operating system"
```

---

### Task 2: `Load` lê, `Validate` julga

**Files:**
- Modify: `internal/config/config.go:35-79`
- Modify: `internal/config/config_test.go:61-94`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `config.Load(path string) (Config, error)` sem regras de negócio; `config.Validate(cfg Config) error`; `config.EnsureFile(path string) error`; campo `CrawlSettings.IntervalHours int` com tag `yaml:"interval_hours"`.

**Contexto para quem implementa:** hoje `Load` recusa config sem fontes, sem anos ou com preço zero. Isso impede o daemon de subir numa máquina recém-instalada, e sem daemon não existe tela para configurar. A validação sai do caminho de leitura e vira função própria. Três testes existentes mudam de alvo: `TestLoadRejectsEmptyMatchCriteria` e `TestLoadRejectsEnabledSourceWithoutURLs` passam a exercitar `Validate`; `TestLoadRejectsAnEmptyDatabasePath` vira teste de padrão, porque `database_path` ausente passa a virar `hunter.db` ao lado do config.

- [ ] **Step 1: Escrever o teste que falha**

```go
func TestLoadAcceptsAnIncompleteConfigSoTheDaemonCanStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("sources:\n  - olx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load on an incomplete config returned error: %v", err)
	}
	if want := filepath.Join(dir, "hunter.db"); cfg.DatabasePath != want {
		t.Errorf("DatabasePath = %q, want the default next to the config", cfg.DatabasePath)
	}
	if cfg.Crawl.IntervalHours != 12 {
		t.Errorf("IntervalHours = %d, want the default of 12", cfg.Crawl.IntervalHours)
	}
	if cfg.DevtoolsURL != "" {
		t.Errorf("DevtoolsURL = %q, want empty so the daemon manages Chrome", cfg.DevtoolsURL)
	}
}

func TestValidateRejectsWhatCannotCollect(t *testing.T) {
	base := Config{
		DatabasePath: "/tmp/hunter.db",
		Sources:      []string{"olx"},
		SourceURLs:   map[string][]string{"olx": {"https://www.olx.com.br/x"}},
		Match:        MatchCriteria{Years: []int{2014}, MaxPriceCents: 7500000},
	}
	if err := Validate(base); err != nil {
		t.Fatalf("Validate on a complete config returned error: %v", err)
	}

	cases := []struct {
		name  string
		break_ func(*Config)
		want  string
	}{
		{"no source", func(c *Config) { c.Sources = nil }, "sources"},
		{"no match year", func(c *Config) { c.Match.Years = nil }, "year"},
		{"no max price", func(c *Config) { c.Match.MaxPriceCents = 0 }, "price"},
		{"source without urls", func(c *Config) { c.SourceURLs = map[string][]string{} }, "olx"},
		{"no database path", func(c *Config) { c.DatabasePath = "" }, "database_path"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := base
			c.break_(&cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("Validate accepted a config with %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestEnsureFileWritesASkeletonThatLoadsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := EnsureFile(path); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load of the skeleton returned error: %v", err)
	}
	if len(cfg.Sources) != 6 {
		t.Errorf("skeleton enabled %d sources, want the six known ones", len(cfg.Sources))
	}
	if err := Validate(cfg); err == nil {
		t.Error("Validate accepted the skeleton, but a fresh install has no target yet")
	}
}

func TestEnsureFileLeavesAnExistingConfigAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("sources:\n  - olx\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureFile(path); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Errorf("EnsureFile rewrote an existing config:\n%s", after)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/config/ -v`
Expected: FAIL na compilação, `undefined: Validate` e `undefined: EnsureFile`.

- [ ] **Step 3: Implementar o mínimo**

Em `internal/config/config.go`, `CrawlSettings` ganha o campo e `Load` perde as regras de negócio:

```go
const defaultIntervalHours = 12

type CrawlSettings struct {
	IntervalHours   int `yaml:"interval_hours"`
	TimeoutSeconds  int `yaml:"timeout_seconds"`
	MaxConcurrent   int `yaml:"max_concurrent"`
	MaxAlertsPerRun int `yaml:"max_alerts_per_run"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Config{}, fmt.Errorf("resolving the config directory: %w", err)
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = filepath.Join(dir, "hunter.db")
	} else if !filepath.IsAbs(cfg.DatabasePath) {
		cfg.DatabasePath = filepath.Join(dir, cfg.DatabasePath)
	}
	if cfg.Crawl.IntervalHours <= 0 {
		cfg.Crawl.IntervalHours = defaultIntervalHours
	}
	return cfg, nil
}

func Validate(cfg Config) error {
	if cfg.DatabasePath == "" {
		return fmt.Errorf("config has no database_path: sqlite would open a throwaway database and every round would re-notify the same listings")
	}
	if len(cfg.Sources) == 0 {
		return fmt.Errorf("config has no sources enabled")
	}
	if len(cfg.Match.Years) == 0 {
		return fmt.Errorf("config has no target year")
	}
	if cfg.Match.MaxPriceCents <= 0 {
		return fmt.Errorf("config has no max price")
	}
	for _, name := range cfg.Sources {
		if len(cfg.SourceURLs[name]) == 0 {
			return fmt.Errorf("source %q is enabled but has no urls under source_urls", name)
		}
	}
	return nil
}
```

O `defaultDevtoolsURL` e sua atribuição saem: campo vazio passa a significar que o daemon gerencia o navegador.

Em `internal/config/skeleton.go`:

```go
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const skeleton = `sources:
  - olx
  - mercadolivre
  - webmotors
  - mobiauto
  - instagram
  - marketplace
source_urls: {}
match:
  years: []
  maybe_years: []
  max_price_cents: 0
  maybe_max_price_cents: 0
crawl:
  interval_hours: 12
  timeout_seconds: 240
  max_concurrent: 4
  max_alerts_per_run: 5
`

func EnsureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking the config file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating the config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(skeleton), 0o644); err != nil {
		return fmt.Errorf("writing the first config: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Migrar os três testes que mudaram de alvo**

`TestLoadRejectsEmptyMatchCriteria` e `TestLoadRejectsEnabledSourceWithoutURLs` são cobertos pelos casos de `TestValidateRejectsWhatCannotCollect` e devem ser removidos. `TestLoadRejectsAnEmptyDatabasePath` é substituído pela asserção de padrão em `TestLoadAcceptsAnIncompleteConfigSoTheDaemonCanStart`. `TestLoadReadsSourceURLsAndDevtoolsURL` precisa perder a expectativa de `devtools_url` padrão quando a chave está ausente — o `testdata/config.yaml` traz a chave explícita, então confirme com `grep devtools internal/config/testdata/config.yaml` antes de mexer.

- [ ] **Step 5: Rodar os testes para ver passar**

Run: `go test ./internal/config/ -v && go build ./...`
Expected: PASS. O `go build` falha em `cmd/hunter/main.go` se algo ainda depender do comportamento antigo — se falhar, o conserto é da Task 10 e pode ficar para lá; nesse caso rode `go vet ./internal/...`.

- [ ] **Step 6: Commit**

```bash
git add internal/config
git commit -m "feat: an incomplete config no longer stops the hunter from starting"
```

---

### Task 3: Config relido quando o arquivo muda

**Files:**
- Create: `internal/config/watcher.go`
- Test: `internal/config/watcher_test.go`

**Interfaces:**
- Consumes: `config.Load`, `config.Config` da Task 2.
- Produces: `config.NewWatcher(path string) (*config.Watcher, error)`, `(*Watcher).Current() Config`, `(*Watcher).Path() string`.

**Contexto para quem implementa:** o agendador decide de minuto em minuto e o dashboard renderiza a qualquer momento. Os dois precisam do config atual sem que ninguém guarde uma cópia velha. O `Watcher` relê o arquivo quando o `mtime` mudou e, se a releitura falhar, continua servindo o último config válido — um YAML salvo pela metade não pode derrubar a coleta.

- [ ] **Step 1: Escrever o teste que falha**

```go
package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(time.Duration(len(body)) * time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherPicksUpAnEditedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfig(t, path, "sources:\n  - olx\n")

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if got := w.Current().Sources; len(got) != 1 || got[0] != "olx" {
		t.Fatalf("Sources = %v, want [olx]", got)
	}

	writeConfig(t, path, "sources:\n  - olx\n  - webmotors\n")
	if got := w.Current().Sources; len(got) != 2 {
		t.Errorf("Sources = %v, want the two from the edited file", got)
	}
}

func TestWatcherKeepsTheLastGoodConfigWhenTheFileBreaks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfig(t, path, "sources:\n  - olx\n")

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	var warned bytes.Buffer
	w.warn = &warned

	writeConfig(t, path, "sources: [unclosed\n")
	if got := w.Current().Sources; len(got) != 1 || got[0] != "olx" {
		t.Errorf("Sources = %v, want the last good config", got)
	}
	if warned.Len() == 0 {
		t.Error("a broken config was swallowed without a warning")
	}
}

func TestWatcherRefusesAMissingFileUpFront(t *testing.T) {
	if _, err := NewWatcher(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("NewWatcher accepted a path with no file")
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/config/ -run TestWatcher -v`
Expected: FAIL na compilação, `undefined: NewWatcher`.

- [ ] **Step 3: Implementar o mínimo**

```go
package config

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Watcher struct {
	path    string
	mu      sync.RWMutex
	current Config
	modTime time.Time
	warn    io.Writer
}

func NewWatcher(path string) (*Watcher, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	w := &Watcher{path: path, current: cfg, warn: os.Stderr}
	if info, err := os.Stat(path); err == nil {
		w.modTime = info.ModTime()
	}
	return w, nil
}

func (w *Watcher) Path() string { return w.path }

func (w *Watcher) Current() Config {
	info, err := os.Stat(w.path)
	if err != nil {
		return w.snapshot()
	}
	w.mu.RLock()
	unchanged := info.ModTime().Equal(w.modTime)
	w.mu.RUnlock()
	if unchanged {
		return w.snapshot()
	}

	cfg, err := Load(w.path)
	if err != nil {
		fmt.Fprintf(w.warn, "config at %s is unreadable, keeping the last good one: %v\n", w.path, err)
		w.mu.Lock()
		w.modTime = info.ModTime()
		w.mu.Unlock()
		return w.snapshot()
	}

	w.mu.Lock()
	w.current = cfg
	w.modTime = info.ModTime()
	w.mu.Unlock()
	return cfg
}

func (w *Watcher) snapshot() Config {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.current
}
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/config/ -v`
Expected: PASS em todos, inclusive nos da Task 2.

- [ ] **Step 5: Commit**

```bash
git add internal/config/watcher.go internal/config/watcher_test.go
git commit -m "feat: editing the config file no longer needs a restart"
```

---
### Task 4: A hora da última rodada

**Files:**
- Modify: `internal/store/store.go` (ao lado de `LastRunAt`, hoje em `:361`)
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `(*store.Store).LastRunStartedAt() (time.Time, bool, error)`.

**Contexto para quem implementa:** `LastRunAt(source)` já existe e devolve o `finished_at` da última rodada de uma fonte. O agendador precisa de outra coisa: o `started_at` mais recente entre todas as fontes, sem filtrar por status, porque uma rodada que falhou consumiu o ciclo igual. `MAX()` numa tabela vazia devolve uma linha com `NULL`, e não `sql.ErrNoRows` — por isso o `sql.NullTime`. O helper `openTemp(t)` já existe em `store_test.go:12`.

- [ ] **Step 1: Escrever o teste que falha**

```go
func TestLastRunStartedAtIsEmptyBeforeTheFirstRound(t *testing.T) {
	s := openTemp(t)
	at, ever, err := s.LastRunStartedAt()
	if err != nil {
		t.Fatalf("LastRunStartedAt: %v", err)
	}
	if ever {
		t.Errorf("a fresh database reported a previous round at %s", at)
	}
}

func TestLastRunStartedAtTakesTheLatestAcrossSourcesAndStatuses(t *testing.T) {
	s := openTemp(t)
	old := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 8, 31, 7, 30, 0, 0, time.UTC)

	if err := s.RecordRun("olx", old, old.Add(time.Minute), 12, "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRun("instagram", recent, recent.Add(time.Minute), 0, "error", "blocked"); err != nil {
		t.Fatal(err)
	}

	at, ever, err := s.LastRunStartedAt()
	if err != nil {
		t.Fatalf("LastRunStartedAt: %v", err)
	}
	if !ever {
		t.Fatal("LastRunStartedAt found no round after two were recorded")
	}
	if !at.Equal(recent) {
		t.Errorf("LastRunStartedAt = %s, want the failed round at %s", at, recent)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/store/ -run TestLastRunStartedAt -v`
Expected: FAIL na compilação, `s.LastRunStartedAt undefined`.

- [ ] **Step 3: Implementar o mínimo**

```go
func (s *Store) LastRunStartedAt() (time.Time, bool, error) {
	var at sql.NullTime
	if err := s.db.QueryRow("SELECT MAX(started_at) FROM source_runs").Scan(&at); err != nil {
		return time.Time{}, false, fmt.Errorf("querying the last round: %w", err)
	}
	if !at.Valid {
		return time.Time{}, false, nil
	}
	return at.Time, true, nil
}
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/store/ -v`
Expected: PASS, inclusive os testes que já existiam.

- [ ] **Step 5: Commit**

```bash
git add internal/store
git commit -m "feat: the store answers when the last round started"
```

---

### Task 5: O agendador

**Files:**
- Create: `internal/schedule/schedule.go`
- Test: `internal/schedule/schedule_test.go`

**Interfaces:**
- Consumes: `config.Watcher` e `config.Validate` (Tasks 2 e 3); `(*store.Store).LastRunStartedAt` (Task 4).
- Produces: `schedule.Due(now, lastRun time.Time, everRan bool, interval time.Duration) bool`; `schedule.Runner` com campos `Config *config.Watcher`, `LastRun func() (time.Time, bool, error)`, `Collect func(config.Config) error`, `Now func() time.Time`, `Tick time.Duration`, `Warn io.Writer`; e `(*Runner).Run(ctx context.Context) error`.

**Contexto para quem implementa:** o daemon morre e renasce a cada logout, então o agendador não pode ser um `time.Ticker` de doze horas — ele acorda de minuto em minuto e decide a partir do banco. Duas coletas nunca correm juntas porque o laço é sequencial: `Collect` bloqueia o laço, e o tick que ficar pendente durante uma rodada longa cai num `Due` que já responde `false`. Não há flag nem goroutine.

Há uma armadilha para fechar: se `Collect` falhar antes de `crawl.Run` gravar em `source_runs`, o banco não registra nada e o próximo tick tentaria de novo, um minuto depois, contra o mesmo site que acabou de recusar. Por isso o `Runner` guarda em memória a hora da última tentativa e decide pelo mais recente entre ela e o banco.

- [ ] **Step 1: Escrever o teste que falha**

```go
package schedule

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
)

func TestDue(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	interval := 12 * time.Hour

	cases := []struct {
		name    string
		lastRun time.Time
		everRan bool
		want    bool
	}{
		{"never ran", time.Time{}, false, true},
		{"ran eleven hours ago", now.Add(-11 * time.Hour), true, false},
		{"ran exactly the interval ago", now.Add(-12 * time.Hour), true, true},
		{"ran thirteen hours ago", now.Add(-13 * time.Hour), true, true},
		{"clock went backwards", now.Add(time.Hour), true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Due(now, c.lastRun, c.everRan, interval); got != c.want {
				t.Errorf("Due = %v, want %v", got, c.want)
			}
		})
	}
}

func watcherFor(t *testing.T, body string) *config.Watcher {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := config.NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	return w
}

const completeConfig = `sources:
  - olx
source_urls:
  olx:
    - https://www.olx.com.br/x
match:
  years: [2014]
  max_price_cents: 7500000
crawl:
  interval_hours: 12
`

func runOnce(t *testing.T, r *Runner) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunnerCollectsWhenTheIntervalHasPassed(t *testing.T) {
	collected := 0
	r := &Runner{
		Config:  watcherFor(t, completeConfig),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error { collected++; return nil },
		Now:     time.Now,
		Tick:    time.Millisecond,
		Warn:    io.Discard,
	}
	runOnce(t, r)
	if collected == 0 {
		t.Fatal("the runner never collected on a fresh database")
	}
}

func TestRunnerStaysQuietWhileTheConfigCannotCollect(t *testing.T) {
	collected := 0
	r := &Runner{
		Config:  watcherFor(t, "sources:\n  - olx\n"),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error { collected++; return nil },
		Now:     time.Now,
		Tick:    time.Millisecond,
		Warn:    io.Discard,
	}
	runOnce(t, r)
	if collected != 0 {
		t.Errorf("the runner collected %d times with an incomplete config", collected)
	}
}

func TestRunnerCollectsWhenTheDatabaseCannotBeRead(t *testing.T) {
	collected := 0
	r := &Runner{
		Config:  watcherFor(t, completeConfig),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, errors.New("database is locked") },
		Collect: func(config.Config) error { collected++; return nil },
		Now:     time.Now,
		Tick:    time.Millisecond,
		Warn:    io.Discard,
	}
	runOnce(t, r)
	if collected == 0 {
		t.Fatal("a database error stopped the hunt instead of erring towards collecting")
	}
}

func TestRunnerDoesNotRetryEveryTickAfterAFailedRound(t *testing.T) {
	collected := 0
	r := &Runner{
		Config:  watcherFor(t, completeConfig),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error { collected++; return errors.New("chrome is gone") },
		Now:     time.Now,
		Tick:    time.Millisecond,
		Warn:    io.Discard,
	}
	runOnce(t, r)
	if collected != 1 {
		t.Errorf("collected %d times after a failure that recorded no round, want 1", collected)
	}
}

func TestRunnerDoesNotOverlapRounds(t *testing.T) {
	inFlight := 0
	overlapped := false
	r := &Runner{
		Config:  watcherFor(t, completeConfig),
		LastRun: func() (time.Time, bool, error) { return time.Time{}, false, nil },
		Collect: func(config.Config) error {
			inFlight++
			if inFlight > 1 {
				overlapped = true
			}
			time.Sleep(20 * time.Millisecond)
			inFlight--
			return nil
		},
		Now:  time.Now,
		Tick: time.Millisecond,
		Warn: io.Discard,
	}
	runOnce(t, r)
	if overlapped {
		t.Error("two rounds ran at the same time")
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/schedule/ -v`
Expected: FAIL na compilação, `undefined: Due` e `undefined: Runner`.

- [ ] **Step 3: Implementar o mínimo**

```go
package schedule

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
)

const defaultTick = time.Minute

func Due(now, lastRun time.Time, everRan bool, interval time.Duration) bool {
	if !everRan {
		return true
	}
	return now.Sub(lastRun) >= interval
}

type Runner struct {
	Config      *config.Watcher
	LastRun     func() (time.Time, bool, error)
	Collect     func(config.Config) error
	Now         func() time.Time
	Tick        time.Duration
	Warn        io.Writer
	lastAttempt time.Time
}

func (r *Runner) Run(ctx context.Context) error {
	tick := r.Tick
	if tick <= 0 {
		tick = defaultTick
	}
	if r.Warn == nil {
		r.Warn = io.Discard
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.step()
		}
	}
}

func (r *Runner) step() {
	cfg := r.Config.Current()
	if err := config.Validate(cfg); err != nil {
		return
	}

	now := r.Now()
	lastRun, everRan, err := r.LastRun()
	if err != nil {
		fmt.Fprintf(r.Warn, "cannot tell when the last round started, collecting anyway: %v\n", err)
		lastRun, everRan = time.Time{}, false
	}
	if !r.lastAttempt.IsZero() && r.lastAttempt.After(lastRun) {
		lastRun, everRan = r.lastAttempt, true
	}

	interval := time.Duration(cfg.Crawl.IntervalHours) * time.Hour
	if !Due(now, lastRun, everRan, interval) {
		return
	}

	r.lastAttempt = now
	if err := r.Collect(cfg); err != nil {
		fmt.Fprintf(r.Warn, "round failed: %v\n", err)
	}
}
```

A comparação é `now.Sub(lastRun) >= interval` e não uma diferença em valor absoluto: um relógio que andou para trás produz duração negativa, que fica abaixo de qualquer intervalo e devolve `false`. É o caso `clock went backwards` do teste, e o comportamento certo — ajuste de horário não é motivo para coletar.

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/schedule/ -v`
Expected: PASS em todos os seis.

- [ ] **Step 5: Commit**

```bash
git add internal/schedule
git commit -m "feat: the hunter decides on its own when the next round is due"
```

---

### Task 6: Log em arquivo

**Files:**
- Create: `internal/logging/logging.go`
- Test: `internal/logging/logging_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `logging.Open(path string, maxBytes int64) (*logging.File, error)`, com `Write([]byte) (int, error)` e `Close() error`.

**Contexto para quem implementa:** quem redireciona a saída hoje é o plist do `launchd`, e isso não existe em Linux nem Windows. O processo passa a duplicar `stdout` e `stderr` num arquivo dentro do diretório do aplicativo. A rotação é a mais simples que funciona: ao estourar o tamanho, o arquivo atual vira `hunter.log.1`, substituindo o anterior, e um novo é aberto. Duas gerações bastam para diagnosticar uma coleta que deu errado ontem.

- [ ] **Step 1: Escrever o teste que falha**

```go
package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesTheDirectoryAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "hunter.log")
	f, err := Open(path, 1024)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := f.Write([]byte("first round\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	again, err := Open(path, 1024)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if _, err := again.Write([]byte("second round\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	again.Close()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "first round") || !strings.Contains(string(body), "second round") {
		t.Errorf("log lost a round:\n%s", body)
	}
}

func TestWriteRotatesAndKeepsTheLineThatOverflowed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hunter.log")
	f, err := Open(path, 32)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	if _, err := f.Write([]byte(strings.Repeat("a", 30) + "\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := f.Write([]byte("the line that overflowed\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "the line that overflowed") {
		t.Errorf("the overflowing line was dropped:\n%s", current)
	}

	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("reading the rotated file: %v", err)
	}
	if !strings.Contains(string(rotated), "aaa") {
		t.Errorf("the rotated file lost the earlier lines:\n%s", rotated)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/logging/ -v`
Expected: FAIL na compilação, `undefined: Open`.

- [ ] **Step 3: Implementar o mínimo**

```go
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type File struct {
	path string
	max  int64
	mu   sync.Mutex
	file *os.File
	size int64
}

func Open(path string, maxBytes int64) (*File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating the log directory: %w", err)
	}
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening the log file: %w", err)
	}
	info, err := handle.Stat()
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("sizing the log file: %w", err)
	}
	return &File{path: path, max: maxBytes, file: handle, size: info.Size()}, nil
}

func (f *File) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.max > 0 && f.size+int64(len(p)) > f.max {
		if err := f.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := f.file.Write(p)
	f.size += int64(n)
	return n, err
}

func (f *File) rotate() error {
	if err := f.file.Close(); err != nil {
		return fmt.Errorf("closing the log file before rotating: %w", err)
	}
	if err := os.Rename(f.path, f.path+".1"); err != nil {
		return fmt.Errorf("rotating the log file: %w", err)
	}
	handle, err := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening the log file after rotating: %w", err)
	}
	f.file = handle
	f.size = 0
	return nil
}

func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Close()
}
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/logging/ -v`
Expected: PASS nos dois.

- [ ] **Step 5: Commit**

```bash
git add internal/logging
git commit -m "feat: the hunter keeps its own log file"
```

---

### Task 7: Achar o navegador

**Files:**
- Create: `internal/browser/locate.go`
- Test: `internal/browser/locate_test.go`

**Interfaces:**
- Consumes: nada.
- Produces: `browser.Locate() (string, error)`, e a função interna `locate(goos string, env func(string) string, lookPath func(string) (string, error), exists func(string) bool) (string, error)`.

**Contexto para quem implementa:** hoje o caminho do Chrome é uma constante do macOS dentro de `deploy/hunter-crawl.sh`. A busca é por ordem de preferência: Google Chrome, Chromium, Microsoft Edge. Em macOS e Windows são caminhos de instalação conhecidos; em Linux é o `PATH`. Não achar nada precisa dizer o que instalar — um `exec: not found` no log não ajuda quem está com o programa parado.

- [ ] **Step 1: Escrever o teste que falha**

```go
package browser

import (
	"errors"
	"strings"
	"testing"
)

func envOf(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func existsOnly(paths ...string) func(string) bool {
	present := make(map[string]bool, len(paths))
	for _, p := range paths {
		present[p] = true
	}
	return func(path string) bool { return present[path] }
}

func noLookPath(string) (string, error) { return "", errors.New("not in PATH") }

func TestLocateFindsChromeOnMacOS(t *testing.T) {
	chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	got, err := locate("darwin", envOf(nil), noLookPath, existsOnly(chrome))
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != chrome {
		t.Errorf("locate = %q, want %q", got, chrome)
	}
}

func TestLocatePrefersChromeOverEdge(t *testing.T) {
	chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	edge := "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"
	got, err := locate("darwin", envOf(nil), noLookPath, existsOnly(edge, chrome))
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != chrome {
		t.Errorf("locate = %q, want Chrome to win over Edge", got)
	}
}

func TestLocateFindsChromeOnWindows(t *testing.T) {
	env := envOf(map[string]string{"ProgramFiles": `C:\Program Files`})
	want := `C:\Program Files\Google\Chrome\Application\chrome.exe`
	got, err := locate("windows", env, noLookPath, existsOnly(want))
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != want {
		t.Errorf("locate = %q, want %q", got, want)
	}
}

func TestLocateUsesThePathOnLinux(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "chromium" {
			return "/usr/bin/chromium", nil
		}
		return "", errors.New("not in PATH")
	}
	got, err := locate("linux", envOf(nil), lookPath, existsOnly())
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != "/usr/bin/chromium" {
		t.Errorf("locate = %q, want the chromium found in PATH", got)
	}
}

func TestLocateSaysWhatToInstallWhenNothingIsThere(t *testing.T) {
	_, err := locate("linux", envOf(nil), noLookPath, existsOnly())
	if err == nil {
		t.Fatal("locate found a browser on an empty machine")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "chrome") {
		t.Errorf("error = %q, want it to name the browser to install", err)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/browser/ -v`
Expected: FAIL na compilação, `undefined: locate`.

- [ ] **Step 3: Implementar o mínimo**

```go
package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Locate() (string, error) {
	return locate(runtime.GOOS, os.Getenv, exec.LookPath, func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	})
}

func locate(goos string, env func(string) string, lookPath func(string) (string, error), exists func(string) bool) (string, error) {
	for _, candidate := range candidates(goos, env) {
		if exists(candidate) {
			return candidate, nil
		}
	}
	for _, name := range pathNames(goos) {
		if found, err := lookPath(name); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("no browser found: install Google Chrome, Chromium or Microsoft Edge and start the hunter again")
}

func candidates(goos string, env func(string) string) []string {
	switch goos {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	case "windows":
		var paths []string
		for _, base := range []string{env("ProgramFiles"), env("ProgramFiles(x86)"), env("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			paths = append(paths,
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "Chromium", "Application", "chrome.exe"),
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
		return paths
	default:
		return nil
	}
}

func pathNames(goos string) []string {
	if goos == "windows" {
		return nil
	}
	return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"}
}
```

Note que no macOS a busca em `PATH` também vale como reserva, para quem instalou por outro caminho. A ordem de `candidates` é a ordem de preferência, e o teste que prefere Chrome a Edge trava isso.

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/browser/ -v`
Expected: PASS nos cinco.

- [ ] **Step 5: Commit**

```bash
git add internal/browser
git commit -m "feat: the hunter finds chrome, chromium or edge on any system"
```

---
### Task 8: Subir e encerrar o navegador

**Files:**
- Create: `internal/browser/launch.go`
- Test: `internal/browser/launch_test.go`

**Interfaces:**
- Consumes: `browser.locate` da Task 7.
- Produces: `browser.Options{ExecutablePath, ProfileDir, ExistingURL string; Headless bool}`, `browser.Launch(ctx context.Context, opts Options) (*Handle, error)`, `(*Handle).DevtoolsURL() string`, `(*Handle).Owned() bool`, `(*Handle).Close() error`, `browser.HeadlessNeeded(goos string, env func(string) string) bool`, `browser.Flags(opts Options, port int) []string`.

**Contexto para quem implementa:** este pacote substitui o `deploy/hunter-crawl.sh`. Três regras vêm do spec e cada uma tem teste: o perfil nunca é descartado, porque é ele que guarda a sessão do Instagram; o daemon só encerra o processo que ele mesmo subiu; e a porta é pedida ao sistema em vez de ser 9222 fixa, para não colidir com um Chrome de depuração do próprio usuário.

Quando `ExistingURL` está preenchido e não responde, o resultado é erro e não um navegador novo: quem escreveu `devtools_url` no config pediu um navegador externo, e subir outro por baixo seria desobedecer em silêncio.

- [ ] **Step 1: Escrever o teste que falha**

```go
package browser

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFlagsCarryTheProfileAndTheHiddenWindow(t *testing.T) {
	got := Flags(Options{ProfileDir: "/tmp/profile"}, 9333)
	joined := strings.Join(got, " ")

	for _, want := range []string{
		"--remote-debugging-port=9333",
		"--user-data-dir=/tmp/profile",
		"--no-first-run",
		"--no-default-browser-check",
		"--window-position=-32000,-32000",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("flags miss %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "--headless") {
		t.Errorf("a windowed launch asked for headless:\n%s", joined)
	}
}

func TestFlagsGoHeadlessWithoutAScreen(t *testing.T) {
	joined := strings.Join(Flags(Options{ProfileDir: "/tmp/profile", Headless: true}, 9333), " ")
	if !strings.Contains(joined, "--headless=new") {
		t.Errorf("headless launch missed the flag:\n%s", joined)
	}
	if strings.Contains(joined, "--window-position") {
		t.Errorf("headless launch still asked for a window position:\n%s", joined)
	}
}

func TestHeadlessNeededOnlyOnLinuxWithoutADisplay(t *testing.T) {
	empty := func(string) string { return "" }
	withX11 := func(key string) string {
		if key == "DISPLAY" {
			return ":0"
		}
		return ""
	}
	withWayland := func(key string) string {
		if key == "WAYLAND_DISPLAY" {
			return "wayland-0"
		}
		return ""
	}

	if !HeadlessNeeded("linux", empty) {
		t.Error("a Linux server without a display was not sent to headless")
	}
	if HeadlessNeeded("linux", withX11) {
		t.Error("a Linux desktop on X11 was sent to headless")
	}
	if HeadlessNeeded("linux", withWayland) {
		t.Error("a Linux desktop on Wayland was sent to headless")
	}
	if HeadlessNeeded("darwin", empty) || HeadlessNeeded("windows", empty) {
		t.Error("macOS or Windows was sent to headless")
	}
}

func TestLaunchReusesAnExistingBrowserAndNeverKillsIt(t *testing.T) {
	started := 0
	killed := 0
	d := deps{
		start:    func(*exec.Cmd) error { started++; return nil },
		kill:     func(*exec.Cmd) error { killed++; return nil },
		probe:    func(string) error { return nil },
		freePort: func() (int, error) { return 9333, nil },
		sleep:    func(time.Duration) {},
	}

	h, err := launch(context.Background(), Options{ExistingURL: "http://127.0.0.1:9222"}, d)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if started != 0 {
		t.Errorf("started %d browsers while one was already answering", started)
	}
	if h.Owned() {
		t.Error("the handle claims a browser it did not start")
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if killed != 0 {
		t.Errorf("Close killed a browser it did not start")
	}
}

func TestLaunchFailsWhenTheConfiguredBrowserIsNotAnswering(t *testing.T) {
	d := deps{
		start:    func(*exec.Cmd) error { t.Fatal("started a browser instead of failing"); return nil },
		kill:     func(*exec.Cmd) error { return nil },
		probe:    func(string) error { return errors.New("connection refused") },
		freePort: func() (int, error) { return 9333, nil },
		sleep:    func(time.Duration) {},
	}

	_, err := launch(context.Background(), Options{ExistingURL: "http://127.0.0.1:9222"}, d)
	if err == nil {
		t.Fatal("launch accepted a devtools_url that answers nothing")
	}
	if !strings.Contains(err.Error(), "devtools_url") {
		t.Errorf("error = %q, want it to name the setting that has to change", err)
	}
}

func TestLaunchStartsAndClosesItsOwnBrowser(t *testing.T) {
	started := 0
	killed := 0
	answers := false
	d := deps{
		start:    func(*exec.Cmd) error { started++; answers = true; return nil },
		kill:     func(*exec.Cmd) error { killed++; return nil },
		probe: func(string) error {
			if answers {
				return nil
			}
			return errors.New("connection refused")
		},
		freePort: func() (int, error) { return 9444, nil },
		sleep:    func(time.Duration) {},
	}

	h, err := launch(context.Background(), Options{ExecutablePath: "/usr/bin/chromium", ProfileDir: "/tmp/profile"}, d)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if started != 1 {
		t.Errorf("started %d browsers, want 1", started)
	}
	if got, want := h.DevtoolsURL(), "http://127.0.0.1:9444"; got != want {
		t.Errorf("DevtoolsURL = %q, want %q", got, want)
	}
	if !h.Owned() {
		t.Error("the handle does not claim the browser it started")
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if killed != 1 {
		t.Errorf("Close killed %d browsers, want 1", killed)
	}
}

func TestLaunchKillsTheBrowserThatNeverAnswers(t *testing.T) {
	killed := 0
	d := deps{
		start:    func(*exec.Cmd) error { return nil },
		kill:     func(*exec.Cmd) error { killed++; return nil },
		probe:    func(string) error { return errors.New("connection refused") },
		freePort: func() (int, error) { return 9444, nil },
		sleep:    func(time.Duration) {},
	}

	if _, err := launch(context.Background(), Options{ExecutablePath: "/usr/bin/chromium", ProfileDir: "/tmp/profile"}, d); err == nil {
		t.Fatal("launch returned a handle for a browser that never answered")
	}
	if killed != 1 {
		t.Errorf("a browser that never answered was left running (killed = %d)", killed)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/browser/ -run 'TestFlags|TestHeadless|TestLaunch' -v`
Expected: FAIL na compilação, `undefined: Flags`, `undefined: deps`, `undefined: launch`.

- [ ] **Step 3: Implementar o mínimo**

```go
package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"
)

const (
	probeAttempts = 30
	probeInterval = time.Second
	probeTimeout  = 2 * time.Second
)

type Options struct {
	ExecutablePath string
	ProfileDir     string
	ExistingURL    string
	Headless       bool
}

type Handle struct {
	url   string
	cmd   *exec.Cmd
	kill  func(*exec.Cmd) error
	owned bool
}

type deps struct {
	start    func(*exec.Cmd) error
	kill     func(*exec.Cmd) error
	probe    func(url string) error
	freePort func() (int, error)
	sleep    func(time.Duration)
}

func Launch(ctx context.Context, opts Options) (*Handle, error) {
	return launch(ctx, opts, deps{
		start:    func(cmd *exec.Cmd) error { return cmd.Start() },
		kill:     func(cmd *exec.Cmd) error { return cmd.Process.Kill() },
		probe:    probeDevtools,
		freePort: freePort,
		sleep:    time.Sleep,
	})
}

func launch(ctx context.Context, opts Options, d deps) (*Handle, error) {
	if opts.ExistingURL != "" {
		if err := d.probe(opts.ExistingURL); err != nil {
			return nil, fmt.Errorf("nothing is answering at %s: start Chrome with a debugging port or clear devtools_url so the hunter starts one: %w", opts.ExistingURL, err)
		}
		return &Handle{url: opts.ExistingURL}, nil
	}

	port, err := d.freePort()
	if err != nil {
		return nil, fmt.Errorf("choosing a debugging port: %w", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	cmd := exec.CommandContext(ctx, opts.ExecutablePath, Flags(opts, port)...)
	if err := d.start(cmd); err != nil {
		return nil, fmt.Errorf("starting %s: %w", opts.ExecutablePath, err)
	}

	handle := &Handle{url: url, cmd: cmd, kill: d.kill, owned: true}
	for attempt := 0; attempt < probeAttempts; attempt++ {
		if err := d.probe(url); err == nil {
			return handle, nil
		}
		d.sleep(probeInterval)
	}
	handle.Close()
	return nil, fmt.Errorf("the browser never answered at %s", url)
}

func Flags(opts Options, port int) []string {
	flags := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + opts.ProfileDir,
		"--no-first-run",
		"--no-default-browser-check",
	}
	if opts.Headless {
		flags = append(flags, "--headless=new")
	} else {
		flags = append(flags, "--window-position=-32000,-32000", "--window-size=1280,900")
	}
	return append(flags, "about:blank")
}

func HeadlessNeeded(goos string, env func(string) string) bool {
	if goos != "linux" {
		return false
	}
	return env("DISPLAY") == "" && env("WAYLAND_DISPLAY") == ""
}

func (h *Handle) DevtoolsURL() string { return h.url }

func (h *Handle) Owned() bool { return h.owned }

func (h *Handle) Close() error {
	if !h.owned || h.cmd == nil || h.kill == nil {
		return nil
	}
	return h.kill(h.cmd)
}

func probeDevtools(url string) error {
	client := &http.Client{Timeout: probeTimeout}
	response, err := client.Get(url + "/json/version")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the browser answered %d at %s", response.StatusCode, url)
	}
	return nil
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/browser/ -v`
Expected: PASS nos onze testes do pacote.

- [ ] **Step 5: Commit**

```bash
git add internal/browser
git commit -m "feat: the hunter starts and stops its own chrome, keeping the logged-in profile"
```

---

### Task 9: Notificação em Linux e Windows

**Files:**
- Create: `internal/notify/linux.go`, `internal/notify/windows.go`
- Modify: `internal/notify/notify.go`
- Test: `internal/notify/linux_test.go`, `internal/notify/windows_test.go`, `internal/notify/notify_test.go`

**Interfaces:**
- Consumes: `notify.Alert` e `notify.Notifier`, que já existem.
- Produces: `notify.NewLinux() *Linux`, `notify.NewWindows() *Windows`, `notify.New() Notifier`, e a função interna `newFor(goos string) Notifier`.

**Contexto para quem implementa:** o `MacOS` em `internal/notify/macos.go` é o molde — copie o padrão do executor injetado (`run func(ctx, name string, args ...string) error`) e o helper `recorder()` de `macos_test.go:18`. Nenhum teste dispara notificação de verdade.

Duas coisas que não podem ser perdidas:

**A mensagem é dado externo.** Ela carrega o título do anúncio, escrito por um vendedor qualquer na OLX. No Windows isso entra num script de PowerShell, então a mensagem viaja por variável de ambiente e nunca por interpolação no script — aspas e crases num título não podem virar comando.

**Falhar é melhor que engolir.** `crawl.Notify` (`internal/crawl/crawl.go:192`) aborta sem marcar a linha como notificada quando `Send` devolve erro, e por isso nada se perde. Um Linux sem `notify-send` deve devolver erro dizendo o pacote a instalar, e não fingir que notificou.

- [ ] **Step 1: Escrever o teste que falha**

```go
// internal/notify/linux_test.go
package notify

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestLinuxSendsThroughNotifySend(t *testing.T) {
	calls, run := recorder()
	n := NewLinux()
	n.binaryPath = "/usr/bin/notify-send"
	n.run = run

	alert := Alert{
		Message: "Harley-Davidson Street Glide 2014 - R$ 72.000 - curitiba/PR [olx]",
		URL:     "https://pr.olx.com.br/motos/harley-1509210244",
	}
	if err := n.Send(context.Background(), alert); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("ran %d commands, want 1", len(*calls))
	}

	got := (*calls)[0]
	if got.name != "/usr/bin/notify-send" {
		t.Errorf("command = %q, want the cached notify-send path", got.name)
	}
	want := []string{
		"--app-name", "Harley Hunter",
		"--urgency", "normal",
		"Harley Hunter",
		alert.Message + "\n" + alert.URL,
	}
	if !reflect.DeepEqual(got.args, want) {
		t.Errorf("args =\n%q\nwant\n%q", got.args, want)
	}
}

func TestLinuxFailsLoudlyWithoutNotifySend(t *testing.T) {
	n := NewLinux()
	n.binaryPath = ""

	err := n.Send(context.Background(), Alert{Message: "any"})
	if err == nil {
		t.Fatal("Send reported success without notify-send installed")
	}
	if !strings.Contains(err.Error(), "notify-send") {
		t.Errorf("error = %q, want it to name the missing program", err)
	}
}

// internal/notify/windows_test.go
package notify

import (
	"context"
	"strings"
	"testing"
)

type recordedShell struct {
	name string
	env  []string
	args []string
}

func TestWindowsPassesTheMessageThroughTheEnvironment(t *testing.T) {
	var calls []recordedShell
	n := NewWindows()
	n.run = func(ctx context.Context, name string, env []string, args ...string) error {
		calls = append(calls, recordedShell{name: name, env: env, args: args})
		return nil
	}

	alert := Alert{
		Message: `Harley "Street Glide" 2014 - R$ 72.000 [olx]`,
		URL:     "https://pr.olx.com.br/motos/harley-1509210244",
	}
	if err := n.Send(context.Background(), alert); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("ran %d commands, want 1", len(calls))
	}

	got := calls[0]
	if !strings.HasPrefix(got.name, "powershell") {
		t.Errorf("command = %q, want powershell", got.name)
	}
	script := strings.Join(got.args, " ")
	if strings.Contains(script, "Street Glide") {
		t.Errorf("the listing title was interpolated into the script:\n%s", script)
	}
	if !strings.Contains(script, "HUNTER_ALERT_BODY") {
		t.Errorf("the script does not read the message from the environment:\n%s", script)
	}

	var carried bool
	for _, pair := range got.env {
		if pair == "HUNTER_ALERT_BODY="+alert.Message+"\n"+alert.URL {
			carried = true
		}
	}
	if !carried {
		t.Errorf("the message never reached the environment:\n%q", got.env)
	}
}

// internal/notify/notify_test.go
package notify

import "testing"

func TestNewForPicksTheNotifierOfEachSystem(t *testing.T) {
	if _, ok := newFor("darwin").(*MacOS); !ok {
		t.Error("darwin did not get the macOS notifier")
	}
	if _, ok := newFor("windows").(*Windows); !ok {
		t.Error("windows did not get the Windows notifier")
	}
	if _, ok := newFor("linux").(*Linux); !ok {
		t.Error("linux did not get the Linux notifier")
	}
	if _, ok := newFor("freebsd").(*Linux); !ok {
		t.Error("an unknown system did not fall back to notify-send")
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/notify/ -v`
Expected: FAIL na compilação, `undefined: NewLinux`, `undefined: NewWindows`, `undefined: newFor`.

- [ ] **Step 3: Implementar o mínimo**

```go
// internal/notify/linux.go
package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const notifySendBin = "notify-send"

type Linux struct {
	binaryPath string
	run        func(ctx context.Context, name string, args ...string) error
}

func NewLinux() *Linux {
	path, err := exec.LookPath(notifySendBin)
	if err != nil {
		path = ""
	}
	return &Linux{
		binaryPath: path,
		run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
	}
}

func (l *Linux) Send(ctx context.Context, alert Alert) error {
	if l.binaryPath == "" {
		return fmt.Errorf("%s is not installed: install libnotify-bin on Debian or Ubuntu, libnotify on Fedora or Arch, and the pending alerts appear on the next round", notifySendBin)
	}
	body := alert.Message
	if strings.HasPrefix(strings.ToLower(alert.URL), "https://") {
		body += "\n" + alert.URL
	}
	args := []string{"--app-name", alertTitle, "--urgency", "normal", alertTitle, body}
	if err := l.run(ctx, l.binaryPath, args...); err != nil {
		return fmt.Errorf("showing notification: %w", err)
	}
	return nil
}
```

```go
// internal/notify/windows.go
package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const toastScript = `$ErrorActionPreference = 'Stop'
[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$text = $template.GetElementsByTagName('text')
[void]$text.Item(0).AppendChild($template.CreateTextNode($env:HUNTER_ALERT_TITLE))
[void]$text.Item(1).AppendChild($template.CreateTextNode($env:HUNTER_ALERT_BODY))
$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($env:HUNTER_ALERT_TITLE).Show($toast)`

type Windows struct {
	run func(ctx context.Context, name string, env []string, args ...string) error
}

func NewWindows() *Windows {
	return &Windows{
		run: func(ctx context.Context, name string, env []string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Env = env
			return cmd.Run()
		},
	}
}

func (w *Windows) Send(ctx context.Context, alert Alert) error {
	body := alert.Message
	if strings.HasPrefix(strings.ToLower(alert.URL), "https://") {
		body += "\n" + alert.URL
	}
	env := append(os.Environ(),
		"HUNTER_ALERT_TITLE="+alertTitle,
		"HUNTER_ALERT_BODY="+body,
	)
	args := []string{"-NoProfile", "-NonInteractive", "-Command", toastScript}
	if err := w.run(ctx, "powershell.exe", env, args...); err != nil {
		return fmt.Errorf("showing notification: %w", err)
	}
	return nil
}
```

```go
// acrescentar em internal/notify/notify.go
func New() Notifier { return newFor(runtime.GOOS) }

func newFor(goos string) Notifier {
	switch goos {
	case "darwin":
		return NewMacOS()
	case "windows":
		return NewWindows()
	default:
		return NewLinux()
	}
}
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `go test ./internal/notify/ -v`
Expected: PASS. Os testes de `MacOS` continuam passando sem alteração.

- [ ] **Step 5: Commit**

```bash
git add internal/notify
git commit -m "feat: alerts reach the desktop on linux and windows too"
```

---
### Task 10: O dashboard lê as fontes sob demanda

**Files:**
- Modify: `internal/web/server.go:49`, `internal/web/server.go:103-126`, `internal/web/server.go:360-361`, `internal/web/server.go:393-394`
- Modify: `internal/web/server_test.go`, `internal/web/lifecycle_test.go`, `internal/web/export_test.go`, `internal/web/fresh_test.go`
- Test: `internal/web/server_test.go`

**Interfaces:**
- Consumes: nada de tasks anteriores.
- Produces: `web.NewServer(s *store.Store, sources func() []string) http.Handler`.

**Contexto para quem implementa:** hoje `NewServer` recebe `[]string` uma vez e guarda no campo `sources` (`server.go:49`), lido no `/health` (`:360`) e nos indicadores de fonte (`:393`). Um processo que fica dias no ar mostraria para sempre a lista que existia no boot. Trocando por uma função, o dashboard passa a refletir o `config.Watcher` da Task 3 sem nenhum canal entre os dois.

São 80 chamadas de `NewServer` no pacote, quase todas em teste, no formato `NewServer(s, []string{"olx"})`. A troca é mecânica e tem comando pronto abaixo — confira o resultado com `go build ./...` antes de seguir.

- [ ] **Step 1: Escrever o teste que falha**

```go
func TestHealthFollowsTheEnabledSourcesWithoutARestart(t *testing.T) {
	s := openTemp(t)
	enabled := []string{"olx"}
	srv := NewServer(s, func() []string { return enabled })

	if _, body := get(t, srv, "/health"); strings.Contains(body, "webmotors") {
		t.Fatal("health listed a source that was not enabled")
	}

	enabled = []string{"olx", "webmotors"}
	_, body := get(t, srv, "/health")
	if !strings.Contains(body, "webmotors") {
		t.Errorf("health did not pick up the source enabled after boot:\n%s", body)
	}
}
```

O helper `openTemp` do pacote `web` está em `server_test.go`; se o nome divergir, use o que o arquivo já define para abrir um store temporário.

- [ ] **Step 2: Rodar o teste para ver falhar**

Run: `go test ./internal/web/ -run TestHealthFollows -v`
Expected: FAIL na compilação, porque `NewServer` ainda recebe `[]string`.

- [ ] **Step 3: Implementar o mínimo**

Em `internal/web/server.go`:

```go
type server struct {
	store   *store.Store
	sources func() []string
	// os demais campos ficam como estão
}

func NewServer(s *store.Store, sources func() []string) http.Handler {
	// o corpo não muda, exceto a atribuição do campo:
	srv := &server{
		store:      s,
		sources:    sources,
		listTmpl:   parse("list.html"),
		detailTmpl: parse("detail.html"),
		healthTmpl: parse("health.html"),
	}
	// ...
}
```

Nos dois pontos que iteram sobre as fontes (`:360` e `:393`), troque `s.sources` por uma variável local lida uma vez por requisição, para que a lista não mude no meio da renderização:

```go
	names := s.sources()
	items := make([]sourceHealth, 0, len(names))
	for _, name := range names {
```

- [ ] **Step 4: Converter os call sites de teste**

```bash
sed -i '' -E 's/NewServer\(([A-Za-z0-9_]+), \[\]string\{([^}]*)\}\)/NewServer(\1, fixed(\2))/g' internal/web/*_test.go
```

E acrescente o helper uma única vez, em `internal/web/server_test.go`:

```go
func fixed(names ...string) func() []string {
	return func() []string { return names }
}
```

Depois rode `grep -n 'NewServer(' internal/web/*_test.go | grep '\[\]string'` e converta à mão o que sobrar — chamadas com `nil` ou com uma variável no lugar do literal não casam com o padrão.

- [ ] **Step 5: Rodar os testes para ver passar**

Run: `go test ./internal/web/ -v`
Expected: PASS em todos. `cmd/hunter/main.go` ainda não compila; é a Task 11 que conserta o único call site de produção.

- [ ] **Step 6: Commit**

```bash
git add internal/web
git commit -m "feat: the dashboard shows the sources enabled right now"
```

---

### Task 11: O daemon

**Files:**
- Modify: `cmd/hunter/main.go`
- Create: `cmd/hunter/output.go`

**Interfaces:**
- Consumes: tudo das Tasks 1 a 10.
- Produces: os comandos `serve`, `crawl`, `repair-silenced` e `paths`.

**Contexto para quem implementa:** esta é a task de fiação, e é onde o macOS sai do `cmd/hunter`. O `-config` deixa de ter `config/config.yaml` como padrão e passa a resolver para o diretório do aplicativo; `serve` deixa de ser só o dashboard; `sendAlerts` troca `notify.NewMacOS()` por `notify.New()`; e o navegador passa a ser aberto e fechado a cada rodada, para não deixar um Chrome parado consumindo memória por doze horas — o perfil no disco é que guarda o login, não o processo.

O log em arquivo não exige reescrever as dezenas de `fmt.Printf` espalhadas: `fmt.Println` escreve na variável `os.Stdout` lida em tempo de execução, então trocar a variável por um `os.Pipe` e copiar para `io.MultiWriter(terminal, arquivo)` duplica tudo de uma vez.

- [ ] **Step 1: Escrever o duplicador de saída**

Em `cmd/hunter/output.go`:

```go
package main

import (
	"io"
	"os"

	"github.com/andreabreu76/harley-hunter/internal/logging"
)

const maxLogBytes = 5 << 20

func teeOutput(path string) (func(), error) {
	file, err := logging.Open(path, maxLogBytes)
	if err != nil {
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		file.Close()
		return nil, err
	}

	terminalOut, terminalErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = writer, writer

	copied := make(chan struct{})
	go func() {
		io.Copy(io.MultiWriter(terminalOut, file), reader)
		close(copied)
	}()

	return func() {
		os.Stdout, os.Stderr = terminalOut, terminalErr
		writer.Close()
		<-copied
		file.Close()
	}, nil
}
```

- [ ] **Step 2: Reescrever a entrada e os comandos**

```go
func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	dir, err := paths.AppDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path := *configPath
	if path == "" {
		path = paths.ConfigFile(dir)
	}
	if err := config.EnsureFile(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var runErr error
	switch flag.Arg(0) {
	case "crawl":
		runErr = runCrawl(path, dir)
	case "serve":
		runErr = runServe(path, dir)
	case "repair-silenced":
		runErr = runRepairSilenced(path)
	case "paths":
		runErr = runPaths(dir, path)
	default:
		fmt.Fprintln(os.Stderr, "usage: hunter [-config path] <serve|crawl|repair-silenced|paths>")
		os.Exit(2)
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
}

func runPaths(dir, configPath string) error {
	fmt.Printf("directory: %s\nconfig:    %s\ndatabase:  %s\nprofile:   %s\nlog:       %s\n",
		dir, configPath, paths.DatabaseFile(dir), paths.ChromeProfile(dir), paths.LogFile(dir))
	return nil
}
```

- [ ] **Step 3: Escrever o daemon e a rodada**

```go
const dashboardAddr = "127.0.0.1:8080"

func runServe(configPath, dir string) error {
	stopTee, err := teeOutput(paths.LogFile(dir))
	if err != nil {
		return err
	}
	defer stopTee()

	watcher, err := config.NewWatcher(configPath)
	if err != nil {
		return err
	}
	db, err := store.Open(watcher.Current().DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := &schedule.Runner{
		Config:  watcher,
		LastRun: db.LastRunStartedAt,
		Collect: func(cfg config.Config) error { return collect(ctx, cfg, db, dir) },
		Now:     time.Now,
		Warn:    os.Stderr,
	}
	go runner.Run(ctx)

	srv := &http.Server{
		Addr:    dashboardAddr,
		Handler: web.NewServer(db, func() []string { return watcher.Current().Sources }),
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	fmt.Printf("dashboard: http://%s\n", dashboardAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func collect(ctx context.Context, cfg config.Config, db *store.Store, dir string) error {
	handle, err := openBrowser(ctx, cfg, dir)
	if err != nil {
		return err
	}
	defer handle.Close()

	fetcher := source.NewBrowserFetcher(handle.DevtoolsURL())
	defer releaseTabs(fetcher)

	sources, err := buildSources(cfg, fetcher)
	if err != nil {
		return err
	}

	started := time.Now()
	report, err := crawl.Run(ctx, sources, db, cfg)
	if err != nil {
		return err
	}
	printReport(cfg, report, time.Since(started))
	refreshFipe(db)
	sendAlerts(cfg, db)
	return nil
}

func openBrowser(ctx context.Context, cfg config.Config, dir string) (*browser.Handle, error) {
	opts := browser.Options{ExistingURL: cfg.DevtoolsURL}
	if opts.ExistingURL == "" {
		executable, err := browser.Locate()
		if err != nil {
			return nil, err
		}
		opts.ExecutablePath = executable
		opts.ProfileDir = paths.ChromeProfile(dir)
		opts.Headless = browser.HeadlessNeeded(runtime.GOOS, os.Getenv)
	}
	return browser.Launch(ctx, opts)
}
```

`runCrawl` passa a ser a rodada única: carrega o config com `config.Load`, recusa com `config.Validate` e chama `collect`. `sendAlerts` troca `notify.NewMacOS()` por `notify.New()`. O `printReport` já menciona o Chrome em caso de falha compartilhada; troque a menção a `cfg.DevtoolsURL` pelo endereço em uso, que agora vem do handle.

- [ ] **Step 4: Compilar e rodar toda a bateria**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build limpo e todos os testes passando.

- [ ] **Step 5: Verificar o daemon à mão, com um diretório descartável**

```bash
export HARLEY_HUNTER_HOME=$(mktemp -d)
go run ./cmd/hunter paths
go run ./cmd/hunter serve
```

Expected: `paths` imprime os cinco caminhos dentro do diretório temporário. `serve` cria o `config.yaml` esqueleto, anuncia o dashboard, responde em `http://127.0.0.1:8080` e **não** coleta, porque o esqueleto não passa em `Validate`. `Ctrl-C` encerra sem deixar Chrome aberto — confira com `pgrep -fl "remote-debugging-port"`. Depois, `unset HARLEY_HUNTER_HOME`.

- [ ] **Step 6: Commit**

```bash
git add cmd/hunter
git commit -m "feat: serve keeps the hunt running and schedules its own rounds"
```

---

### Task 12: Fim do launchd como relógio, e o README

**Files:**
- Delete: `deploy/hunter-crawl.sh`
- Modify: `deploy/com.andreabreu.harleyhunter.plist`
- Modify: `Makefile`
- Modify: `README.md`

**Interfaces:**
- Consumes: os comandos da Task 11.
- Produces: `make cross`, e um agente de macOS que apenas mantém o daemon vivo.

**Contexto para quem implementa:** o `launchd` deixa de ser relógio e vira só autostart — sem `StartInterval`, com `KeepAlive`, apontando direto para o binário. O autostart de Linux e Windows é da fase 9 e não entra aqui. O `hunter-crawl.sh` inteiro foi absorvido pelo pacote `browser` e sai do repositório.

- [ ] **Step 1: Substituir o plist**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.andreabreu.harleyhunter</string>
    <key>ProgramArguments</key>
    <array>
        <string>/Users/andreabreu/bin/hunter</string>
        <string>serve</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    <string>/Users/andreabreu/Library/Application Support/harley-hunter/logs/launchd.error.log</string>
</dict>
</plist>
```

- [ ] **Step 2: Atualizar o Makefile**

Os alvos `crawl` e `logs` mudam de destino, `agent-status` deixa de perguntar por uma porta fixa, e entra o `cross`:

```make
BIN ?= $(HOME)/bin/hunter
AGENT := com.andreabreu.harleyhunter
PLIST := $(HOME)/Library/LaunchAgents/$(AGENT).plist
APPDIR := $(HOME)/Library/Application Support/harley-hunter

crawl: build ## roda uma coleta única
	$(BIN) crawl

serve: build ## sobe o daemon com o dashboard em http://127.0.0.1:8080
	$(BIN) serve

logs: ## acompanha o log do daemon
	tail -f "$(APPDIR)/logs/hunter.log"

cross: ## compila para os três sistemas em dist/
	@for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
		os=$${target%/*}; arch=$${target#*/}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		GOOS=$$os GOARCH=$$arch go build -o dist/hunter-$$os-$$arch$$ext ./cmd/hunter || exit 1; \
		echo "dist/hunter-$$os-$$arch$$ext"; \
	done

agent-status: ## mostra o estado do agente
	@launchctl list | grep $(AGENT) || echo "agente não carregado"
	@curl -sf -o /dev/null http://127.0.0.1:8080/health \
		&& echo "dashboard ok" || echo "dashboard fora do ar"
```

- [ ] **Step 3: Verificar o build cruzado**

Run: `make cross`
Expected: seis binários em `dist/`, sem erro. `dist/` já está no `.gitignore`.

- [ ] **Step 4: Reescrever as seções do README que mudaram de lugar**

O README precisa passar a dizer, em português: que `serve` é o daemon e agenda a própria coleta; onde ficam config, banco, perfil e log em cada sistema, com a menção ao `hunter paths`; que `interval_hours` no `crawl` do YAML substitui o `StartInterval` do plist; que o Chrome é encontrado sozinho e que `devtools_url` preenchido significa navegador externo; e que Instagram e Facebook Marketplace só devolvem resultado com sessão logada no perfil dedicado — nesta fase, o login é feito abrindo o Chrome com aquele perfil à mão, e a tela que faz isso por botão é da fase 8.

E a migração do banco existente, em passos:

```bash
launchctl bootout gui/$(id -u)/com.andreabreu.harleyhunter
mkdir -p "$HOME/Library/Application Support/harley-hunter"
cp hunter.db hunter.db-wal hunter.db-shm "$HOME/Library/Application Support/harley-hunter/"
cp config/config.yaml "$HOME/Library/Application Support/harley-hunter/"
make agent-install
```

Os três arquivos do banco, e não só o `.db`: o banco roda em WAL, e copiar apenas o primeiro descarta as transações ainda não integradas. Depois de copiar, apague a linha `database_path` do config novo, para que ele use o `hunter.db` ao lado — e confira com `hunter paths`.

- [ ] **Step 5: Remover o script e commitar**

```bash
git rm deploy/hunter-crawl.sh
git add Makefile README.md deploy/com.andreabreu.harleyhunter.plist
git commit -m "feat: launchd only keeps the daemon alive, and the shell script is gone"
```

---

## Fechamento da fase

- [ ] **Bateria completa**

Run: `go build ./... && go vet ./... && go test ./... && make cross`
Expected: tudo verde e os seis binários gerados.

- [ ] **Rodada real no macOS**

Com o config real, rodar `hunter crawl` uma vez e conferir no dashboard que a coleta gravou. É a prova de que o `browser` novo faz o que o shell script fazia.

- [ ] **Rodada real no Linux (opcional, na própria máquina)**

```bash
docker run --rm -it -v "$PWD":/src -w /src golang:1.26 bash -lc \
  'apt-get update -qq && apt-get install -y -qq chromium libnotify-bin >/dev/null && \
   HARLEY_HUNTER_HOME=/tmp/hunter go run ./cmd/hunter paths && \
   HARLEY_HUNTER_HOME=/tmp/hunter timeout 60 go run ./cmd/hunter serve'
```

Expected: `paths` responde com os caminhos do XDG, `serve` sobe o dashboard e o `locate` encontra o Chromium. Sem `DISPLAY`, o navegador vai para `--headless=new`.

- [ ] **Merge**

```bash
git checkout main
git merge --no-ff fase-6 -m "merge: fase 6 do harley-hunter"
```

- [ ] **Handoff**

Escrever `~/Documents/Claude/harley-hunter/2026-XX-XX-handoff-fase6.md` com o que ficou pendente de validação real (Windows, e Linux se o Docker não tiver sido usado), o que mudou de lugar na máquina do autor, e o estado em que a fase 7 encontra o projeto.
