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
MINIMIZE_FIDELITY ?= exact

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
TARGET_SELECTION_POLICY ?=
TARGET_RANDOM_SEED ?= 1
TARGET_MIN_COVERAGE_GAIN_SCORE ?= 0
TARGET_MAX_STAGNANT_ROUNDS ?= 0
TARGET_AUTO_PIVOT ?= false
TARGET_OBSERVATION_RUN ?=
TARGET_FOOTPRINT ?=
TARGET_OBSERVATION_PLAN ?=
TARGET_OBSERVATION_MODE ?=
TARGET_FALLBACK_REPORT ?=
TARGET_REFINED_OBSERVATION_PLAN ?=
TARGET_CONTROL_RUN ?=
TARGET_COMPARE_RUN ?=
TARGET_PAIR_DIFFERENTIAL ?=
TARGET_PAIR_CAMPAIGN_MANIFEST ?=
TARGET_PAIR_CAMPAIGN_OUT ?=
TARGET_PAIR_REPORTS ?=
TARGET_PAIR_CALIBRATION_SUMMARY ?=
TARGET_PAIR_REVIEW_MANIFESTS ?=
TARGET_RUNTIME_PAIR_CONTROL_KIND ?=
TARGET_RUNTIME_PAIR_CONTROL_DESCRIPTION ?=
TARGET_RUNTIME_PAIR_CONTROL_COMMAND ?=
TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE ?=
TARGET_CONTRACT_CANDIDATES ?=
TARGET_CONTRACT_SOURCE_ROOT ?=
TARGET_CONTRACT_CANDIDATE_REPORT ?=
EXPECT_FILES ?=

# Phase 5B feedback experiment v3
PHASE5B_V3_BLOCK ?= 1
PHASE5B_V3_OUT ?= runs/phase5b-feedback-v3/block-$(PHASE5B_V3_BLOCK)
PHASE5B_V3_CORPUS ?= corpus/phase5b-feedback-v3/block-$(PHASE5B_V3_BLOCK)
PHASE5B_V3_SEEDS ?= active-ipc-residue,active-ipc-residue-fork,capability-residue-fork,delayed-effect,shell-execution-context-residue,shell-execution-context-residue-fork,shell-path-residue
PHASE5B_V3_ROUNDS ?= 4
PHASE5B_V3_CANDIDATE_LIMIT ?= 8
PHASE5B_V3_TIMEOUT ?= 5m
PHASE5B_V3_RANDOM_SEED ?= $(PHASE5B_V3_BLOCK)
PHASE5B_V3_MIN_COVERAGE_GAIN_SCORE ?= 500
PHASE5B_V3_MAX_STAGNANT_ROUNDS ?= 1

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

# Advanced MAF-only provider overrides. Leave unset in the common path so
# OPENAI_API_KEY / OPENAI_BASE_URL are reused automatically.
# COPILOT_PROVIDER_BASE_URL ?=
# COPILOT_PROVIDER_TYPE ?= openai
# COPILOT_PROVIDER_API_KEY ?=

# Local repo settings
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
TARGET_RUNTIME_PAIR_CONTROL_COMMAND_ARGS := $(if $(TARGET_RUNTIME_PAIR_CONTROL_COMMAND),--control-command '$(subst ','"'"',$(TARGET_RUNTIME_PAIR_CONTROL_COMMAND))',)
TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE_ARGS := $(if $(TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE),--control-command-file $(TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE),)
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
TARGET_OBSERVATION_PLAN_ARGS := $(if $(TARGET_OBSERVATION_PLAN),--observation-plan $(TARGET_OBSERVATION_PLAN),)
TARGET_OBSERVATION_MODE_ARGS := $(if $(TARGET_OBSERVATION_MODE),--observation-mode $(TARGET_OBSERVATION_MODE),)
TARGET_SELECTION_ARGS := $(if $(TARGET_SELECTION_POLICY),--selection-policy $(TARGET_SELECTION_POLICY) --random-seed $(TARGET_RANDOM_SEED),)
TARGET_CAMPAIGN_CONTROL_ARGS := $(if $(filter-out 0,$(TARGET_MIN_COVERAGE_GAIN_SCORE)),--min-coverage-gain-score $(TARGET_MIN_COVERAGE_GAIN_SCORE),) $(if $(filter-out 0,$(TARGET_MAX_STAGNANT_ROUNDS)),--max-stagnant-rounds $(TARGET_MAX_STAGNANT_ROUNDS),) $(if $(filter true,$(TARGET_AUTO_PIVOT)),--auto-pivot,)
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
TARGET_RUN_ARGS = --out $(OUT) --timeout $(TARGET_TIMEOUT) --observe-delay $(TARGET_OBSERVE_DELAY) $(TARGET_LATE_OBSERVE_ARGS) $(TARGET_OBSERVATION_PLAN_ARGS) $(TARGET_OBSERVATION_MODE_ARGS) $(ENV_ARGS) $(CONTAINER_ARGS) $(TARGET_PROMPT_ARGS) $(TARGET_PROMPT_FILE_ARGS) $(TARGET_EXPECT_ARGS)

.PHONY: help list fault-plans timing-profiles primitives matrix run-case run-pair run-mvp run-action run-authority run-shell run-fs run-branch run-suite run-diff-suite run-matrix-suite run-campaign target-list target-tasks target-seeds target-scenarios target-groups target-prompt-profiles target-footprint target-plan-probes target-refine-plan target-runtime-pair target-matrix target-minimize target-run target-suite target-matrix-suite target-campaign target-langgraph-shell-react target-langgraph-shell-react-suite target-langgraph-shell-react-matrix-suite target-langgraph-shell-react-campaign target-langgraph-shell-react-check target-maf-github-copilot-shell target-maf-github-copilot-shell-suite target-maf-github-copilot-shell-matrix-suite target-maf-github-copilot-shell-campaign target-maf-github-copilot-shell-check target-maf-workflow-checkpoint target-maf-workflow-checkpoint-suite target-maf-workflow-checkpoint-check phase5b-v3-fixed phase5b-v3-random phase5b-v3-feedback phase5b-v3-full corpus-list corpus-analyze corpus-show corpus-verify replay test-go fmt-go mock-build mock-start

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
	@echo "  make target-minimize MINIMIZE_FROM=runs/target-suite-<id>/target-suite-result.json MINIMIZE_EXECUTE=true MINIMIZE_MAX_TRIALS=16 MINIMIZE_FIDELITY=semantic"
	@echo "  make target-run TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh"
	@echo "  make target-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh REPEAT=3"
	@echo "  make target-matrix-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all"
	@echo "  make target-campaign TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all ROUNDS=2 CANDIDATE_LIMIT=3 TARGET_SELECTION_POLICY=random"
	@echo "  make target-langgraph-shell-react-check LANGCHAIN_MODEL=openai:gpt-4.1-mini"
	@echo "  make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini"
	@echo "  make target-langgraph-shell-react-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini REPEAT=3"
	@echo "  make target-langgraph-shell-react-matrix-suite TARGET_GROUP=phase5a-baseline REPEAT=1 CANDIDATE_LIMIT=3"
	@echo "  make target-langgraph-shell-react-campaign TARGET_GROUP=phase5a-baseline ROUNDS=2 CANDIDATE_LIMIT=3"
	@echo "  make phase5b-v3-feedback PHASE5B_V3_BLOCK=1 LANGCHAIN_MODEL=openai:gpt-4.1-mini"
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
	@echo "  make target-footprint TARGET_OBSERVATION_RUN=runs/<target-run-id>"
	@echo "  make target-plan-probes TARGET_FOOTPRINT=runs/<target-run-id>/resource-footprint.json"
	@echo "  make target-refine-plan TARGET_OBSERVATION_PLAN=runs/<target-run-id>/observation-plan.json TARGET_FALLBACK_REPORT=runs/<target-run-id>/targeted-probe-report.json"
	@echo "  make target-compare TARGET_CONTROL_RUN=runs/<control-run-id> TARGET_COMPARE_RUN=runs/<target-run-id>"
	@echo "  make target-runtime-pair TARGET_RUNTIME_PAIR_CONTROL_KIND=fresh-runtime TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE=control.sh TARGET_COMMAND_FILE=target.sh"
	@echo "  make target-pair-campaign TARGET_PAIR_CAMPAIGN_MANIFEST=target-pair-campaign.json TARGET_PAIR_CAMPAIGN_OUT=runs/<pair-campaign>"
	@echo "  make target-calibration-summary TARGET_PAIR_REPORTS=runs/<pair-campaign> TARGET_PAIR_CALIBRATION_SUMMARY=runs/<pair-campaign>/target-pair-calibration-summary.json [TARGET_PAIR_REVIEW_MANIFESTS=reviews.json]"
	@echo "  make target-contract-candidates TARGET_CONTRACT_CANDIDATES=contract-candidates.json TARGET_CONTRACT_SOURCE_ROOT=<source-root> TARGET_CONTRACT_CANDIDATE_REPORT=runs/target-contract-candidate-validation.json"
	@echo "  make target-signatures"
	@echo "  make target-run TARGET_OBSERVATION_PLAN=runs/<target-run-id>/observation-plan.json"
	@echo "  make target-run TARGET_OBSERVATION_PLAN=runs/<target-run-id>/observation-plan.json TARGET_OBSERVATION_MODE=pruned-filesystem"
	@echo "  make target-run TARGET_OBSERVATION_PLAN=runs/<target-run-id>/observation-plan.json TARGET_OBSERVATION_MODE=pruned ENV=local"
	@echo "  make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest"
	@echo "Variables: OUT=$(OUT), CORPUS=$(CORPUS), DELAY=$(DELAY), ENV=$(ENV), CONTAINER_IMAGE=$(CONTAINER_IMAGE), LIMIT=$(LIMIT), VERIFY_LIMIT=$(VERIFY_LIMIT), ROUNDS=$(ROUNDS), DIFFERENTIAL=$(DIFFERENTIAL), TIMING=$(TIMING), INCLUDE_PLANNED=$(INCLUDE_PLANNED), FEEDBACK_FROM=$(FEEDBACK_FROM), CANDIDATE_LIMIT=$(CANDIDATE_LIMIT), TARGET_ADAPTER=$(TARGET_ADAPTER), TARGET_ID=$(TARGET_ID), TARGET_TASK=$(TARGET_TASK), TARGET_TASKS=$(TARGET_TASKS), TARGET_SEED=$(TARGET_SEED), TARGET_SEEDS=$(TARGET_SEEDS), TARGET_GROUP=$(TARGET_GROUP), TARGET_GROUPS=$(TARGET_GROUPS), TARGET_PROMPT_PROFILE=$(TARGET_PROMPT_PROFILE), TARGET_PROMPT_PROFILES=$(TARGET_PROMPT_PROFILES), TARGET_TIMEOUT=$(TARGET_TIMEOUT), TARGET_OBSERVE_DELAY=$(TARGET_OBSERVE_DELAY), TARGET_LATE_OBSERVE_DELAY=$(TARGET_LATE_OBSERVE_DELAY), TARGET_SELECTION_POLICY=$(TARGET_SELECTION_POLICY), TARGET_RANDOM_SEED=$(TARGET_RANDOM_SEED), TARGET_COMMAND_FILE=$(TARGET_COMMAND_FILE), TARGET_CONTRACT_CANDIDATES=$(TARGET_CONTRACT_CANDIDATES), TARGET_CONTRACT_SOURCE_ROOT=$(TARGET_CONTRACT_SOURCE_ROOT), TARGET_CONTRACT_CANDIDATE_REPORT=$(TARGET_CONTRACT_CANDIDATE_REPORT), EXPECT_FILES=$(EXPECT_FILES), LANGCHAIN_MODEL=$(LANGCHAIN_MODEL), OPENAI_API_KEY=$(OPENAI_API_KEY), OPENAI_BASE_URL=$(OPENAI_BASE_URL), COPILOT_MODEL=$(COPILOT_MODEL), COPILOT_PROVIDER_BASE_URL=$(COPILOT_PROVIDER_BASE_URL), COPILOT_PROVIDER_TYPE=$(COPILOT_PROVIDER_TYPE), COPILOT_PROVIDER_API_KEY=$(COPILOT_PROVIDER_API_KEY), LANGGRAPH_POLICY=$(LANGGRAPH_POLICY), LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE), LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND), LANGGRAPH_CHECKPOINT_DIR=$(LANGGRAPH_CHECKPOINT_DIR), LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE), LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY), LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX), LANGGRAPH_CHECKPOINT_SELECTOR=$(LANGGRAPH_CHECKPOINT_SELECTOR), LANGGRAPH_FORK_USER_MESSAGE=$(LANGGRAPH_FORK_USER_MESSAGE), MAF_PYTHON=$(MAF_PYTHON), MAF_TIMEOUT=$(MAF_TIMEOUT), MAF_COPILOT_CLI=$(MAF_COPILOT_CLI), MAF_SESSION_HOME=$(MAF_SESSION_HOME), MAF_LOG_LEVEL=$(MAF_LOG_LEVEL), MAF_ALLOW_UNSUPPORTED_TASKS=$(MAF_ALLOW_UNSUPPORTED_TASKS), MAF_WORKFLOW_EFFECT_SERVICE_URL=$(MAF_WORKFLOW_EFFECT_SERVICE_URL), DOTENV_FILE=$(DOTENV_FILE), MOCK_URL=$(MOCK_URL)"

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

target-signatures:
	$(SYNCFUZZ) target signatures

target-groups:
	$(SYNCFUZZ) target groups

target-prompt-profiles:
	$(SYNCFUZZ) target prompt-profiles

target-footprint:
	@test -n "$(TARGET_OBSERVATION_RUN)" || (echo "usage: make target-footprint TARGET_OBSERVATION_RUN=runs/<target-run-id> [TARGET_FOOTPRINT=path]"; exit 2)
	$(SYNCFUZZ) target footprint --run $(TARGET_OBSERVATION_RUN) $(if $(TARGET_FOOTPRINT),--out $(TARGET_FOOTPRINT),)

target-plan-probes:
	@test -n "$(TARGET_FOOTPRINT)" || (echo "usage: make target-plan-probes TARGET_FOOTPRINT=runs/<target-run-id>/resource-footprint.json [TARGET_OBSERVATION_PLAN=path]"; exit 2)
	$(SYNCFUZZ) target plan-probes --footprint $(TARGET_FOOTPRINT) $(if $(TARGET_OBSERVATION_PLAN),--out $(TARGET_OBSERVATION_PLAN),)

target-refine-plan:
	@test -n "$(TARGET_OBSERVATION_PLAN)" || (echo "usage: make target-refine-plan TARGET_OBSERVATION_PLAN=runs/<target-run-id>/observation-plan.json TARGET_FALLBACK_REPORT=runs/<target-run-id>/targeted-probe-report.json [TARGET_REFINED_OBSERVATION_PLAN=path]"; exit 2)
	@test -n "$(TARGET_FALLBACK_REPORT)" || (echo "usage: make target-refine-plan TARGET_OBSERVATION_PLAN=runs/<target-run-id>/observation-plan.json TARGET_FALLBACK_REPORT=runs/<target-run-id>/targeted-probe-report.json [TARGET_REFINED_OBSERVATION_PLAN=path]"; exit 2)
	$(SYNCFUZZ) target refine-plan --plan $(TARGET_OBSERVATION_PLAN) --fallback-report $(TARGET_FALLBACK_REPORT) $(if $(TARGET_REFINED_OBSERVATION_PLAN),--out $(TARGET_REFINED_OBSERVATION_PLAN),)

target-compare:
	@test -n "$(TARGET_CONTROL_RUN)" || (echo "usage: make target-compare TARGET_CONTROL_RUN=runs/<control-run-id> TARGET_COMPARE_RUN=runs/<target-run-id> [TARGET_PAIR_DIFFERENTIAL=path]"; exit 2)
	@test -n "$(TARGET_COMPARE_RUN)" || (echo "usage: make target-compare TARGET_CONTROL_RUN=runs/<control-run-id> TARGET_COMPARE_RUN=runs/<target-run-id> [TARGET_PAIR_DIFFERENTIAL=path]"; exit 2)
	$(SYNCFUZZ) target compare --control $(TARGET_CONTROL_RUN) --target $(TARGET_COMPARE_RUN) $(if $(TARGET_PAIR_DIFFERENTIAL),--out $(TARGET_PAIR_DIFFERENTIAL),)

target-runtime-pair:
	@test -n "$(TARGET_RUNTIME_PAIR_CONTROL_KIND)" || (echo "usage: make target-runtime-pair TARGET_RUNTIME_PAIR_CONTROL_KIND=fresh-runtime TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE=control.sh TARGET_COMMAND_FILE=target.sh"; exit 2)
	@test -n "$(TARGET_RUNTIME_PAIR_CONTROL_COMMAND)$(TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE)" || (echo "usage: make target-runtime-pair TARGET_RUNTIME_PAIR_CONTROL_KIND=fresh-runtime TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE=control.sh TARGET_COMMAND_FILE=target.sh"; exit 2)
	@test -n "$(TARGET_COMMAND)$(TARGET_COMMAND_FILE)" || (echo "usage: make target-runtime-pair TARGET_RUNTIME_PAIR_CONTROL_KIND=fresh-runtime TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE=control.sh TARGET_COMMAND_FILE=target.sh"; exit 2)
	$(SYNCFUZZ) target runtime-pair --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) --control-kind $(TARGET_RUNTIME_PAIR_CONTROL_KIND) $(if $(TARGET_RUNTIME_PAIR_CONTROL_DESCRIPTION),--control-description "$(TARGET_RUNTIME_PAIR_CONTROL_DESCRIPTION)",) $(TARGET_RUNTIME_PAIR_CONTROL_COMMAND_ARGS) $(TARGET_RUNTIME_PAIR_CONTROL_COMMAND_FILE_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS) --out $(OUT) --timeout $(TARGET_TIMEOUT) --observe-delay $(TARGET_OBSERVE_DELAY) $(TARGET_LATE_OBSERVE_ARGS) $(TARGET_OBSERVATION_PLAN_ARGS) $(TARGET_OBSERVATION_MODE_ARGS) $(ENV_ARGS) $(CONTAINER_ARGS) $(TARGET_PROMPT_ARGS) $(TARGET_PROMPT_FILE_ARGS) $(TARGET_EXPECT_ARGS)

target-pair-campaign:
	@test -n "$(TARGET_PAIR_CAMPAIGN_MANIFEST)" || (echo "usage: make target-pair-campaign TARGET_PAIR_CAMPAIGN_MANIFEST=target-pair-campaign.json TARGET_PAIR_CAMPAIGN_OUT=runs/<pair-campaign>"; exit 2)
	@test -n "$(TARGET_PAIR_CAMPAIGN_OUT)" || (echo "usage: make target-pair-campaign TARGET_PAIR_CAMPAIGN_MANIFEST=target-pair-campaign.json TARGET_PAIR_CAMPAIGN_OUT=runs/<pair-campaign>"; exit 2)
	$(SYNCFUZZ) target pair-campaign --manifest $(TARGET_PAIR_CAMPAIGN_MANIFEST) --out $(TARGET_PAIR_CAMPAIGN_OUT)

target-calibration-summary:
	@test -n "$(TARGET_PAIR_REPORTS)" || (echo "usage: make target-calibration-summary TARGET_PAIR_REPORTS=runs/<pair-campaign>[,runs/<target-run-id>/target-pair-differential.json] TARGET_PAIR_CALIBRATION_SUMMARY=path [TARGET_PAIR_REVIEW_MANIFESTS=reviews.json]"; exit 2)
	@test -n "$(TARGET_PAIR_CALIBRATION_SUMMARY)" || (echo "usage: make target-calibration-summary TARGET_PAIR_REPORTS=runs/<pair-campaign>[,runs/<target-run-id>/target-pair-differential.json] TARGET_PAIR_CALIBRATION_SUMMARY=path [TARGET_PAIR_REVIEW_MANIFESTS=reviews.json]"; exit 2)
	$(SYNCFUZZ) target calibration-summary --inputs $(TARGET_PAIR_REPORTS) $(if $(TARGET_PAIR_REVIEW_MANIFESTS),--review-manifests $(TARGET_PAIR_REVIEW_MANIFESTS),) --out $(TARGET_PAIR_CALIBRATION_SUMMARY)

target-contract-candidates:
	@test -n "$(TARGET_CONTRACT_CANDIDATES)" || (echo "usage: make target-contract-candidates TARGET_CONTRACT_CANDIDATES=contract-candidates.json TARGET_CONTRACT_SOURCE_ROOT=<source-root> TARGET_CONTRACT_CANDIDATE_REPORT=path"; exit 2)
	@test -n "$(TARGET_CONTRACT_SOURCE_ROOT)" || (echo "usage: make target-contract-candidates TARGET_CONTRACT_CANDIDATES=contract-candidates.json TARGET_CONTRACT_SOURCE_ROOT=<source-root> TARGET_CONTRACT_CANDIDATE_REPORT=path"; exit 2)
	@test -n "$(TARGET_CONTRACT_CANDIDATE_REPORT)" || (echo "usage: make target-contract-candidates TARGET_CONTRACT_CANDIDATES=contract-candidates.json TARGET_CONTRACT_SOURCE_ROOT=<source-root> TARGET_CONTRACT_CANDIDATE_REPORT=path"; exit 2)
	$(SYNCFUZZ) target contract-candidates --input $(TARGET_CONTRACT_CANDIDATES) --source-root $(TARGET_CONTRACT_SOURCE_ROOT) --out $(TARGET_CONTRACT_CANDIDATE_REPORT)

target-matrix:
	$(SYNCFUZZ) target matrix --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS)

target-minimize:
	@test -n "$(MINIMIZE_FROM)" || (echo "usage: make target-minimize MINIMIZE_FROM=runs/target-suite-<id>/target-suite-result.json [MINIMIZE_EXECUTE=true]"; exit 2)
	$(SYNCFUZZ) target minimize --from $(MINIMIZE_FROM) --out $(OUT) $(if $(filter true,$(MINIMIZE_EXECUTE)),--execute --candidate-limit $(MINIMIZE_CANDIDATE_LIMIT) --max-trials $(MINIMIZE_MAX_TRIALS) --fidelity $(MINIMIZE_FIDELITY),)

target-run:
	$(LOAD_DOTENV); $(SYNCFUZZ) target run --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_RUN_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

target-suite:
	$(LOAD_DOTENV); $(SYNCFUZZ) target suite --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

target-matrix-suite:
	$(LOAD_DOTENV); $(SYNCFUZZ) target suite --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --matrix $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) $(TARGET_SELECTION_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

target-campaign:
	$(LOAD_DOTENV); $(SYNCFUZZ) target campaign --adapter $(TARGET_ADAPTER) --target $(TARGET_ID) --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --rounds $(ROUNDS) --repeat $(REPEAT) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) $(TARGET_SELECTION_ARGS) $(TARGET_CAMPAIGN_CONTROL_ARGS) --corpus $(CORPUS) $(TARGET_RUN_ARGS) $(TARGET_COMMAND_ARGS) $(TARGET_COMMAND_FILE_ARGS)

target-langgraph-shell-react:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND) SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR='$(LANGGRAPH_CHECKPOINT_DIR)' SYNCFUZZ_LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR='$(LANGGRAPH_CHECKPOINT_SELECTOR)' SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' $(SYNCFUZZ) target run --target langgraph-shell-react --task $(TARGET_TASK) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh

target-langgraph-shell-react-suite:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND) SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR='$(LANGGRAPH_CHECKPOINT_DIR)' SYNCFUZZ_LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR='$(LANGGRAPH_CHECKPOINT_SELECTOR)' SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' $(SYNCFUZZ) target suite --target langgraph-shell-react --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh

target-langgraph-shell-react-matrix-suite:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react-matrix-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND) SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR='$(LANGGRAPH_CHECKPOINT_DIR)' SYNCFUZZ_LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR='$(LANGGRAPH_CHECKPOINT_SELECTOR)' SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' $(SYNCFUZZ) target suite --target langgraph-shell-react --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --matrix $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) $(TARGET_SELECTION_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh

target-langgraph-shell-react-campaign:
	@$(LOAD_DOTENV); model="$(LANGCHAIN_MODEL)"; test -n "$$model" || model="$$LANGCHAIN_MODEL"; test -n "$$model" || (echo "usage: make target-langgraph-shell-react-campaign LANGCHAIN_MODEL=openai:gpt-4.1-mini"; exit 2)
	$(LOAD_DOTENV); $(LANGCHAIN_MODEL_ENV) $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE=true SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=$(LANGGRAPH_POLICY) SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE=$(LANGGRAPH_DOCKER_IMAGE) SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND=$(LANGGRAPH_CHECKPOINT_BACKEND) SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR='$(LANGGRAPH_CHECKPOINT_DIR)' SYNCFUZZ_LANGGRAPH_PROCESS_MODE=$(LANGGRAPH_PROCESS_MODE) SYNCFUZZ_LANGGRAPH_REPLAY=$(LANGGRAPH_REPLAY) SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=$(LANGGRAPH_CHECKPOINT_INDEX) SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR='$(LANGGRAPH_CHECKPOINT_SELECTOR)' SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='$(LANGGRAPH_FORK_USER_MESSAGE)' $(SYNCFUZZ) target campaign --target langgraph-shell-react --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --rounds $(ROUNDS) --repeat $(REPEAT) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) $(TARGET_SELECTION_ARGS) $(TARGET_CAMPAIGN_CONTROL_ARGS) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/langgraph-shell-react.sh

phase5b-v3-fixed:
	$(MAKE) --no-print-directory target-langgraph-shell-react-campaign OUT=$(PHASE5B_V3_OUT)/fixed CORPUS=$(PHASE5B_V3_CORPUS)/fixed TARGET_SEEDS=$(PHASE5B_V3_SEEDS) TARGET_PROMPT_PROFILES=baseline ROUNDS=$(PHASE5B_V3_ROUNDS) CANDIDATE_LIMIT=$(PHASE5B_V3_CANDIDATE_LIMIT) TARGET_TIMEOUT=$(PHASE5B_V3_TIMEOUT) TARGET_SELECTION_POLICY=fixed

phase5b-v3-random:
	$(MAKE) --no-print-directory target-langgraph-shell-react-campaign OUT=$(PHASE5B_V3_OUT)/random CORPUS=$(PHASE5B_V3_CORPUS)/random TARGET_SEEDS=$(PHASE5B_V3_SEEDS) TARGET_PROMPT_PROFILES=baseline ROUNDS=$(PHASE5B_V3_ROUNDS) CANDIDATE_LIMIT=$(PHASE5B_V3_CANDIDATE_LIMIT) TARGET_TIMEOUT=$(PHASE5B_V3_TIMEOUT) TARGET_SELECTION_POLICY=random TARGET_RANDOM_SEED=$(PHASE5B_V3_RANDOM_SEED)

phase5b-v3-feedback:
	$(MAKE) --no-print-directory target-langgraph-shell-react-campaign OUT=$(PHASE5B_V3_OUT)/feedback CORPUS=$(PHASE5B_V3_CORPUS)/feedback TARGET_SEEDS=$(PHASE5B_V3_SEEDS) TARGET_PROMPT_PROFILES=baseline ROUNDS=$(PHASE5B_V3_ROUNDS) CANDIDATE_LIMIT=$(PHASE5B_V3_CANDIDATE_LIMIT) TARGET_TIMEOUT=$(PHASE5B_V3_TIMEOUT) TARGET_SELECTION_POLICY=feedback

phase5b-v3-full:
	$(MAKE) --no-print-directory target-langgraph-shell-react-campaign OUT=$(PHASE5B_V3_OUT)/full CORPUS=$(PHASE5B_V3_CORPUS)/full TARGET_SEEDS=$(PHASE5B_V3_SEEDS) TARGET_PROMPT_PROFILES=baseline ROUNDS=$(PHASE5B_V3_ROUNDS) CANDIDATE_LIMIT=$(PHASE5B_V3_CANDIDATE_LIMIT) TARGET_TIMEOUT=$(PHASE5B_V3_TIMEOUT) TARGET_SELECTION_POLICY=feedback TARGET_AUTO_PIVOT=true TARGET_MIN_COVERAGE_GAIN_SCORE=$(PHASE5B_V3_MIN_COVERAGE_GAIN_SCORE) TARGET_MAX_STAGNANT_ROUNDS=$(PHASE5B_V3_MAX_STAGNANT_ROUNDS)

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

target-maf-github-copilot-shell-check:
	@$(LOAD_DOTENV); target_python="$(MAF_PYTHON)"; test -n "$$target_python" || target_python="$$MAF_PYTHON"; test -n "$$target_python" || target_python="targets/maf_github_copilot_shell/venv/bin/python"; test -x "$$target_python" || target_python="python3"; $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) "$$target_python" targets/maf_github_copilot_shell/run_target.py --check

target-maf-github-copilot-shell:
	$(LOAD_DOTENV); $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_PYTHON_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) $(MAF_ALLOW_UNSUPPORTED_ENV) $(SYNCFUZZ) target run --target maf-github-copilot-shell --task $(TARGET_TASK) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-github-copilot-shell.sh

target-maf-github-copilot-shell-suite:
	$(LOAD_DOTENV); $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_PYTHON_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) $(MAF_ALLOW_UNSUPPORTED_ENV) $(SYNCFUZZ) target suite --target maf-github-copilot-shell --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-github-copilot-shell.sh

target-maf-github-copilot-shell-matrix-suite:
	$(LOAD_DOTENV); $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_PYTHON_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) $(MAF_ALLOW_UNSUPPORTED_ENV) $(SYNCFUZZ) target suite --target maf-github-copilot-shell --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --matrix $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) $(TARGET_SELECTION_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-github-copilot-shell.sh

target-maf-github-copilot-shell-campaign:
	$(LOAD_DOTENV); $(OPENAI_API_KEY_ENV) $(OPENAI_BASE_URL_ENV) $(COPILOT_MODEL_ENV) $(COPILOT_PROVIDER_BASE_URL_ENV) $(COPILOT_PROVIDER_TYPE_ENV) $(COPILOT_PROVIDER_API_KEY_ENV) $(MAF_PYTHON_ENV) $(MAF_TIMEOUT_ENV) $(MAF_COPILOT_CLI_ENV) $(MAF_SESSION_HOME_ENV) $(MAF_LOG_LEVEL_ENV) $(MAF_ALLOW_UNSUPPORTED_ENV) $(SYNCFUZZ) target campaign --target maf-github-copilot-shell --task $(TARGET_TASK) $(TARGET_TASKS_ARGS) $(TARGET_SEED_ARGS) $(TARGET_SEEDS_ARGS) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) $(TARGET_PROMPT_PROFILE_ARGS) $(TARGET_PROMPT_PROFILES_ARGS) --rounds $(ROUNDS) --repeat $(REPEAT) $(FEEDBACK_ARGS) $(CANDIDATE_LIMIT_ARGS) $(TARGET_SELECTION_ARGS) $(TARGET_CAMPAIGN_CONTROL_ARGS) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-github-copilot-shell.sh

target-maf-workflow-checkpoint-check:
	@$(LOAD_DOTENV); target_python="$(MAF_PYTHON)"; test -n "$$target_python" || target_python="$$MAF_PYTHON"; test -n "$$target_python" || target_python="targets/maf_github_copilot_shell/venv/bin/python"; test -x "$$target_python" || target_python="python3"; $(MAF_PYTHON_ENV) $(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) "$$target_python" targets/maf_workflow_checkpoint/run_target.py --check

target-maf-workflow-checkpoint:
	$(LOAD_DOTENV); $(MAF_PYTHON_ENV) $(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) $(SYNCFUZZ) target run --target maf-workflow-checkpoint --task $(MAF_WORKFLOW_TASK) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-workflow-checkpoint.sh

target-maf-workflow-checkpoint-suite:
	$(LOAD_DOTENV); $(MAF_PYTHON_ENV) $(MAF_WORKFLOW_EFFECT_SERVICE_URL_ENV) $(SYNCFUZZ) target suite --target maf-workflow-checkpoint --task $(MAF_WORKFLOW_TASK) $(TARGET_GROUP_ARGS) $(TARGET_GROUPS_ARGS) --repeat $(REPEAT) --corpus $(CORPUS) $(TARGET_RUN_ARGS) --command-file examples/target-commands/maf-workflow-checkpoint.sh

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
