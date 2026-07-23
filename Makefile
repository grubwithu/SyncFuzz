.DEFAULT_GOAL := help

# Output / corpus
OUT ?= runs
CORPUS ?= corpus

# Execution budgets
REPEAT ?= 1
ROUNDS ?= 2
LIMIT ?= 20
VERIFY_LIMIT ?= 0

# Core local/container execution
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
MINIMIZE_FROM ?=
MINIMIZE_EXECUTE ?= false
MINIMIZE_CANDIDATE_LIMIT ?= 1
MINIMIZE_MAX_TRIALS ?= 32

# Generic target runner
TARGET_ADAPTER ?= command
TARGET_ID ?= command
TARGET_TASK ?= orphan-process
TARGET_TASKS ?=
TARGET_SEED ?=
TARGET_SEEDS ?=
TARGET_GROUP ?=
TARGET_GROUPS ?=
TARGET_PROMPT_PROFILE ?=
TARGET_PROMPT_PROFILES ?=
TARGET_COMMAND ?=
TARGET_COMMAND_FILE ?=
TARGET_PROMPT ?=
TARGET_PROMPT_FILE ?=
TARGET_TIMEOUT ?= 2m
TARGET_OBSERVE_DELAY ?= 500ms
TARGET_LATE_OBSERVE_DELAY ?= $(if $(filter orphan-process-long-delay,$(TARGET_TASK)),7s,)
EXPECT_FILES ?=

# LangGraph target
LANGCHAIN_MODEL ?=
OPENAI_API_KEY ?=
OPENAI_BASE_URL ?=
LANGGRAPH_POLICY ?= host
LANGGRAPH_DOCKER_IMAGE ?=
LANGGRAPH_CHECKPOINT_BACKEND ?= memory
LANGGRAPH_CHECKPOINT_DIR ?=
LANGGRAPH_PROCESS_MODE ?= single
LANGGRAPH_REPLAY ?= false
LANGGRAPH_CHECKPOINT_INDEX ?= -1
LANGGRAPH_CHECKPOINT_SELECTOR ?=
LANGGRAPH_FORK_USER_MESSAGE ?=
LANGGRAPH_PROFILE_IMAGE ?= syncfuzz-langgraph:dev
LANGGRAPH_SYNTHESIS_OBJECTIVE ?=
LANGGRAPH_SYNTHESIS_CANDIDATE ?=
LANGGRAPH_SYNTHESIS_ROOT ?=
LANGGRAPH_SYNTHESIS_FRONTIER ?=
LANGGRAPH_SYNTHESIS_MANIFEST ?=
LANGGRAPH_SYNTHESIS_BINDING ?=
LANGGRAPH_SYNTHESIS_BEFORE_COORDINATE ?=
LANGGRAPH_SYNTHESIS_AFTER_COORDINATE ?=
LANGGRAPH_SYNTHESIS_FORK_PLAN ?=
LANGGRAPH_SYNTHESIS_BOUND_PROFILE ?=
LANGGRAPH_SYNTHESIS_RUNTIME_ROOT ?=
LANGGRAPH_SYNTHESIS_PASSIVE_SOCKET ?=

# MAF target
COPILOT_MODEL ?=
MAF_PYTHON ?=
MAF_TIMEOUT ?=
MAF_COPILOT_CLI ?=
MAF_SESSION_HOME ?=
MAF_LOG_LEVEL ?=
MAF_ALLOW_UNSUPPORTED_TASKS ?= false
MAF_WORKFLOW_TASK ?= $(if $(filter orphan-process,$(TARGET_TASK)),maf-workflow-checkpoint-continuity,$(TARGET_TASK))
MAF_WORKFLOW_EFFECT_SERVICE_URL ?=
MAF_WORKFLOW_FORK_ROOT ?= runs/maf-workflow-native-fork-smoke

# Advanced MAF-only provider overrides. Leave unset in the common path so
# OPENAI_API_KEY / OPENAI_BASE_URL are reused automatically.
# COPILOT_PROVIDER_BASE_URL ?=
# COPILOT_PROVIDER_TYPE ?= openai
# COPILOT_PROVIDER_API_KEY ?=

# Local repo settings
DOTENV_FILE ?= ./.env
GO_CACHE ?= /tmp/syncfuzz-go-cache
EBPF_BINARY ?= /tmp/syncfuzz-ebpf
EBPF_SUDO ?= sudo
EBPF_SMOKE_COMMAND ?= sh -c "touch frontier-marker; (sleep 3) >/dev/null 2>&1 &"
EBPF_SMOKE_EXPECT_FILES ?= frontier-marker
EBPF_SMOKE_OBSERVE_DELAY ?= 100ms
EBPF_FD_IDENTITY_COMMAND ?= sh -c "printf identity > held-fd; (exec 9<held-fd; rm held-fd; sleep 3) >/dev/null 2>&1 &"
EBPF_FD_IDENTITY_OBSERVE_DELAY ?= 100ms
CALIBRATION_PATH_RUN ?=
CALIBRATION_FD_RUN ?=
CALIBRATION_SOCKET_RUN ?=
CALIBRATION_AUDIT_OUT ?= runs/v2.2-link-calibration-audit.json
PROFILE_RESOURCES ?= false

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
TARGET_TASKS_ARGS := $(if $(TARGET_TASKS),--tasks $(TARGET_TASKS),)
TARGET_SEED_ARGS := $(if $(TARGET_SEED),--seed $(TARGET_SEED),)
TARGET_SEEDS_ARGS := $(if $(TARGET_SEEDS),--seeds $(TARGET_SEEDS),)
TARGET_GROUP_ARGS := $(if $(TARGET_GROUP),--group $(TARGET_GROUP),)
TARGET_GROUPS_ARGS := $(if $(TARGET_GROUPS),--groups $(TARGET_GROUPS),)
TARGET_PROMPT_PROFILE_ARGS := $(if $(TARGET_PROMPT_PROFILE),--prompt-profile $(TARGET_PROMPT_PROFILE),)
TARGET_PROMPT_PROFILES_ARGS := $(if $(TARGET_PROMPT_PROFILES),--prompt-profiles $(TARGET_PROMPT_PROFILES),)
TARGET_PROMPT_ARGS := $(if $(TARGET_PROMPT),--prompt "$(TARGET_PROMPT)",)
TARGET_PROMPT_FILE_ARGS := $(if $(TARGET_PROMPT_FILE),--prompt-file $(TARGET_PROMPT_FILE),)
TARGET_EXPECT_ARGS := $(if $(EXPECT_FILES),--expect-files $(EXPECT_FILES),)
TARGET_LATE_OBSERVE_ARGS := $(if $(TARGET_LATE_OBSERVE_DELAY),--late-observe-delay $(TARGET_LATE_OBSERVE_DELAY),)
EBPF_SMOKE_COMMAND_ARGS := --command '$(subst ','"'"',$(EBPF_SMOKE_COMMAND))'
EBPF_FD_IDENTITY_COMMAND_ARGS := --command '$(subst ','"'"',$(EBPF_FD_IDENTITY_COMMAND))'
PROFILE_RESOURCE_ARGS := $(if $(filter true,$(PROFILE_RESOURCES)),--profile-resources,)
LANGCHAIN_MODEL_ENV := $(if $(LANGCHAIN_MODEL),LANGCHAIN_MODEL='$(subst ','"'"',$(LANGCHAIN_MODEL))',)
OPENAI_API_KEY_ENV := $(if $(OPENAI_API_KEY),OPENAI_API_KEY='$(subst ','"'"',$(OPENAI_API_KEY))',)
OPENAI_BASE_URL_ENV := $(if $(OPENAI_BASE_URL),OPENAI_BASE_URL='$(subst ','"'"',$(OPENAI_BASE_URL))',)
COPILOT_MODEL_ENV := $(if $(COPILOT_MODEL),COPILOT_MODEL='$(subst ','"'"',$(COPILOT_MODEL))',)
COPILOT_PROVIDER_BASE_URL_ENV := $(if $(COPILOT_PROVIDER_BASE_URL),COPILOT_PROVIDER_BASE_URL='$(subst ','"'"',$(COPILOT_PROVIDER_BASE_URL))',)
COPILOT_PROVIDER_TYPE_ENV := $(if $(COPILOT_PROVIDER_TYPE),COPILOT_PROVIDER_TYPE='$(subst ','"'"',$(COPILOT_PROVIDER_TYPE))',)
COPILOT_PROVIDER_API_KEY_ENV := $(if $(COPILOT_PROVIDER_API_KEY),COPILOT_PROVIDER_API_KEY='$(subst ','"'"',$(COPILOT_PROVIDER_API_KEY))',)
MAF_PYTHON_ENV := $(if $(MAF_PYTHON),MAF_PYTHON='$(subst ','"'"',$(MAF_PYTHON))',)
MAF_TIMEOUT_ENV := $(if $(MAF_TIMEOUT),MAF_TIMEOUT='$(subst ','"'"',$(MAF_TIMEOUT))',)
MAF_COPILOT_CLI_ENV := $(if $(MAF_COPILOT_CLI),MAF_COPILOT_CLI='$(subst ','"'"',$(MAF_COPILOT_CLI))',)
MAF_SESSION_HOME_ENV := $(if $(MAF_SESSION_HOME),MAF_SESSION_HOME='$(subst ','"'"',$(MAF_SESSION_HOME))',)
MAF_LOG_LEVEL_ENV := $(if $(MAF_LOG_LEVEL),MAF_LOG_LEVEL='$(subst ','"'"',$(MAF_LOG_LEVEL))',)
MAF_ALLOW_UNSUPPORTED_ENV := $(if $(filter true,$(MAF_ALLOW_UNSUPPORTED_TASKS)),MAF_ALLOW_UNSUPPORTED_TASKS=true,)
MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV := $(if $(MAF_WORKFLOW_EFFECT_SERVICE_URL),MAF_WORKFLOW_EFFECT_SERVICE_URL='$(subst ','"'"',$(MAF_WORKFLOW_EFFECT_SERVICE_URL))',)
LOAD_DOTENV = set -a; test ! -f "$(DOTENV_FILE)" || . "$(DOTENV_FILE)"; set +a
SYNCFUZZ = GOCACHE=$(GO_CACHE) go run ./cmd/syncfuzz
RUN_ARGS = --out $(OUT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)
SUITE_ARGS = --out $(OUT) --corpus $(CORPUS) --repeat $(REPEAT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(CASE_ARGS) $(MOCK_ARGS) $(TIMING_ARGS)
CAMPAIGN_ARGS = --out $(OUT) --corpus $(CORPUS) --rounds $(ROUNDS) --repeat $(REPEAT) --delay $(DELAY) $(ENV_ARGS) $(CONTAINER_ARGS) $(CASE_ARGS) $(MOCK_ARGS) $(DIFFERENTIAL_ARGS) $(TIMING_ARGS) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS)
TARGET_RUN_ARGS = --out $(OUT) --timeout $(TARGET_TIMEOUT) --observe-delay $(TARGET_OBSERVE_DELAY) $(TARGET_LATE_OBSERVE_ARGS) $(ENV_ARGS) $(CONTAINER_ARGS) $(TARGET_PROMPT_ARGS) $(TARGET_PROMPT_FILE_ARGS) $(TARGET_EXPECT_ARGS)

.PHONY: help list fault-plans timing-profiles primitives matrix run-case run-pair run-mvp run-action run-authority run-shell run-fs run-branch run-suite run-diff-suite run-matrix-suite run-campaign target-list target-tasks target-seeds target-scenarios target-groups target-prompt-profiles target-matrix target-minimize target-run target-suite target-matrix-suite target-campaign target-profile-processes ebpf-build ebpf-profile-smoke ebpf-fd-identity-smoke ebpf-unix-socket-smoke ebpf-calibration-audit langgraph-profile-image synthesis-langgraph-profile synthesis-langgraph-bind-frontier synthesis-langgraph-prepare-fork target-langgraph-shell-react target-langgraph-shell-react-suite target-langgraph-shell-react-matrix-suite target-langgraph-shell-react-campaign target-langgraph-shell-react-check target-maf-github-copilot-shell target-maf-github-copilot-shell-suite target-maf-github-copilot-shell-matrix-suite target-maf-github-copilot-shell-campaign target-maf-github-copilot-shell-check target-maf-workflow-checkpoint target-maf-workflow-checkpoint-suite target-maf-workflow-checkpoint-check maf-workflow-native-fork-smoke corpus-list corpus-analyze corpus-show corpus-verify replay test-go fmt-go mock-build mock-start

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
	@echo "  make target-tasks"
	@echo "  make target-seeds"
	@echo "  make target-scenarios"
	@echo "  make target-groups"
	@echo "  make target-prompt-profiles"
	@echo "  make target-matrix TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all"
	@echo "  make target-minimize MINIMIZE_FROM=runs/target-suite-<id>/target-suite-result.json"
	@echo "  make target-minimize MINIMIZE_FROM=runs/target-suite-<id>/target-suite-result.json MINIMIZE_EXECUTE=true MINIMIZE_MAX_TRIALS=16"
	@echo "  make target-run TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh"
	@echo "  make ebpf-profile-smoke"
	@echo "  make ebpf-fd-identity-smoke"
	@echo "  make ebpf-unix-socket-smoke"
	@echo "  make ebpf-calibration-audit CALIBRATION_PATH_RUN=runs/<id> CALIBRATION_FD_RUN=runs/<id> CALIBRATION_SOCKET_RUN=runs/<id>"
	@echo "  make target-profile-processes TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh EXPECT_FILES=late-effect PROFILE_RESOURCES=true"
	@echo "  make target-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh REPEAT=3"
	@echo "  make target-matrix-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all"
	@echo "  make target-campaign TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all ROUNDS=2 CANDIDATE_LIMIT=3"
	@echo "  make target-langgraph-shell-react-check LANGCHAIN_MODEL=openai:gpt-4.1-mini"
	@echo "  make langgraph-profile-image LANGGRAPH_PROFILE_IMAGE=syncfuzz-langgraph:dev"
	@echo "  make synthesis-langgraph-profile LANGGRAPH_SYNTHESIS_OBJECTIVE=<objective.json> LANGGRAPH_SYNTHESIS_CANDIDATE=<candidate.json> LANGGRAPH_SYNTHESIS_ROOT=runs/<name>"
	@echo "  make synthesis-langgraph-bind-frontier LANGGRAPH_SYNTHESIS_OBJECTIVE=<objective.json> LANGGRAPH_SYNTHESIS_CANDIDATE=<candidate.json> LANGGRAPH_SYNTHESIS_ROOT=runs/<name> LANGGRAPH_SYNTHESIS_FRONTIER=before-command..after-command LANGGRAPH_SYNTHESIS_MANIFEST=runs/<target-run>/langgraph-native-checkpoints.json LANGGRAPH_SYNTHESIS_BINDING=runs/<name>/langgraph-native-frontier-binding.json LANGGRAPH_SYNTHESIS_BEFORE_COORDINATE=runs/<name>/before-coordinate.json LANGGRAPH_SYNTHESIS_AFTER_COORDINATE=runs/<name>/after-coordinate.json"
	@echo "  make synthesis-langgraph-prepare-fork LANGGRAPH_SYNTHESIS_OBJECTIVE=<objective.json> LANGGRAPH_SYNTHESIS_CANDIDATE=<candidate.json> LANGGRAPH_SYNTHESIS_ROOT=runs/<name> LANGGRAPH_SYNTHESIS_BINDING=runs/<name>/langgraph-native-frontier-binding.json LANGGRAPH_SYNTHESIS_FORK_PLAN=runs/<name>/langgraph-fork-plan.json LANGGRAPH_SYNTHESIS_BOUND_PROFILE=runs/<name>/bound-profile-run.json LANGGRAPH_SYNTHESIS_RUNTIME_ROOT=runs/<name>/recovery-runtimes LANGGRAPH_SYNTHESIS_PASSIVE_SOCKET=agent.sock"
	@echo "  make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini"
	@echo "  make target-langgraph-shell-react-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini REPEAT=3"
	@echo "  make target-langgraph-shell-react-matrix-suite TARGET_GROUP=phase5a-baseline REPEAT=1 CANDIDATE_LIMIT=3"
	@echo "  make target-langgraph-shell-react-campaign TARGET_GROUP=phase5a-baseline ROUNDS=2 CANDIDATE_LIMIT=3"
	@echo "  make target-langgraph-shell-react-suite TARGET_GROUP=workspace-residue REPEAT=5"
	@echo "  make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini OPENAI_BASE_URL=https://api.example.com/v1"
	@echo "  make target-langgraph-shell-react TARGET_TASK=orphan-process-long-delay"
	@echo "  make target-langgraph-shell-react TARGET_TASK=persistent-shell-poisoning-replay"
	@echo "  make target-langgraph-shell-react TARGET_TASK=persistent-shell-poisoning-fork"
	@echo "  make target-langgraph-shell-react TARGET_TASK=file-residue-fork"
	@echo "  make target-langgraph-shell-react TARGET_TASK=directory-residue-fork"
	@echo "  make target-langgraph-shell-react TARGET_TASK=delete-residue-fork"
	@echo "  make target-langgraph-shell-react TARGET_TASK=symlink-residue-fork"
	@echo "  make target-langgraph-shell-react LANGGRAPH_CHECKPOINT_BACKEND=disk"
	@echo "  make target-langgraph-shell-react TARGET_TASK=delete-residue-fork LANGGRAPH_PROCESS_MODE=split-process"
	@echo "  make target-maf-github-copilot-shell-check"
	@echo "  make target-maf-github-copilot-shell"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=persistent-shell-poisoning"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=env-residue"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=function-residue"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=cwd-residue"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=umask-residue"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=maf-session-continuity"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=persistent-shell-poisoning MAF_TIMEOUT=110"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=file-residue"
	@echo "  make target-maf-github-copilot-shell TARGET_TASK=rename-residue"
	@echo "  make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-http-effect-replay"
	@echo "  make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-http-effect-replay MAF_WORKFLOW_EFFECT_SERVICE_URL=http://127.0.0.1:8910"
	@echo "  make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-resource-replay"
	@echo "  make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-authority-token-replay"
	@echo "  make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-approval-pending-replay"
	@echo "  make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-rehydrate-divergence"
	@echo "  make maf-workflow-native-fork-smoke MAF_WORKFLOW_FORK_ROOT=runs/maf-v2.3-fork-smoke"
	@echo "  make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-baseline REPEAT=3"
	@echo "  make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-shell-context REPEAT=1"
	@echo "  make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-workspace-residue REPEAT=1"
	@echo "  make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-session REPEAT=1"
	@echo "  make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-phase5b REPEAT=1"
	@echo "  make target-maf-github-copilot-shell-matrix-suite TARGET_GROUP=maf-baseline TARGET_PROMPT_PROFILES=all REPEAT=1 CANDIDATE_LIMIT=3"
	@echo "  make target-maf-github-copilot-shell-campaign TARGET_GROUP=maf-baseline TARGET_PROMPT_PROFILES=all ROUNDS=2 CANDIDATE_LIMIT=3"
	@echo "  make corpus-list"
	@echo "  make corpus-analyze"
	@echo "  make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>"
	@echo "  make corpus-verify"
	@echo "  make replay ENTRY_ID=<entry_id_or_unique_prefix>"
	@echo "  make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest"
	@echo "Variables: OUT=$(OUT), CORPUS=$(CORPUS), DELAY=$(DELAY), ENV=$(ENV), CONTAINER_IMAGE=$(CONTAINER_IMAGE), LIMIT=$(LIMIT), VERIFY_LIMIT=$(VERIFY_LIMIT), ROUNDS=$(ROUNDS), DIFFERENTIAL=$(DIFFERENTIAL), TIMING=$(TIMING), INCLUDE_PLANNED=$(INCLUDE_PLANNED), FEEDBACK_FROM=$(FEEDBACK_FROM), CANDIDATE_LIMIT=$(CANDIDATE_LIMIT), TARGET_ADAPTER=$(TARGET_ADAPTER), TARGET_ID=$(TARGET_ID), TARGET_TASK=$(TARGET_TASK), TARGET_TASKS=$(TARGET_TASKS), TARGET_SEED=$(TARGET_SEED), TARGET_SEEDS=$(TARGET_SEEDS), TARGET_GROUP=$(TARGET_GROUP), TARGET_GROUPS=$(TARGET_GROUPS), TARGET_PROMPT_PROFILE=$(TARGET_PROMPT_PROFILE), TARGET_PROMPT_PROFILES=$(TARGET_PROMPT_PROFILES), TARGET_TIMEOUT=$(TARGET_TIMEOUT), TARGET_OBSERVE_DELAY=$(TARGET_OBSERVE_DELAY), TARGET_LATE_OBSERVE_DELAY=$(TARGET_LATE_OBSERVE_DELAY), TARGET_COMMAND_FILE=$(TARGET_COMMAND_FILE), EXPECT_FILES=$(EXPECT_FILES), LANGCHAIN_MODEL=$(LANGCHAIN_MODEL), OPENAI_API_KEY=$(OPENAI_API_KEY), OPENAI_BASE_URL=$(OPENAI_BASE_URL), COPILOT_MODEL=$(COPILOT_MODEL), COPILOT_PROVIDER_BASE_URL=$(COPILOT_PROVIDER_BASE_URL), COPILOT_PROVIDER_TYPE=$(COPILOT_PROVIDER_TYPE), COPILOT_PROVIDER_API_KEY=$(COPILOT_PROVIDER_API_KEY), LANGGRAPH_POLICY=$(LANGGRAPH_POLICY), LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE), LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND), LANGGRAPH_CHECKPOINT_DIR=$(LANGGRAPH_CHECKPOINT_DIR), LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE), LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY), LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX), LANGGRAPH_CHECKPOINT_SELECTOR=$(LANGGRAPH_CHECKPOINT_SELECTOR), LANGGRAPH_FORK_USER_MESSAGE=$(LANGGRAPH_FORK_USER_MESSAGE), MAF_PYTHON=$(MAF_PYTHON), MAF_TIMEOUT=$(MAF_TIMEOUT), MAF_COPILOT_CLI=$(MAF_COPILOT_CLI), MAF_SESSION_HOME=$(MAF_SESSION_HOME), MAF_LOG_LEVEL=$(MAF_LOG_LEVEL), MAF_ALLOW_UNSUPPORTED_TASKS=$(MAF_ALLOW_UNSUPPORTED_TASKS), MAF_WORKFLOW_EFFECT_SERVICE_URL=$(MAF_WORKFLOW_EFFECT_SERVICE_URL), DOTENV_FILE=$(DOTENV_FILE), MOCK_URL=$(MOCK_URL)"

list:
	$(SYNCFUZZ) list

fault-plans:
	$(SYNCFUZZ) fault-plans

timing-profiles:
	$(SYNCFUZZ) timing-profiles

primitives:
	$(SYNCFUZZ) primitives $(PLANNED_ARGS)

matrix:
	$(SYNCFUZZ) matrix $(CASE_ARGS) $(TIMING_ARGS) $(PLANNED_ARGS)

run-case:
	$(SYNCFUZZ) run --case $(CASE) $(RUN_ARGS)

run-pair:
	$(SYNCFUZZ) pair --case $(CASE) $(RUN_ARGS)

run-mvp:
	$(SYNCFUZZ) run --case orphan-process $(RUN_ARGS)

run-action:
	$(SYNCFUZZ) run --case action-replay $(RUN_ARGS)

run-authority:
	$(SYNCFUZZ) run --case authority-resurrection $(RUN_ARGS)

run-shell:
	$(SYNCFUZZ) run --case persistent-shell-poisoning $(RUN_ARGS)

run-fs:
	$(SYNCFUZZ) run --case partial-filesystem-rollback $(RUN_ARGS)

run-branch:
	$(SYNCFUZZ) run --case branch-leakage $(RUN_ARGS)

run-suite:
	$(SYNCFUZZ) suite $(SUITE_ARGS) $(DIFFERENTIAL_ARGS)

run-diff-suite:
	$(SYNCFUZZ) suite $(SUITE_ARGS) --differential

run-matrix-suite:
	$(SYNCFUZZ) suite --matrix $(SUITE_ARGS) $(DIFFERENTIAL_ARGS) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS)

run-campaign:
	$(SYNCFUZZ) campaign $(CAMPAIGN_ARGS)

target-list:
	$(SYNCFUZZ) target list

target-tasks:
	$(SYNCFUZZ) target tasks

target-seeds:
	$(SYNCFUZZ) target seeds

target-scenarios:
	$(SYNCFUZZ) target scenarios

target-groups:
	$(SYNCFUZZ) target groups

target-prompt-profiles:
	$(SYNCFUZZ) target prompt-profiles

target-matrix:
	$(SYNCFUZZ) target matrix --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS)

target-minimize:
	@test -n "$(MINIMIZE_FROM)" || (echo "usage: make target-minimize MINIMIZE_FROM=runs/target-suite-<id>/target-suite-result.json [MINIMIZE_EXECUTE=true]"; exit 2)
	$(SYNCFUZZ) target minimize --from $(MINIMIZE_FROM) --out $(OUT) $(if $(filter true,$(MINIMIZE_EXECUTE)),--execute --candidate-limit $(MINIMIZE_CANDIDATE_LIMIT) --max-trials $(MINIMIZE_MAX_TRIALS),)

target-run:
	$(LOAD_DOTENV); $(SYNCFUZZ) target run --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_RUN_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

# Build a binary with the generated eBPF objects embedded. Run `go generate
# ./internal/syncfuzz/profiling` first only when a profiling/*.bpf.c file changes.
ebpf-build:
	GOCACHE=$(GO_CACHE) go build -o $(EBPF_BINARY) ./cmd/syncfuzz

# The privileged smoke test proves cgroup-scoped process/resource events,
# controller checkpoints, state probes, and a checkpoint-effect frontier.
ebpf-profile-smoke: ebpf-build
	$(EBPF_SUDO) $(EBPF_BINARY) target run --env container --container-image $(CONTAINER_IMAGE) --profile-processes --profile-resources --timeout $(TARGET_TIMEOUT) --observe-delay $(EBPF_SMOKE_OBSERVE_DELAY) --out $(OUT) $(EBPF_SMOKE_COMMAND_ARGS) --expect-files $(EBPF_SMOKE_EXPECT_FILES)

# This keeps an unlinked workspace file open in FD 9. It verifies that the
# collector and checkpoint probe agree on its kernel (device,inode) identity.
ebpf-fd-identity-smoke: ebpf-build
	$(EBPF_SUDO) $(EBPF_BINARY) target run --env container --container-image $(CONTAINER_IMAGE) --profile-processes --profile-resources --timeout $(TARGET_TIMEOUT) --observe-delay $(EBPF_FD_IDENTITY_OBSERVE_DELAY) --out $(OUT) $(EBPF_FD_IDENTITY_COMMAND_ARGS)

# A filesystem-bound Unix listener requires the endpoint pathname, kernel
# socket, holder FD, and holder process to form one probe-confirmed closure.
ebpf-unix-socket-smoke: ebpf-build
	$(EBPF_SUDO) $(EBPF_BINARY) target run --env container --container-image $(CONTAINER_IMAGE) --profile-processes --profile-resources --timeout $(TARGET_TIMEOUT) --observe-delay $(EBPF_SMOKE_OBSERVE_DELAY) --out $(OUT) --command-file examples/target-commands/unix-socket-listener.sh

# Audits the three known-answer V2.2 runs without requiring BPF privileges.
# The resulting precision/recall is fixture-scoped, not a global coverage claim.
ebpf-calibration-audit:
	@test -n "$(CALIBRATION_PATH_RUN)" && test -n "$(CALIBRATION_FD_RUN)" && test -n "$(CALIBRATION_SOCKET_RUN)" || (echo "usage: make ebpf-calibration-audit CALIBRATION_PATH_RUN=runs/<id> CALIBRATION_FD_RUN=runs/<id> CALIBRATION_SOCKET_RUN=runs/<id> [CALIBRATION_AUDIT_OUT=runs/v2.2-link-calibration-audit.json]"; exit 2)
	$(SYNCFUZZ) profile calibration-audit --path-run $(CALIBRATION_PATH_RUN) --fd-run $(CALIBRATION_FD_RUN) --socket-run $(CALIBRATION_SOCKET_RUN) --out $(CALIBRATION_AUDIT_OUT)

# Use this for an arbitrary command adapter. EBPF_SUDO remains explicit so a
# caller can choose an environment-preserving sudo policy if credentials from
# .env must reach the target command.
target-profile-processes: ebpf-build
	@test -n "$(TARGET_COMMAND)$(TARGET_COMMAND_FILE)" || (echo "usage: make target-profile-processes TARGET_COMMAND=<command>|TARGET_COMMAND_FILE=<path> [EXPECT_FILES=file]"; exit 2)
	$(LOAD_DOTENV); $(EBPF_SUDO) $(EBPF_BINARY) target run --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_PROMPT_PROFILE_ARGS) --out $(OUT) --timeout $(TARGET_TIMEOUT) --observe-delay $(TARGET_OBSERVE_DELAY) $(TARGET_LATE_OBSERVE_ARGS) --env container --container-image $(CONTAINER_IMAGE) --profile-processes $(PROFILE_RESOURCE_ARGS) $(TARGET_EXPECT_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

target-suite:
	$(LOAD_DOTENV); $(SYNCFUZZ) target suite --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

target-matrix-suite:
	$(LOAD_DOTENV); $(SYNCFUZZ) target suite --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --matrix $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

target-campaign:
	$(LOAD_DOTENV); $(SYNCFUZZ) target campaign --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --rounds $(ROUNDS) --repeat $(REPEAT) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) --corpus $(CORPUS) --out $(OUT) $(TARGET_RUN_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

target-langgraph-shell-react:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND) SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR='$(LANGGRAPH_CHECKPOINT_DIR)' SYNCFUZZ_LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR='$(LANGGRAPH_CHECKPOINT_SELECTOR)' SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' $(SYNCFUZZ) target run --target langgraph-shell-react --task $(TARGET_TASK) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh

target-langgraph-shell-react-suite:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND) SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR='$(LANGGRAPH_CHECKPOINT_DIR)' SYNCFUZZ_LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR='$(LANGGRAPH_CHECKPOINT_SELECTOR)' SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' $(SYNCFUZZ) target suite --target langgraph-shell-react --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh

target-langgraph-shell-react-matrix-suite:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react-matrix-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND) SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR='$(LANGGRAPH_CHECKPOINT_DIR)' SYNCFUZZ_LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR='$(LANGGRAPH_CHECKPOINT_SELECTOR)' SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' $(SYNCFUZZ) target suite --target langgraph-shell-react --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --matrix $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh

target-langgraph-shell-react-campaign:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react-campaign LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND) SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR='$(LANGGRAPH_CHECKPOINT_DIR)' SYNCFUZZ_LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR='$(LANGGRAPH_CHECKPOINT_SELECTOR)' SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' $(SYNCFUZZ) target campaign --target langgraph-shell-react --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --rounds $(ROUNDS) --repeat $(REPEAT) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) --corpus $(CORPUS) --out $(OUT) $(TARGET_RUN_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh

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

# This image is intentionally separate from the ordinary local-venv wrapper.
# It is the unprivileged target runtime used by `synthesis execute-langgraph`;
# eBPF remains host-side and the command itself must opt into model-provider
# network access explicitly.
langgraph-profile-image:
	docker build --tag $(LANGGRAPH_PROFILE_IMAGE) --file targets/langgraph_shell_react/Dockerfile .

# This is a real synthesis candidate profile, not a legacy target task. The
# user must select a sudo policy that preserves exactly the provider variables
# needed by the target process; those values never enter JSON artifacts.
synthesis-langgraph-profile: ebpf-build
	@test -n "$(LANGGRAPH_SYNTHESIS_OBJECTIVE)" && test -n "$(LANGGRAPH_SYNTHESIS_CANDIDATE)" && test -n "$(LANGGRAPH_SYNTHESIS_ROOT)" || (echo "usage: make synthesis-langgraph-profile LANGGRAPH_SYNTHESIS_OBJECTIVE=<objective.json> LANGGRAPH_SYNTHESIS_CANDIDATE=<candidate.json> LANGGRAPH_SYNTHESIS_ROOT=runs/<name> [LANGGRAPH_PROFILE_IMAGE=syncfuzz-langgraph:dev]"; exit 2)
	@$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) test -n "$$LANGCHAIN_MODEL" || (echo "LANGCHAIN_MODEL is required in the shell or $(DOTENV_FILE)"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(EBPF_SUDO) --preserve-env=LANGCHAIN_MODEL,OPENAI_API_KEY,OPENAI_ADMIN_KEY,OPENAI_BASE_URL,ANTHROPIC_API_KEY $(EBPF_BINARY) synthesis execute-langgraph --objective $(LANGGRAPH_SYNTHESIS_OBJECTIVE) --candidate $(LANGGRAPH_SYNTHESIS_CANDIDATE) --allow-network --container-image $(LANGGRAPH_PROFILE_IMAGE) --timeout $(TARGET_TIMEOUT) --observe-delay $(TARGET_OBSERVE_DELAY) --out $(LANGGRAPH_SYNTHESIS_ROOT)/langgraph-candidate-execution.json --out-profile-run $(LANGGRAPH_SYNTHESIS_ROOT)/profile-run.json

# This is offline: it consumes a completed, timestamped native checkpoint
# manifest and refuses to substitute controller observation checkpoints.
synthesis-langgraph-bind-frontier:
	@test -n "$(LANGGRAPH_SYNTHESIS_OBJECTIVE)" && test -n "$(LANGGRAPH_SYNTHESIS_CANDIDATE)" && test -n "$(LANGGRAPH_SYNTHESIS_ROOT)" && test -n "$(LANGGRAPH_SYNTHESIS_FRONTIER)" && test -n "$(LANGGRAPH_SYNTHESIS_MANIFEST)" && test -n "$(LANGGRAPH_SYNTHESIS_BINDING)" || (echo "usage: make synthesis-langgraph-bind-frontier LANGGRAPH_SYNTHESIS_OBJECTIVE=<objective.json> LANGGRAPH_SYNTHESIS_CANDIDATE=<candidate.json> LANGGRAPH_SYNTHESIS_ROOT=runs/<name> LANGGRAPH_SYNTHESIS_FRONTIER=before-command..after-command LANGGRAPH_SYNTHESIS_MANIFEST=runs/<target-run>/langgraph-native-checkpoints.json LANGGRAPH_SYNTHESIS_BINDING=runs/<name>/langgraph-native-frontier-binding.json"; exit 2)
	$(SYNCFUZZ) synthesis bind-langgraph-frontier --objective $(LANGGRAPH_SYNTHESIS_OBJECTIVE) --candidate $(LANGGRAPH_SYNTHESIS_CANDIDATE) --profile-run $(LANGGRAPH_SYNTHESIS_ROOT)/profile-run.json --frontier $(LANGGRAPH_SYNTHESIS_FRONTIER) --manifest $(LANGGRAPH_SYNTHESIS_MANIFEST) --out-binding $(LANGGRAPH_SYNTHESIS_BINDING) $(if $(LANGGRAPH_SYNTHESIS_BEFORE_COORDINATE),--out-before-coordinate $(LANGGRAPH_SYNTHESIS_BEFORE_COORDINATE),) $(if $(LANGGRAPH_SYNTHESIS_AFTER_COORDINATE),--out-after-coordinate $(LANGGRAPH_SYNTHESIS_AFTER_COORDINATE),)

synthesis-langgraph-prepare-fork:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$(LANGGRAPH_SYNTHESIS_OBJECTIVE)" && test -n "$(LANGGRAPH_SYNTHESIS_CANDIDATE)" && test -n "$(LANGGRAPH_SYNTHESIS_ROOT)" && test -n "$(LANGGRAPH_SYNTHESIS_BINDING)" && test -n "$$model" && test -n "$(LANGGRAPH_SYNTHESIS_FORK_PLAN)" && test -n "$(LANGGRAPH_SYNTHESIS_BOUND_PROFILE)" && test -n "$(LANGGRAPH_SYNTHESIS_RUNTIME_ROOT)" && test -n "$(LANGGRAPH_SYNTHESIS_PASSIVE_SOCKET)" || (echo "usage: make synthesis-langgraph-prepare-fork LANGCHAIN_MODEL=<provider:model> LANGGRAPH_SYNTHESIS_OBJECTIVE=<objective.json> LANGGRAPH_SYNTHESIS_CANDIDATE=<candidate.json> LANGGRAPH_SYNTHESIS_ROOT=runs/<name> LANGGRAPH_SYNTHESIS_BINDING=runs/<name>/langgraph-native-frontier-binding.json LANGGRAPH_SYNTHESIS_FORK_PLAN=runs/<name>/langgraph-fork-plan.json LANGGRAPH_SYNTHESIS_BOUND_PROFILE=runs/<name>/bound-profile-run.json LANGGRAPH_SYNTHESIS_RUNTIME_ROOT=runs/<name>/recovery-runtimes LANGGRAPH_SYNTHESIS_PASSIVE_SOCKET=agent.sock"; exit 2)
	$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; $(SYNCFUZZ) synthesis prepare-langgraph-fork --objective $(LANGGRAPH_SYNTHESIS_OBJECTIVE) --candidate $(LANGGRAPH_SYNTHESIS_CANDIDATE) --profile-run $(LANGGRAPH_SYNTHESIS_ROOT)/profile-run.json --binding $(LANGGRAPH_SYNTHESIS_BINDING) --model "$$model" --container-image $(LANGGRAPH_PROFILE_IMAGE) --runtime-root $(LANGGRAPH_SYNTHESIS_RUNTIME_ROOT) --passive-unix-socket-path $(LANGGRAPH_SYNTHESIS_PASSIVE_SOCKET) --out-plan $(LANGGRAPH_SYNTHESIS_FORK_PLAN) --out-profile-run $(LANGGRAPH_SYNTHESIS_BOUND_PROFILE)

target-maf-github-copilot-shell-check:
	@$(LOAD_DOTENV); target_python="$(MAF_PYTHON)"; test -n "$$target_python" || target_python="$$MAF_PYTHON"; test -n "$$target_python" || target_python="targets/maf_github_copilot_shell/venv/bin/python"; test -x "$$target_python" || target_python="python3"; $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) "$$target_python" targets/maf_github_copilot_shell/run_target.py --check

target-maf-github-copilot-shell:
	$(LOAD_DOTENV); $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_PYTHON_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) $(MAF_ALLOW_UNSUPPORTED_ENV) $(SYNCFUZZ) target run --target maf-github-copilot-shell --task $(TARGET_TASK) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-github-copilot-shell.sh

target-maf-github-copilot-shell-suite:
	$(LOAD_DOTENV); $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_PYTHON_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) $(MAF_ALLOW_UNSUPPORTED_ENV) $(SYNCFUZZ) target suite --target maf-github-copilot-shell --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-github-copilot-shell.sh

target-maf-github-copilot-shell-matrix-suite:
	$(LOAD_DOTENV); $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_PYTHON_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) $(MAF_ALLOW_UNSUPPORTED_ENV) $(SYNCFUZZ) target suite --target maf-github-copilot-shell --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --matrix $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-github-copilot-shell.sh

target-maf-github-copilot-shell-campaign:
	$(LOAD_DOTENV); $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_PYTHON_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) $(MAF_ALLOW_UNSUPPORTED_ENV) $(SYNCFUZZ) target campaign --target maf-github-copilot-shell --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --rounds $(ROUNDS) --repeat $(REPEAT) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) --corpus $(CORPUS) --out $(OUT) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-github-copilot-shell.sh

target-maf-workflow-checkpoint-check:
	@$(LOAD_DOTENV); target_python="$(MAF_PYTHON)"; test -n "$$target_python" || target_python="$$MAF_PYTHON"; test -n "$$target_python" || target_python="targets/maf_github_copilot_shell/venv/bin/python"; test -x "$$target_python" || target_python="python3"; $(MAF_PYTHON_ENV) $(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) "$$target_python" targets/maf_workflow_checkpoint/run_target.py --check

target-maf-workflow-checkpoint:
	$(LOAD_DOTENV); $(MAF_PYTHON_ENV) $(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) $(SYNCFUZZ) target run --target maf-workflow-checkpoint --task $(MAF_WORKFLOW_TASK) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-workflow-checkpoint.sh

target-maf-workflow-checkpoint-suite:
	$(LOAD_DOTENV); $(MAF_PYTHON_ENV) $(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) $(SYNCFUZZ) target suite --target maf-workflow-checkpoint --task $(MAF_WORKFLOW_TASK) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-workflow-checkpoint.sh

# This is an adapter calibration, not a StateSeed or a coverage experiment.
# It prepares two exact MAF Workflow file checkpoints, then restores each in a
# separate Python process and separate cloned workspace. The target refuses to
# overwrite an existing root so each invocation remains auditable.
maf-workflow-native-fork-smoke:
	@$(LOAD_DOTENV); set -e; \
	target_python="$(MAF_PYTHON)"; test -n "$$target_python" || target_python="$$MAF_PYTHON"; test -n "$$target_python" || target_python="targets/maf_github_copilot_shell/venv/bin/python"; test -x "$$target_python" || target_python="python3"; \
	root="$(MAF_WORKFLOW_FORK_ROOT)"; test ! -e "$$root" || (echo "refusing to overwrite existing MAF fork root: $$root"; exit 2); \
	mkdir -p "$$root"; \
	$(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) "$$target_python" targets/maf_workflow_checkpoint/run_target.py --mode prepare-fork --workspace "$$root/prepared" --task-id maf-workflow-checkpoint-continuity; \
	before_id="$$($$target_python -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["native_checkpoints"][0]["checkpoint_id"])' "$$root/prepared/maf-workflow-fork-manifest.json")"; \
	after_id="$$($$target_python -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["native_checkpoints"][1]["checkpoint_id"])' "$$root/prepared/maf-workflow-fork-manifest.json")"; \
	$(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) "$$target_python" targets/maf_workflow_checkpoint/run_target.py --mode fork-observe --source-workspace "$$root/prepared" --workspace "$$root/before" --task-id maf-workflow-checkpoint-continuity --checkpoint-id "$$before_id" --runtime-instance-id "maf-workflow-smoke-before"; \
	$(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) "$$target_python" targets/maf_workflow_checkpoint/run_target.py --mode fork-observe --source-workspace "$$root/prepared" --workspace "$$root/after" --task-id maf-workflow-checkpoint-continuity --checkpoint-id "$$after_id" --runtime-instance-id "maf-workflow-smoke-after"; \
	echo "MAF native fork calibration artifacts: $$root"

corpus-list:
	$(SYNCFUZZ) corpus list --corpus $(CORPUS) --limit $(LIMIT)

corpus-analyze:
	$(SYNCFUZZ) corpus analyze --corpus $(CORPUS) --limit $(LIMIT)

corpus-show:
	@test -n "$(ENTRY_ID)" || (echo "usage: make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>"; exit 2)
	$(SYNCFUZZ) corpus show --corpus $(CORPUS) --id $(ENTRY_ID)

corpus-verify:
	$(SYNCFUZZ) corpus verify --corpus $(CORPUS) --limit $(VERIFY_LIMIT) $(RUN_ARGS)

replay:
	@test -n "$(ENTRY_ID)" || (echo "usage: make replay ENTRY_ID=<entry_id_or_unique_prefix>"; exit 2)
	$(SYNCFUZZ) replay --corpus $(CORPUS) --id $(ENTRY_ID) $(RUN_ARGS)

test-go:
	GOCACHE=$(GO_CACHE) go test ./...

fmt-go:
	gofmt -w cmd internal

mock-build:
	cd services/mock-servers && npm run build

mock-start:
	cd services/mock-servers && npm run start
