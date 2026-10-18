# go-sdk is a multi-module workspace. Each target works on ONE module at a time.
#
#   make test MODULE=cqrs                 # unit tests of one module
#   make test MODULE=db TAGS=unit,database # with extra build tags (spins up containers)
#   make lint MODULE=cqrs
#   make tidy MODULE=cqrs
#   make modules                          # list module names
#
# Never run tests for every module at once on a laptop: db/outbox/s3 start
# testcontainers, and `-race` multiplies the load. `make test-all` is
# sequential and limited to GOMAXPROCS=2 and still takes a long time; prefer
# `make test MODULE=<name>` for the module you actually changed.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Module list comes from go.work, so it never drifts from the workspace.
MODULES := $(shell awk '/^\t\.\//{sub(/^\t\.\//,""); print}' go.work)

MODULE ?=
TAGS   ?= unit

# Same resolution as CI: no workspace, every module resolves through its own go.mod.
export GOWORK := off
export GOFLAGS := -count=1

.PHONY: help modules test test-all lint tidy

help:
	@sed -n '2,13p' $(MAKEFILE_LIST)

modules:
	@printf '%s\n' $(MODULES)

define require_module
	@if [ -z "$(MODULE)" ]; then echo "MODULE is required, e.g. make $@ MODULE=cqrs"; exit 2; fi
	@if [ ! -f "$(MODULE)/go.mod" ]; then echo "unknown module '$(MODULE)'; run 'make modules'"; exit 2; fi
endef

test:
	$(require_module)
	@if find "$(MODULE)" -name '*_test.go' -type f | grep -q .; then \
		cd "$(MODULE)" && go test -p 1 -tags="$(TAGS)" -shuffle=on ./... ; \
	else \
		echo "$(MODULE): no test files"; \
	fi

# Sequential, one module after another, CPU capped. Stops at the first failure.
test-all:
	@for m in $(MODULES); do \
		echo "==> $$m"; \
		GOMAXPROCS=2 $(MAKE) --no-print-directory test MODULE=$$m TAGS="$(TAGS)" || exit 1; \
	done

lint:
	$(require_module)
	@cd "$(MODULE)" && golangci-lint run --config=./.golangci.yml ./...

tidy:
	$(require_module)
	@cd "$(MODULE)" && go mod tidy
