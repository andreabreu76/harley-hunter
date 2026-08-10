BIN ?= $(HOME)/bin/hunter
CONFIG ?= config/config.yaml
AGENT := com.andreabreu.harleyhunter
PLIST := $(HOME)/Library/LaunchAgents/$(AGENT).plist
LOGS := $(HOME)/Library/Logs

.DEFAULT_GOAL := help
.PHONY: help build crawl serve test fmt vet clean agent-install agent-uninstall agent-status logs

help: ## lista os targets
	@grep -hE '^[a-z][a-z-]*:.*##' $(MAKEFILE_LIST) | sort | awk -F':.*## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

build: ## compila o binário em ~/bin/hunter
	go build -o $(BIN) ./cmd/hunter

crawl: build ## roda uma coleta e dispara os alertas pendentes
	deploy/hunter-crawl.sh

serve: build ## sobe o dashboard em http://127.0.0.1:8080
	$(BIN) -config $(CONFIG) serve

test: ## roda os testes
	go test ./...

fmt: ## formata o código
	go fmt ./...

vet: ## roda o go vet
	go vet ./...

clean: ## remove o binário instalado
	rm -f $(BIN)

agent-install: build ## instala e carrega o agente launchd
	cp deploy/$(AGENT).plist $(PLIST)
	-launchctl bootout gui/$$(id -u)/$(AGENT) 2>/dev/null
	launchctl bootstrap gui/$$(id -u) $(PLIST)

agent-uninstall: ## descarrega o agente launchd
	launchctl bootout gui/$$(id -u)/$(AGENT)

agent-status: ## mostra o estado do agente e do Chrome de CDP
	@launchctl list | grep $(AGENT) || echo "agente não carregado"
	@curl -sf -o /dev/null http://127.0.0.1:9222/json/version \
		&& echo "chrome 9222 ok" || echo "chrome 9222 fora do ar"

logs: ## acompanha os logs da coleta
	tail -f $(LOGS)/harley-hunter.log $(LOGS)/harley-hunter.error.log
