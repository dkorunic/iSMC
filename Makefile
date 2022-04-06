GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
WHITE  := $(shell tput -Txterm setaf 7)
CYAN   := $(shell tput -Txterm setaf 6)
RESET  := $(shell tput -Txterm sgr0)

GOTESTSUM_VERSION := v1.7.0
STATICCHECK_VERSION := 2022.1

# Suppress CGO compiler warnings
export CGO_CFLAGS=-w

.PHONY: all build test

all: help

## Targets:

build: ## Build your project and put the output binary in out/bin/
	@mkdir -p out/bin
	@go build -o out/bin/ .

clean: ## Clean up task related files
	@rm -fr ./out
	@rm -f ./report.xml ./profile.cov

verify: ## Verify dependencies
	@go mod verify
	@go mod download

lint: ## Run linters
	@go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}

	@go mod tidy
	@git diff --exit-code go.mod

	@staticcheck ./...

test: ## Run tests
	@go install gotest.tools/gotestsum@${GOTESTSUM_VERSION}
	@gotestsum -- -coverprofile=profile.cov ./...

coverage: test ## Generate coverage report
	@go tool cover -html=profile.cov

help: ## Show this help
	@echo ''
	@echo 'Usage:'
	@echo '  ${YELLOW}make${RESET} ${GREEN}<target>${RESET}'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} { \
		if (/^[a-zA-Z_-]+:.*?##.*$$/) {printf "    ${YELLOW}%-20s${GREEN}%s${RESET}\n", $$1, $$2} \
		else if (/^## .*$$/) {printf "  ${CYAN}%s${RESET}\n", substr($$1,4)} \
		}' $(MAKEFILE_LIST)
