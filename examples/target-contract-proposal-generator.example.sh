#!/usr/bin/env bash
set -euo pipefail

: "${SYNCFUZZ_CONTRACT_PROPOSAL_REQUEST:?missing proposal request path}"
: "${SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT:?missing proposal output path}"
: "${SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT_SCHEMA:?missing proposal output schema}"
: "${SYNCFUZZ_CONTRACT_PROPOSAL_AUTHORITY:?missing proposal authority}"

test "$SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT_SCHEMA" = "syncfuzz.target-contract-candidates.v1"
test "$SYNCFUZZ_CONTRACT_PROPOSAL_AUTHORITY" = "proposal-only"
test -f "$SYNCFUZZ_CONTRACT_PROPOSAL_REQUEST"

# This deterministic fixture demonstrates the external-generator boundary
# without calling a model. The built-in Go provider is selected with
# `--provider openai-compatible`; an experimental external generator must read
# the request JSON and write a candidate-set JSON to the declared output.
cp target-contract-candidates.example.json "$SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT"
