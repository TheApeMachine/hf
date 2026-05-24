# AGENTS.md

This document defines how coding agents work on **hf**. It is a contract, not a style guide.

hf is the **HuggingFace ingestion layer** for the puter platform. The architecture spec lives in `../puter/ARCHITECTURE.md`; the gap inventory in `../puter/GAPS.md`. The shared coding contract lives in `../puter/AGENTS.md`. **Read those three documents before changing anything in this repo.**

This file holds the hf-specific addenda.

---

## 1. What hf is, and is not

hf reads remote HuggingFace repositories (Hub API, `model_index.json`, `config.json`, `*.safetensors`, `tokenizer.json`) and **synthesizes a manifest** that the manifesto compiler consumes. It is a manifest *source*, not a model runtime.

The two entry points the rest of the stack uses:

- `hub.Client` — downloads, snapshots, revision pinning. Caches blobs under `~/.cache/huggingface` with ETag/SHA verification.
- `config.Generator` — reads a model's `config.json` and emits a manifest by composing sub-manifests from `../manifesto/asset/template/model/architecture/`.

Plus:

- `safetensors.Parser` — yields `types.Token{Kind: KindTensor, Name, Shape, Precision, Span}` for the weight binder.
- `tokenizer.*` — loads `tokenizer.json`, exposes Encode/Decode + chat templates.
- `program.Host` — implements `manifesto/runtime.Host` (ReadLine, EmitToken, WriteImage).

hf **is not**:

- A model zoo. There is no `LlamaGenerator`, `FluxGenerator`, `SD3Generator`, `BertGenerator`. There is **one** Generator that composes manifests, parameterized by `config.json` fields. Adding a new architecture means writing a new `template/model/architecture/<name>.yml` in manifesto, not a new Generator in Go.
- A place for "for now" architecture templates. The current Llama-only template in `config/generator.go` is the named anti-pattern (`../puter/GAPS.md §6.5`), and is the symmetric counterpart to `manifesto/diffusion`. Both will be replaced by manifest composition.

---

## 2. The right shape for `config.Generator`

```go
// Pseudocode — the actual implementation is the to-do.
func (generator *Generator) Generate(config Config) (manifest.Program, error) {
    archName := config.Architectures[0]                      // "LlamaForCausalLM", "FluxTransformer2DModel", ...
    templatePath := generator.resolveTemplate(archName)      // → "asset/template/model/architecture/<name>.yml"
    variables := generator.extractVariables(archName, config)  // hidden_size, num_layers, head_dim, ...
    return manifest.Include(templatePath, variables)
}
```

The Go is: map `config.json` schema → variables → include the YAML. The Go is **not**: implement the architecture's forward pass.

If `resolveTemplate` returns "no template registered for X", the answer is to add a `template/model/architecture/X.yml` in manifesto, not to add a `XGenerator` type here.

`scripts/check_banned.sh §1` and `§2` enforce this.

---

## 3. Manifest synthesis is symmetric with hand-authored

The compiler downstream cannot tell whether a manifest was hand-written or synthesized by hf. Both go through the same `parse → expand → lower → unify → optimize → schedule` pipeline. Same closed-world atomic op set, same zero-host-sync rule, same parity tests.

When a missing primitive forces you toward a shortcut here, surface the gap. Adding new primitives is a `puter` change, not a `hf` one.

---

## 4. No Go-heap leakage into device workspace

hf's main I/O paths allocate Go-heap byte buffers for parsing (safetensors archive bytes, JSON, tokenizer files). This is fine. What is **not** fine is passing Go-slice-backed memory directly into a `device.Backend` call.

Weight bytes flow into the workspace via the manifest's static memory plan: the planner allocates a workspace slot, the loader copies bytes from the safetensors mmap into the planner-owned slot at session init, and `device.Backend` calls operate on the planner-owned pointer. The Go slice does not cross into the device dispatch path.

`scripts/check_banned.sh §3` flags suspicious patterns (`unsafe.Pointer(&slice[0])` adjacent to `device.` calls) for review.

---

## 5. Format coverage

The Generator must handle every model architecture that has a registered `template/model/architecture/<name>.yml` in manifesto. As of this writing:

- ✓ `LlamaForCausalLM` (the only one with a Go template today — to be migrated to manifest composition)
- ✗ FLUX-2 (`flux2.yml` exists in old format, needs `kind:` migration + Generator wiring)
- ✗ SD3, SDXL, BERT, Qwen, Gemma, etc.

Adding architectures is the manifest-side path. Adding a new `*Generator` type here is the wrong path.

---

## 6. Banned patterns

In addition to the shared contract in `../puter/AGENTS.md`:

- **Per-architecture `*Generator` types or functions.** Enforced by `scripts/check_banned.sh §1`.
- **Architecture-name string switches** that route to Go (`case "LlamaForCausalLM": llamaForward()`). The switch must resolve to a YAML template path. Enforced by §2.
- **Heuristic `OperationLookup` rules without shape/dtype validation.** Substring matching tensor names is OK as a first cut; silently mapping to the wrong op when the shape doesn't match the spec is not. Add validation when the lookup fires.

---

## 7. Mechanical enforcement

`scripts/check_banned.sh` enforces §1, §2, §4, §6. Run via `make check`. Use `make verify` (check + test) before declaring work done.

If a rule needs to change, update this AGENTS.md *and* update `scripts/check_banned.sh` *and* note the change in the commit message.

---

## 8. Reading order

1. `../puter/ARCHITECTURE.md` — the spec.
2. `../puter/AGENTS.md` — the shared coding contract.
3. `../puter/GAPS.md` — what's done, what's not (§4 covers hf specifically).
4. This document.
5. The package(s) directly relevant to the task.

## 9. Definition of Done

Same as `../puter/AGENTS.md §2`. Paste the output.
