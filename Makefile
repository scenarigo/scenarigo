SHELL := /bin/bash
.DEFAULT_GOAL := test

GO ?= go
export GOTOOLCHAIN := local

BIN_DIR := $(CURDIR)/.bin
export GOBIN := $(BIN_DIR)
PATH := $(abspath $(BIN_DIR)):$(PATH)
TOOLS_DIR := $(CURDIR)/tools

UNAME_OS := $(shell uname -s)
UNAME_ARCH := $(shell uname -m)
GOOS := $(shell $(GO) env GOOS)
GOARCH := $(shell $(GO) env GOARCH)

PROTO_DIR := $(CURDIR)/testdata/proto
GEN_PB_DIR := $(CURDIR)/testdata/gen/pb
PLUGINS_DIR := $(CURDIR)/test/e2e/testdata/plugins
GEN_PLUGINS_DIR := $(CURDIR)/test/e2e/testdata/gen/plugins

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

PROTOC := $(BIN_DIR)/protoc
PROTOC_VERSION := 24.4
PROTOC_OS := $(UNAME_OS)
ifeq "$(UNAME_OS)" "Darwin"
	PROTOC_OS = osx
endif
PROTOC_ARCH := $(UNAME_ARCH)
ifeq "$(UNAME_ARCH)" "arm64"
	PROTOC_ARCH = aarch_64
endif
PROTOC_ZIP := protoc-$(PROTOC_VERSION)-$(PROTOC_OS)-$(PROTOC_ARCH).zip
$(PROTOC): | $(BIN_DIR)
	@curl -sSOL \
		"https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP)"
	@unzip -j -o $(PROTOC_ZIP) -d $(BIN_DIR) bin/protoc
	@unzip -o $(PROTOC_ZIP) -d $(BIN_DIR) "include/*"
	@rm -f $(PROTOC_ZIP)

PROTOC_GEN_GO := $(BIN_DIR)/protoc-gen-go
$(PROTOC_GEN_GO): | $(BIN_DIR)
	@$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.31.0

PROTOC_GEN_GO_GRPC := $(BIN_DIR)/protoc-gen-go-grpc
$(PROTOC_GEN_GO_GRPC): | $(BIN_DIR)
	@$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0

GOPROTOYAMLTAG := $(BIN_DIR)/goprotoyamltag
$(GOPROTOYAMLTAG): | $(BIN_DIR)
	@$(GO) install github.com/zoncoen/goprotoyamltag@v1.0.0

GOTYPENAMES := $(BIN_DIR)/gotypenames
$(GOTYPENAMES): | $(BIN_DIR)
	@$(GO) install github.com/zoncoen/gotypenames@v1.0.0

MOCKGEN := $(BIN_DIR)/mockgen
$(MOCKGEN): | $(BIN_DIR)
	@$(GO) install github.com/golang/mock/mockgen@v1.6.0

GO_LICENSES := $(BIN_DIR)/go-licenses
$(GO_LICENSES): | $(BIN_DIR)
	@$(GO) install github.com/google/go-licenses@v1.6.0

GOCREDITS := $(BIN_DIR)/gocredits
$(GOCREDITS): | $(BIN_DIR)
	@$(GO) install github.com/Songmu/gocredits/cmd/gocredits@v0.3.0

GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
GOLANGCI_LINT_VERSION := 2.2.1
GOLANGCI_LINT_OS_ARCH := $(shell echo golangci-lint-$(GOLANGCI_LINT_VERSION)-$(GOOS)-$(GOARCH))
GOLANGCI_LINT_GZIP := $(GOLANGCI_LINT_OS_ARCH).tar.gz
$(GOLANGCI_LINT): | $(BIN_DIR)
	@curl -sSOL \
		"https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCI_LINT_VERSION)/$(GOLANGCI_LINT_GZIP)"
	@tar -C $(BIN_DIR) --strip=1 -xf $(GOLANGCI_LINT_GZIP) $(GOLANGCI_LINT_OS_ARCH)/golangci-lint
	@rm $(GOLANGCI_LINT_GZIP)

.PHONY: build
build: | $(BIN_DIR)
	@$(GO) install ./cmd/scenarigo

.PHONY: test
CMD_DIR := cmd
TEST_TARGETS_MODULE := $(shell $(GO) list ./... | grep -v $(CMD_DIR) | grep -v test/e2e)
test: test/norace test/race

.PHONY: test/norace
test/norace:
	@$(GO) test ./...

.PHONY: test/race
test/race:
	@$(GO) test -race ./...

.PHONY: test/ci
test/ci: coverage test/race test/e2e

.PHONY: coverage
coverage: coverage/cmd coverage/module ## measure test coverage

.PHONY: coverage/cmd
coverage/cmd:
	@$(GO) test ./$(CMD_DIR)/... -coverprofile=coverage-cmd.out -covermode=atomic

.PHONY: coverage/module
coverage/module:
	@$(GO) test $(TEST_TARGETS_MODULE) -coverprofile=coverage-module.out -covermode=atomic

.PHONY: test/e2e
test/e2e:
	@$(GO) test ./test/e2e/...

.PHONY: test/examples
test/examples:
	@failure=false; \
	for dir in $(sort $(wildcard examples/*)); do \
		echo "=== $$dir ==="; \
		make test/$$dir; \
		if [ $$? -ne 0 ]; then \
			failure=true; \
		fi; \
		echo ""; \
	done; \
	if "$${failure}"; then \
		exit 1; \
	fi

test/examples/%:
	@dir=examples/$*; \
	if [ -d $(CURDIR)/$${dir}/plugin/src ]; then \
		cd $(CURDIR)/$${dir}/plugin/src; \
		rm -f go.work go.work.sum; \
		$(GO) get github.com/scenarigo/scenarigo@latest ; \
		$(GO) work init . ../../../..; \
	fi; \
	cd $(CURDIR)/$$dir; \
	scenarigo plugin build && scenarigo run

.PHONY: lint
lint: $(GOLANGCI_LINT) ## run lint
	@$(GOLANGCI_LINT) run -c ./.golangci.yml

.PHONY: lint/fix
lint/fix: $(GOLANGCI_LINT) ## fix lint errors
	@$(GOLANGCI_LINT) run -c ./.golangci.yml --fix

.PHONY: lint/ci
lint/ci:
	@$(GO) version
	@make credits
	@git add --all
	@git diff --cached --exit-code || (echo '"make credits" required'; exit 1)

.PHONY: clean
clean: ## remove generated files
	@rm -rf $(BIN_DIR) $(GEN_PB_DIR) $(GEN_PLUGINS_DIR)

.PHONY: gen
gen: gen/proto gen/plugins ## generate necessary files for testing

.PHONY: gen/proto
PROTOC_OPTION := -I$(PROTO_DIR)
PROTOC_GO_OPTION := --plugin=${PROTOC_GEN_GO} --go_out=$(GEN_PB_DIR) --go_opt=paths=source_relative
PROTOC_GO_GRPC_OPTION := --go-grpc_out=require_unimplemented_servers=false:$(GEN_PB_DIR) --go-grpc_opt=paths=source_relative
gen/proto: $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC)
	@rm -rf $(GEN_PB_DIR)
	@mkdir -p $(GEN_PB_DIR)
	@find $(PROTO_DIR) -name '*.proto' | xargs -P8 protoc $(PROTOC_OPTION) $(PROTOC_GO_OPTION) $(PROTOC_GO_GRPC_OPTION)
	@make gen/mock

.PHONY: add-yaml-tag
add-yaml-tag: $(GOPROTOYAMLTAG)
	@for file in $$(find $(GEN_PB_DIR) -name '*.pb.go'); do \
		echo "add yaml tag $$file"; \
		$(GOPROTOYAMLTAG) --filename $$file -w; \
	done

.PHONY: gen/mock
gen/mock: $(GOTYPENAMES) $(MOCKGEN)
	@for file in $$(find $(GEN_PB_DIR) -name '*_grpc.pb.go'); do \
		package=$$(basename $$(dirname $$file)); \
		echo "generate mock for $$file"; \
		dstfile=$$(dirname $$file)/$$(basename $${file%.pb.go})_mock.go; \
		self=github.com/scenarigo/scenarigo`echo $(GEN_PB_DIR)/$$package | perl -pe 's!^$(CURDIR)!!g'`; \
		$(GOTYPENAMES) --filename $$file --only-exported --types interface | xargs -ISTRUCT -L1 -P8 $(MOCKGEN) -source $$file -package $$package -self_package $$self -destination $$dstfile; \
		perl -pi -e 's!^// Source: .*\n!!g' $$dstfile ||  (echo "failed to delete generated marker about source path ( Source: /path/to/name.pb.go )"); \
	done

.PHONY: gen/plugins
gen/plugins:
	@rm -rf $(GEN_PLUGINS_DIR)
	@mkdir -p $(GEN_PLUGINS_DIR)
	@for dir in $$(find $(PLUGINS_DIR) -name '*.go' | xargs -L1 -P8 dirname | sort | uniq); do \
		echo "build plugin $$(basename $$dir).so"; \
		$(GO) build -buildmode=plugin -o $(GEN_PLUGINS_DIR)/$$(basename $$dir).so $$dir; \
	done
	@echo "building WASM plugins..."
	@$(GO) run ./scripts/build-wasm-plugin/main.go $(PLUGINS_DIR)

.PHONY: credits
credits: $(GO_LICENSES) $(GOCREDITS) ## generate CREDITS
	@$(GO) mod download
	@$(GO_LICENSES) check ./...
	@$(GO) mod why github.com/davecgh/go-spew # HACK: download explicitly for credits
	@$(GOCREDITS) . > CREDITS
	@$(GO) mod tidy

.PHONY: matrix
matrix:
	@cd scripts/cross-build && $(GO) run ./build-matrix/main.go

.PHONY: crossbuild
crossbuild:
	@rm -rf ./tmp
	@mkdir  ./tmp
	@for ver in $$(cd scripts/cross-build && $(GO) run ./build-matrix/main.go | jq -r '.[]'); do \
	  GO_VERSION=$${ver} make build/ci; \
	  cat ./dist/*_checksums.txt > ./tmp/checksums.txt; \
	  mv ./dist/*.tar.gz ./tmp/; \
	done
	@rm -rf ./dist ./assets
	@ls ./tmp/

.PHONY: crossbuild/test
crossbuild/test:
	@rm -rf ./dist
	@SNAPSHOT=true GO_VERSION=$$(go version | perl -ape '$$_ = $$F[2]; s/go(.+)/$$1/') make build/ci
	@ls ./dist

.PHONY: build/ci
build/ci:
	@rm -rf assets
	@cd scripts/cross-build && PJ_ROOT=$(CURDIR) $(GO) run ./cross-build/main.go && cd -

.PHONY: help
help: ## print help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
