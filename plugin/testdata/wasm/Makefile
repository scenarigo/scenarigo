SHELL := /bin/bash
.DEFAULT_GOAL := gen

GO ?= go
export GOTOOLCHAIN := local

BIN_DIR := $(CURDIR)/../../../.bin
PATH := $(abspath $(BIN_DIR)):$(PATH)
PROTOC := $(BIN_DIR)/protoc
PROTOC_GEN_GO := $(BIN_DIR)/protoc-gen-go
PROTOC_GEN_GO_GRPC := $(BIN_DIR)/protoc-gen-go-grpc

PROTO_DIR := $(CURDIR)/proto
GEN_PB_DIR := $(CURDIR)/src/pb

.PHONY: gen
gen: gen/proto

.PHONY: gen/proto
PROTOC_OPTION := -I$(PROTO_DIR)
PROTOC_GO_OPTION := --plugin=${PROTOC_GEN_GO} --go_out=$(GEN_PB_DIR) --go_opt=paths=source_relative
PROTOC_GO_GRPC_OPTION := --go-grpc_out=require_unimplemented_servers=false:$(GEN_PB_DIR) --go-grpc_opt=paths=source_relative
gen/proto: $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC)
	@rm -rf $(GEN_PB_DIR)
	@mkdir -p $(GEN_PB_DIR)
	@find $(PROTO_DIR) -name '*.proto' | xargs -P8 $(PROTOC) $(PROTOC_OPTION) $(PROTOC_GO_OPTION) $(PROTOC_GO_GRPC_OPTION)
