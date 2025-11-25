SELF_DIR := $(dir $(lastword $(MAKEFILE_LIST)))

# INCLUDE ==============================================================================================================
# Include Makefile
include ../../../ops/Makefile/common.mk

build: ## Build the cqrs-naming plugin
	@mkdir -p bin
	@GOWORK=off go build -buildmode=plugin -o bin/cqrs-naming.so main.go

