.DEFAULT_GOAL := help

OUT ?= runs
CORPUS ?= corpus
REPEAT ?= 1
LIMIT ?= 20
VERIFY_LIMIT ?= 0
CASE ?= orphan-process
CASES ?=
ENTRY_ID ?=
DELAY ?= 1500ms
MOCK_URL ?=
ENV ?= local
CONTAINER_IMAGE ?= ubuntu:latest
GO_CACHE ?= /tmp/syncfuzz-go-cache

CASE_ARGS := $(if $(CASES),--cases $(CASES),)
MOCK_ARGS := $(if $(MOCK_URL),--mock-url $(MOCK_URL),)
ENV_ARGS := --env $(ENV)
CONTAINER_ARGS := --container-image $(CONTAINER_IMAGE)

.PHONY: help list fault-plans run-case run-mvp run-action run-authority run-shell run-fs run-branch run-suite corpus-list corpus-show corpus-verify replay test-go fmt-go mock-build mock-start

help:
	@echo "SyncFuzz targets:"
	@echo "  make list"
	@echo "  make fault-plans"
	@echo "  make run-case CASE=orphan-process"
	@echo "  make run-suite REPEAT=1 CASES=action-replay,branch-leakage"
	@echo "  make corpus-list"
	@echo "  make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>"
	@echo "  make corpus-verify"
	@echo "  make replay ENTRY_ID=<entry_id_or_unique_prefix>"
	@echo "  make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest"
	@echo "Variables: OUT=$(OUT), CORPUS=$(CORPUS), DELAY=$(DELAY), ENV=$(ENV), CONTAINER_IMAGE=$(CONTAINER_IMAGE), LIMIT=$(LIMIT), VERIFY_LIMIT=$(VERIFY_LIMIT), MOCK_URL=$(MOCK_URL)"

list:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz list

fault-plans:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz fault-plans

run-case:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case $(CASE) --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

run-mvp:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case orphan-process --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

run-action:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case action-replay --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

run-authority:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case authority-resurrection --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

run-shell:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case persistent-shell-poisoning --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

run-fs:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case partial-filesystem-rollback --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

run-branch:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case branch-leakage --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

run-suite:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz suite --out $(OUT) --corpus $(CORPUS) --repeat $(REPEAT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(CASE_ARGS) $(MOCK_ARGS)

corpus-list:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz corpus list --corpus $(CORPUS) --limit $(LIMIT)

corpus-show:
	@test -n "$(ENTRY_ID)" || (echo "usage: make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>"; exit 2)
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz corpus show --corpus $(CORPUS) --id $(ENTRY_ID)

corpus-verify:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz corpus verify --corpus $(CORPUS) --out $(OUT) --limit $(VERIFY_LIMIT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

replay:
	@test -n "$(ENTRY_ID)" || (echo "usage: make replay ENTRY_ID=<entry_id_or_unique_prefix>"; exit 2)
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz replay --corpus $(CORPUS) --id $(ENTRY_ID) --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS)

test-go:
	GOCACHE=$(GO_CACHE) go test ./...

fmt-go:
	gofmt -w cmd internal

mock-build:
	cd services/mock-servers && npm run build

mock-start:
	cd services/mock-servers && npm run start
