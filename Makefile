BIN ?= $(HOME)/bin/hunter
AGENT := com.andreabreu.harleyhunter
PLIST := $(HOME)/Library/LaunchAgents/$(AGENT).plist
APPDIR := $(HOME)/Library/Application Support/harley-hunter

.DEFAULT_GOAL := help
.PHONY: help build crawl serve export test fmt vet clean cross agent-install agent-uninstall agent-status logs

help: ## lista os targets
	@grep -hE '^[a-z][a-z-]*:.*##' $(MAKEFILE_LIST) | sort | awk -F':.*## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

build: ## compila o binário em ~/bin/hunter
	go build -o $(BIN) ./cmd/hunter

crawl: build ## roda uma coleta única
	$(BIN) crawl

serve: build ## sobe o daemon com o dashboard em http://127.0.0.1:8080
	$(BIN) serve

export: ## baixa o json do dashboard em export.json (exige o serve rodando)
	curl -sf "http://127.0.0.1:8080/export.json$(if $(VERDICT),?verdict=$(VERDICT))" -o export.json \
		&& echo "export.json gravado" || echo "o dashboard não respondeu — rode make serve antes"

test: ## roda os testes
	go test ./...

fmt: ## formata o código
	go fmt ./...

vet: ## roda o go vet
	go vet ./...

clean: ## remove o binário instalado
	rm -f $(BIN)

cross: ## compila para os três sistemas em dist/
	@mkdir -p dist
	@for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
		os=$${target%/*}; arch=$${target#*/}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		GOOS=$$os GOARCH=$$arch go build -o dist/hunter-$$os-$$arch$$ext ./cmd/hunter || exit 1; \
		echo "dist/hunter-$$os-$$arch$$ext"; \
	done

agent-install: build ## instala e carrega o agente launchd
	@mkdir -p "$(APPDIR)/logs"
	cp deploy/$(AGENT).plist $(PLIST)
	-launchctl bootout gui/$$(id -u)/$(AGENT) 2>/dev/null
	launchctl bootstrap gui/$$(id -u) $(PLIST)

agent-uninstall: ## descarrega o agente launchd
	launchctl bootout gui/$$(id -u)/$(AGENT)

agent-status: ## mostra o estado do agente
	@launchctl list | grep $(AGENT) || echo "agente não carregado"
	@curl -sf -o /dev/null http://127.0.0.1:8080/health \
		&& echo "dashboard ok" || echo "dashboard fora do ar"

logs: ## acompanha o log do daemon
	tail -f "$(APPDIR)/logs/hunter.log"
