# Semantic conventions of the go-sdk: validate the registry and generate the
# Go code the modules compile against.
#
# Weaver runs from its Docker image so that neither contributors nor CI need a
# Rust toolchain. The image is pinned by tag and digest; the custom manager in
# .github/renovate.json bumps both lines together, so keep them adjacent.
#
#   make check            validate the registry
#   make generate         regenerate the Go packages (commit the result)
#   make diff BASE=<ref>  compare the registry with the one at a git ref

WEAVER_VERSION ?= v0.28.0
WEAVER_IMAGE   ?= otel/weaver:$(WEAVER_VERSION)@sha256:5857bb81d8cc7ed608cdcae37c4b459eb020f21dd4acade35f7933cd6762fbcc

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/..)
BASE ?= origin/main

# Every target in the registry and where its generated code lives.
# <template target> -> <output directory relative to the repository root>
TARGETS := go=watermill/semconv

WEAVER = docker run --rm \
	-u "$(shell id -u):$(shell id -g)" \
	-e HOME=/tmp \
	-v "$(ROOT):/repo" \
	-w /repo \
	$(WEAVER_IMAGE)

.PHONY: check generate diff help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

check: ## Validate the registry
	$(WEAVER) registry check -r semconv/registry

generate: ## Regenerate the Go code from the registry
	@for t in $(TARGETS); do \
		target=$${t%%=*}; out=$${t#*=}; \
		echo "generate $$target -> $$out"; \
		$(WEAVER) registry generate --quiet -r semconv/registry -t semconv/templates $$target $$out || exit 1; \
	done
	@cd $(ROOT) && gofmt -l $(foreach t,$(TARGETS),$(word 2,$(subst =, ,$(t))))

diff: ## Show what changed in the registry since BASE (default origin/main)
	@set -eu; \
	cd $(ROOT); \
	if ! git rev-parse --verify --quiet "$(BASE)" >/dev/null; then \
		echo "unknown git ref $(BASE)"; exit 1; \
	fi; \
	if ! git cat-file -e "$(BASE):semconv/registry/manifest.yaml" 2>/dev/null; then \
		echo "no semconv/registry at $(BASE); nothing to compare"; exit 0; \
	fi; \
	rm -rf semconv/.baseline; mkdir -p semconv/.baseline; \
	trap 'rm -rf semconv/.baseline' EXIT; \
	git archive "$(BASE)" semconv/registry | tar -x -C semconv/.baseline --strip-components=1; \
	$(WEAVER) registry diff -r semconv/registry --baseline-registry semconv/.baseline/registry
