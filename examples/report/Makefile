SHELL := /bin/bash

# Add scenarigo binary to PATH
BIN_DIR := $(CURDIR)/../../.bin
PATH := $(abspath $(BIN_DIR)):$(PATH)

.PHONY: run-with-fail
run-with-fail: ## Run all scenarios including the intentionally failing one (for demonstration)
	@scenarigo plugin build && scenarigo run scenarios/
