SHELL := /bin/bash

ROOT_DIR := $(realpath $(dir $(lastword $(MAKEFILE_LIST))))
BIN_DIR := $(ROOT_DIR)/.bin
PROTO_DIR := $(ROOT_DIR)/testdata/proto
GEN_PB_DIR := $(ROOT_DIR)/testdata/gen/pb
PLUGINS_DIR := $(ROOT_DIR)/testdata/plugins
GEN_PLUGINS_DIR := $(ROOT_DIR)/testdata/gen/plugins

OS := $(shell ./tools/scripts/os.sh)
PROTOC_VERSION := 3.11.4
PROTOC_ZIP := protoc-$(PROTOC_VERSION)-$(OS)-x86_64.zip
PROTOC_OPTION := -I$(PROTO_DIR)
# PROTOC_GO_OPTION := $(PROTOC_OPTION) --plugin=${BIN_DIR}/protoc-gen-go --go_out=plugins=grpc:${ROOT_DIR}
PROTOC_GO_OPTION := $(PROTOC_OPTION) --plugin=${BIN_DIR}/protoc-gen-go --go_out=plugins=grpc,paths=source_relative:$(GEN_PB_DIR)

.PHONY: all
all: test

$(BIN_DIR)/protoc:
	@mkdir -p $(BIN_DIR)
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP)
	unzip -j -o $(PROTOC_ZIP) -d $(BIN_DIR) bin/protoc
	unzip -o $(PROTOC_ZIP) -d $(BIN_DIR) 'include/*'
	@rm -f $(PROTOC_ZIP)

$(BIN_DIR)/protoc-gen-go:
	@mkdir -p $(BIN_DIR)
	@go build -o ${BIN_DIR}/protoc-gen-go github.com/golang/protobuf/protoc-gen-go

$(BIN_DIR)/goprotoyamltag:
	@mkdir -p $(BIN_DIR)
	@go build -o ${BIN_DIR}/goprotoyamltag github.com/zoncoen/goprotoyamltag

$(BIN_DIR)/gotypenames:
	@mkdir -p $(BIN_DIR)
	@go build -o ${BIN_DIR}/gotypenames github.com/zoncoen/gotypenames

$(BIN_DIR)/mockgen:
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/mockgen github.com/golang/mock/mockgen

.PHONY: gen
gen: gen-proto gen-plugins

.PHONY: gen-proto
gen-proto: $(BIN_DIR)/protoc $(BIN_DIR)/protoc-gen-go
	@rm -rf $(GEN_PB_DIR)
	@mkdir -p $(GEN_PB_DIR)
	@find $(PROTO_DIR) -name '*.proto' | xargs -P8 $(BIN_DIR)/protoc $(PROTOC_GO_OPTION)
	@make add-yaml-tag
	@make gen-mock

.PHONY: add-yaml-tag
add-yaml-tag: $(BIN_DIR)/goprotoyamltag
	@for file in $$(find $(GEN_PB_DIR) -name '*.pb.go'); do \
		echo "add yaml tag $$file"; \
		${BIN_DIR}/goprotoyamltag --filename $$file -w; \
	done

.PHONY: gen-mock
gen-mock: $(BIN_DIR)/gotypenames $(BIN_DIR)/mockgen
	@for file in $$(find $(GEN_PB_DIR) -name '*.pb.go'); do \
		package=$$(basename $$(dirname $$file)); \
		echo "generate mock for $$file"; \
		${BIN_DIR}/gotypenames --filename $$file --only-exported --types interface | xargs -ISTRUCT -L1 -P8 ${BIN_DIR}/mockgen -source $$file -package $$package -self_package $(GEN_PB_DIR)/$$package -destination $$(dirname $$file)/$$(basename $${file%.pb.go})_mock.go; \
	done

.PHONY: gen-plugins
gen-plugins:
	@rm -rf $(GEN_PLUGINS_DIR)
	@mkdir -p $(GEN_PLUGINS_DIR)
	@for dir in $$(find $(PLUGINS_DIR) -name '*.go' | xargs -L1 -P8 dirname | sort | uniq); do \
		echo "build plugin $$(basename $$dir).so"; \
		go build -buildmode=plugin -o $(GEN_PLUGINS_DIR)/$$(basename $$dir).so $$dir; \
	done

.PHONY: test
test: $(ROOT_DIR)/testdata/gen
	@go test ./...
