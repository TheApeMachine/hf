#!/usr/bin/env bash
# scripts/check_banned.sh — mechanical enforcement for hf.
#
# hf is the HuggingFace ingestion layer. Its job is to read remote
# repositories (config.json, model_index.json, *.safetensors) and emit a
# manifest that the manifesto compiler consumes. Same closed-world rule
# as everywhere else: no per-architecture Go templates. The generator
# composes manifests by including sub-manifests from
# manifesto/asset/template/model/architecture/, varied by config fields.
#
# Exits 0 if clean, 1 if any violation found.

set -u
cd "$(git rev-parse --show-toplevel 2>/dev/null || dirname "$0"/..)" || exit 2

violations=0
fail() { printf '  %s\n' "$1" >&2; violations=$((violations + 1)); }
section() { printf '\n=== %s ===\n' "$1"; }

# -----------------------------------------------------------------------------
# 1. No per-architecture Generator types or functions.
# A type or function named LlamaGenerator, FluxGenerator, SD3Generator,
# BertGenerator is the named anti-pattern (GAPS.md §6.5). The loader has
# one Generator that composes manifests from asset templates.
# -----------------------------------------------------------------------------
section "1. Per-architecture Generator types or functions"

generator_pattern='(Llama|Flux|SD3|SDXL|Bert|GPT|Mistral|Qwen|Gemma|Dit|UNet|VAE|StableDiffusion)Generator'
while IFS= read -r line; do
    fail "per-architecture Generator: $line"
done < <(grep -rnE --include='*.go' --exclude-dir=vendor --exclude-dir=.git \
    "(type|func)\s+$generator_pattern" . 2>/dev/null || true)

# -----------------------------------------------------------------------------
# 2. No architecture-name string-switch dispatch.
# A switch on architecture name that routes to model-specific Go is the
# same anti-pattern as a per-model Generator type, just spelled
# differently. The dispatch should route to a YAML template path.
# -----------------------------------------------------------------------------
section "2. Architecture-name string-switch routing to Go"

# Heuristic: a case statement matching an architecture class name that
# does NOT immediately resolve to a string (template path or recipe
# identifier). We grep cases and let the reviewer judge.
while IFS= read -r line; do
    # Skip lines that look like they resolve to a string literal —
    # likely a template path — and only flag cases that lead into Go.
    if printf '%s' "$line" | grep -qE '"[a-zA-Z0-9_./-]+\.(yml|yaml)"'; then
        continue
    fi
    fail "architecture-name case (route to YAML template instead): $line"
done < <(grep -rnE --include='*.go' --exclude-dir=vendor --exclude-dir=.git \
    'case\s+"(LlamaForCausalLM|BertModel|FluxTransformer2DModel|StableDiffusion3Pipeline|StableDiffusionPipeline|SDXLPipeline|MistralForCausalLM|Qwen2ForCausalLM|GemmaForCausalLM)' \
    . 2>/dev/null || true)

# -----------------------------------------------------------------------------
# 3. No Go-heap allocation paths into device workspace.
# hf reads bytes into Go memory (acceptable for parsing). It must not pass
# Go-slice-backed memory into device.Backend execution paths. The device
# workspace is allocated off-heap (puter/AGENTS.md §5.2). Loose check:
# flag obvious patterns where hf hands a `[]byte` slice's backing array
# directly to a device call.
# -----------------------------------------------------------------------------
section "3. Go-heap workspace leakage (heuristic)"

while IFS= read -r line; do
    fail "potential Go-heap → device hand-off (review): $line"
done < <(grep -rnE --include='*.go' --exclude-dir=vendor --exclude-dir=.git \
    'unsafe\.Pointer\(&[a-zA-Z_][a-zA-Z0-9_]*\[0\]\).*(device\.|backend\.|Backend\.|cudaMemcpy|MTLBuffer)' \
    . 2>/dev/null || true)

# -----------------------------------------------------------------------------
# 4. Banned phrases (mirror puter/AGENTS.md §1)
# -----------------------------------------------------------------------------
section "4. Banned phrases"

phrases='for now|approximation acceptable|required vs optional backend|fallback to Go|TODO.*later|will implement.*later|placeholder.*until'
while IFS= read -r line; do
    fail "banned phrase: $line"
done < <(grep -rniE --include='*.go' --exclude-dir=vendor --exclude-dir=.git \
    "(//|/\\*).*($phrases)" . 2>/dev/null || true)

# -----------------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------------
printf '\n'
if [ "$violations" -gt 0 ]; then
    printf 'FAILED: %d banned-pattern violation(s)\n' "$violations" >&2
    printf 'See puter/AGENTS.md and puter/GAPS.md §6.5 for the manifest-first rule.\n' >&2
    exit 1
fi
printf 'OK: no banned-pattern violations\n'
