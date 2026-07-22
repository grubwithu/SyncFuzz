#!/usr/bin/env bash
set -euo pipefail

: "${SYNCFUZZ_CONTRACT_PROPOSAL_REQUEST:?missing proposal request path}"
: "${SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT:?missing proposal output path}"
: "${SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT_SCHEMA:?missing proposal output schema}"
: "${SYNCFUZZ_CONTRACT_PROPOSAL_AUTHORITY:?missing proposal authority}"

test "$SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT_SCHEMA" = "syncfuzz.target-contract-candidates.v1"
test "$SYNCFUZZ_CONTRACT_PROPOSAL_AUTHORITY" = "proposal-only"
test -f "$SYNCFUZZ_CONTRACT_PROPOSAL_REQUEST"

# This deterministic fixture demonstrates the generator boundary without
# calling a model. Replace this command with a reviewed LLM wrapper that reads
# the request JSON and writes a candidate-set JSON to the declared output.
cp target-contract-candidates.example.json "$SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT"
