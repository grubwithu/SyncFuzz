.DEFAULT_GOAL := help

OUT ?= runs
CORPUS ?= corpus
REPEAT ?= 1
ROUNDS ?= 2
LIMIT ?= 20
VERIFY_LIMIT ?= 0
CASE ?= orphan-process
CASES ?=
ENTRY_ID ?=
DELAY ?= 1500ms
MOCK_URL ?=
ENV ?= local
CONTAINER_IMAGE ?= ubuntu:latest
DIFFERENTIAL ?= false
TIMING ?=
INCLUDE_PLANNED ?= false
FEEDBACK_FROM ?=
CANDIDATE_LIMIT ?= 0
TARGET_ADAPTER ?= command
TARGET_ID ?= command
TARGET_TASK ?= orphan-process
TARGET_COMMAND ?=
TARGET_COMMAND_FILE ?=
TARGET_PROMPT ?=
TARGET_PROMPT_FILE ?=
TARGET_TIMEOUT ?= 2m
TARGET_OBSERVE_DELAY ?= 500ms
TARGET_LATE_OBSERVE_DELAY ?= $(if $(filter orphan-process-long-delay,$(TARGET_TASK)),7s,)
EXPECT_FILES ?=
LANGCHAIN_MODEL ?=
OPENAI_BASE_URL ?=
LANGGRAPH_POLICY ?= host
LANGGRAPH_DOCKER_IMAGE ?=
LANGGRAPH_REPLAY ?= false
LANGGRAPH_CHECKPOINT_INDEX ?= -1
LANGGRAPH_FORK_USER_MESSAGE ?=
DOTENV_FILE ?= ./.env
GO_CACHE ?= /tmp/syncfuzz-go-cache

CASE_ARGS := $(if $(CASES),--cases $(CASES),)
MOCK_ARGS := $(if $(MOCK_URL),--mock-url $(MOCK_URL),)
ENV_ARGS := --env $(ENV)
CONTAINER_ARGS := --container-image $(CONTAINER_IMAGE)
DIFFERENTIAL_ARGS := $(if $(filter true,$(DIFFERENTIAL)),--differential,)
TIMING_ARGS := $(if $(TIMING),--timing $(TIMING),)
PLANNED_ARGS := $(if $(filter true,$(INCLUDE_PLANNED)),--include-planned,)
FEEDBACK_ARGS := $(if $(FEEDBACK_FROM),--feedback-from $(FEEDBACK_FROM),)
CANDIDATE_LIMIT_ARGS := $(if $(filter-out 0,$(CANDIDATE_LIMIT)),--candidate-limit $(CANDIDATE_LIMIT),)
TARGET_COMMAND_ARGS := $(if $(TARGET_COMMAND),--command '$(subst ','"'"',$(TARGET_COMMAND))',)
TARGET_COMMAND_FILE_ARGS := $(if $(TARGET_COMMAND_FILE),--command-file $(TARGET_COMMAND_FILE),)
TARGET_PROMPT_ARGS := $(if $(TARGET_PROMPT),--prompt "$(TARGET_PROMPT)",)
TARGET_PROMPT_FILE_ARGS := $(if $(TARGET_PROMPT_FILE),--prompt-file $(TARGET_PROMPT_FILE),)
TARGET_EXPECT_ARGS := $(if $(EXPECT_FILES),--expect-files $(EXPECT_FILES),)
TARGET_LATE_OBSERVE_ARGS := $(if $(TARGET_LATE_OBSERVE_DELAY),--late-observe-delay $(TARGET_LATE_OBSERVE_DELAY),)
LANGCHAIN_MODEL_ENV := $(if $(LANGCHAIN_MODEL),LANGCHAIN_MODEL='$(subst ','"'"',$(LANGCHAIN_MODEL))',)
OPENAI_BASE_URL_ENV := $(if $(OPENAI_BASE_URL),OPENAI_BASE_URL='$(subst ','"'"',$(OPENAI_BASE_URL))',)
LOAD_DOTENV = set -a; test ! -f "$(DOTENV_FILE)" || . "$(DOTENV_FILE)"; set +a

.PHONY: help list fault-plans timing-profiles primitives matrix run-case run-pair run-mvp run-action run-authority run-shell run-fs run-branch run-suite run-diff-suite run-matrix-suite run-campaign target-list target-run target-langgraph-shell-react target-langgraph-shell-react-check corpus-list corpus-show corpus-verify replay test-go fmt-go mock-build mock-start

help:
	@echo "SyncFuzz targets:"
	@echo "  make list"
	@echo "  make fault-plans"
	@echo "  make timing-profiles"
	@echo "  make primitives"
	@echo "  make matrix CASES=orphan-process TIMING=baseline,tight"
	@echo "  make run-case CASE=orphan-process"
	@echo "  make run-pair CASE=orphan-process"
	@echo "  make run-suite REPEAT=1 CASES=action-replay,branch-leakage"
	@echo "  make run-diff-suite REPEAT=1 CASES=action-replay,branch-leakage"
	@echo "  make run-matrix-suite CASES=orphan-process TIMING=baseline,tight"
	@echo "  make run-matrix-suite FEEDBACK_FROM=<matrix-result.json> CANDIDATE_LIMIT=3"
	@echo "  make run-campaign ROUNDS=2 CANDIDATE_LIMIT=3 CASES=action-replay"
	@echo "  make target-list"
	@echo "  make target-run TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh"
	@echo "  make target-langgraph-shell-react-check LANGCHAIN_MODEL=openai:gpt-4.1-mini"
	@echo "  make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini"
	@echo "  make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini OPENAI_BASE_URL=https://api.example.com/v1"
	@echo "  make target-langgraph-shell-react TARGET_TASK=orphan-process-long-delay"
	@echo "  make corpus-list"
	@echo "  make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>"
	@echo "  make corpus-verify"
	@echo "  make replay ENTRY_ID=<entry_id_or_unique_prefix>"
	@echo "  make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest"
	@echo "Variables: OUT=$(OUT), CORPUS=$(CORPUS), DELAY=$(DELAY), ENV=$(ENV), CONTAINER_IMAGE=$(CONTAINER_IMAGE), LIMIT=$(LIMIT), VERIFY_LIMIT=$(VERIFY_LIMIT), ROUNDS=$(ROUNDS), DIFFERENTIAL=$(DIFFERENTIAL), TIMING=$(TIMING), INCLUDE_PLANNED=$(INCLUDE_PLANNED), FEEDBACK_FROM=$(FEEDBACK_FROM), CANDIDATE_LIMIT=$(CANDIDATE_LIMIT), TARGET_ADAPTER=$(TARGET_ADAPTER), TARGET_ID=$(TARGET_ID), TARGET_TASK=$(TARGET_TASK), TARGET_TIMEOUT=$(TARGET_TIMEOUT), TARGET_OBSERVE_DELAY=$(TARGET_OBSERVE_DELAY), TARGET_LATE_OBSERVE_DELAY=$(TARGET_LATE_OBSERVE_DELAY), TARGET_COMMAND_FILE=$(TARGET_COMMAND_FILE), EXPECT_FILES=$(EXPECT_FILES), LANGCHAIN_MODEL=$(LANGCHAIN_MODEL), OPENAI_BASE_URL=$(OPENAI_BASE_URL), LANGGRAPH_POLICY=$(LANGGRAPH_POLICY), LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE), LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY), LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX), LANGGRAPH_FORK_USER_MESSAGE=$(LANGGRAPH_FORK_USER_MESSAGE), DOTENV_FILE=$(DOTENV_FILE), MOCK_URL=$(MOCK_URL)"

list:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz list

fault-plans:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz fault-plans

timing-profiles:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz timing-profiles

primitives:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz primitives $(PLANNED_ARGS)

matrix:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz matrix $(CASE_ARGS) $(TIMING_ARGS) $(PLANNED_ARGS)

run-case:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case $(CASE) --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

run-pair:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz pair --case $(CASE) --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

run-mvp:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case orphan-process --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

run-action:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case action-replay --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

run-authority:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case authority-resurrection --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

run-shell:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case persistent-shell-poisoning --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

run-fs:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case partial-filesystem-rollback --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

run-branch:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz run --case branch-leakage --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

run-suite:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz suite --out $(OUT) --corpus $(CORPUS) --repeat $(REPEAT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(CASE_ARGS) $(MOCK_ARGS) $(DIFFERENTIAL_ARGS) $(TIMING_ARGS)

run-diff-suite:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz suite --out $(OUT) --corpus $(CORPUS) --repeat $(REPEAT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(CASE_ARGS) $(MOCK_ARGS) --differential $(TIMING_ARGS)

run-matrix-suite:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz suite --matrix --out $(OUT) --corpus $(CORPUS) --repeat $(REPEAT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(CASE_ARGS) $(MOCK_ARGS) $(DIFFERENTIAL_ARGS) $(TIMING_ARGS) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS)

run-campaign:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz campaign --out $(OUT) --corpus $(CORPUS) --rounds $(ROUNDS) --repeat $(REPEAT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(CASE_ARGS) $(MOCK_ARGS) $(DIFFERENTIAL_ARGS) $(TIMING_ARGS) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS)

target-list:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz target list

target-run:
	$(LOAD_DOTENV); GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz target run --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) --out $(OUT) --timeout $(TARGET_TIMEOUT) --observe-delay $(TARGET_OBSERVE_DELAY) $(TARGET_LATE_OBSERVE_ARGS) $(ENV_ARGS) $(CONTAINER_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS) $(TARGET_PROMPT_ARGS) $(TARGET_PROMPT_FILE_ARGS) $(TARGET_EXPECT_ARGS)

target-langgraph-shell-react:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz target run --target langgraph-shell-react --task $(TARGET_TASK) --out $(OUT) --timeout $(TARGET_TIMEOUT) --observe-delay $(TARGET_OBSERVE_DELAY) $(TARGET_LATE_OBSERVE_ARGS) $(ENV_ARGS) $(CONTAINER_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh $(TARGET_PROMPT_ARGS) $(TARGET_PROMPT_FILE_ARGS) $(TARGET_EXPECT_ARGS)

target-langgraph-shell-react-check:
	@$(LOAD_DOTENV); test -x targets/langgraph_shell_react/venv/bin/python || (echo "missing targets/langgraph_shell_react/venv/bin/python"; exit 2)
	@$(LOAD_DOTENV); targets/langgraph_shell_react/venv/bin/python -c "from langchain.agents import create_agent; from langgraph.checkpoint.memory import InMemorySaver; from langchain.agents.middleware import ShellToolMiddleware; print('langgraph shell target imports ok')"
	@$(LOAD_DOTENV); \
	model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; \
	test -n "$$model" || (echo "usage: make target-langgraph-shell-react-check LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2); \
	base_url="$(OPENAI_BASE_URL)"; test -n "$$base_url" || base_url="$$OPENAI_BASE_URL"; \
	if printf '%s' "$$model" | grep -q '^openai:'; then test -n "$$OPENAI_API_KEY$$OPENAI_ADMIN_KEY" || (echo "OPENAI_API_KEY is not set in this shell or $(DOTENV_FILE)"; exit 2); fi; \
	if printf '%s' "$$model" | grep -q '^openai:'; then if test -n "$$base_url"; then echo "OPENAI_BASE_URL configured for compatible endpoint"; else echo "OPENAI_BASE_URL not set; using provider default"; fi; fi; \
	if printf '%s' "$$model" | grep -q '^anthropic:'; then test -n "$$ANTHROPIC_API_KEY" || (echo "ANTHROPIC_API_KEY is not set in this shell or $(DOTENV_FILE)"; exit 2); fi
	@echo "langgraph shell target environment looks ready"

corpus-list:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz corpus list --corpus $(CORPUS) --limit $(LIMIT)

corpus-show:
	@test -n "$(ENTRY_ID)" || (echo "usage: make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>"; exit 2)
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz corpus show --corpus $(CORPUS) --id $(ENTRY_ID)

corpus-verify:
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz corpus verify --corpus $(CORPUS) --out $(OUT) --limit $(VERIFY_LIMIT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

replay:
	@test -n "$(ENTRY_ID)" || (echo "usage: make replay ENTRY_ID=<entry_id_or_unique_prefix>"; exit 2)
	GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz replay --corpus $(CORPUS) --id $(ENTRY_ID) --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)

test-go:
	GOCACHE=$(GO_CACHE) go test ./...

fmt-go:
	gofmt -w cmd internal

mock-build:
	cd services/mock-servers && npm run build

mock-start:
	cd services/mock-servers && npm run start
