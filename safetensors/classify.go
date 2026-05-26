package safetensors

import (
	"strings"

	"github.com/theapemachine/manifesto/dtype"
	"github.com/theapemachine/manifesto/types"
)

/*
Classify assigns a types.Role to a tensor based on (name, shape, dtype).
Returns types.RoleUnknown when no rule matches — the dispatcher rejects
unknown tensors at wiring time with a clear diagnostic, so this stays a
strict closed-world classifier rather than a heuristic.

The classifier runs in three layers, most specific first:

  1. Suffix-driven rules. Tensor names from PyTorch checkpoints follow
     consistent suffix conventions across HuggingFace model families.
     ".weight" / ".bias" terminate the name; the substring just before
     it carries the semantic role. Most rules are HasSuffix checks
     against literal patterns (e.g. ".attn.to_qkv_mlp_proj.weight").
  2. Generic rank-based fallback. A name we don't recognise but ends in
     ".weight" with rank-2 is almost certainly a Matmul weight; rank-1
     is almost certainly a norm scale. Bias falls through similarly.
  3. RoleUnknown if nothing matched.

Adding a new role pattern is one new entry in suffixRoleRules below —
no other code changes. The unit test enumerates the patterns we
guarantee against the FLUX-2 and Llama-style checkpoints.
*/
func Classify(name string, shape []int64, _ dtype.DType) types.Role {
	if role, ok := matchSuffixRule(name); ok {
		return role
	}

	if strings.HasSuffix(name, ".bias") {
		return classifyBiasByRank(shape)
	}

	if strings.HasSuffix(name, ".weight") {
		return classifyWeightByRank(shape)
	}

	return types.RoleUnknown
}

/*
suffixRule pairs a literal name suffix with the Role it implies.
Order is significant: longer/more specific suffixes come first so a
"to_qkv_mlp_proj.weight" doesn't get swallowed by a generic
"_proj.weight" rule below it.
*/
type suffixRule struct {
	suffix string
	role   types.Role
}

var suffixRoleRules = []suffixRule{
	// FLUX-2 single-stream fused projection. Must come before any
	// generic _proj rule.
	{".attn.to_qkv_mlp_proj.weight", types.RoleAttentionQKVMLP},

	// Attention QKV variants across HuggingFace families.
	{".attn.to_qkv.weight", types.RoleAttentionQKV},
	{".attn.qkv_proj.weight", types.RoleAttentionQKV},
	{".self_attn.qkv_proj.weight", types.RoleAttentionQKV},
	{".attn.Wqkv.weight", types.RoleAttentionQKV},

	// Attention output projection.
	{".attn.to_out.weight", types.RoleAttentionOut},
	{".attn.o_proj.weight", types.RoleAttentionOut},
	{".self_attn.o_proj.weight", types.RoleAttentionOut},
	{".attn.out_proj.weight", types.RoleAttentionOut},

	// QK-Norm scales.
	{".attn.norm_q.weight", types.RoleAttentionQNorm},
	{".attn.norm_k.weight", types.RoleAttentionKNorm},
	{".attn.q_norm.weight", types.RoleAttentionQNorm},
	{".attn.k_norm.weight", types.RoleAttentionKNorm},

	// Modulation projections (AdaLN-style, FLUX double/single stream).
	{"_modulation.linear.weight", types.RoleModulation},
	{"_modulation_img.linear.weight", types.RoleModulation},
	{"_modulation_txt.linear.weight", types.RoleModulation},

	// Final output projection — strict, must end here so we don't
	// catch intermediate linears that happen to contain "proj_out".
	{"proj_out.weight", types.RoleProjectionOut},
	{"lm_head.weight", types.RoleProjectionOut},

	// Token embedding tables (transformer LLMs).
	{"embed_tokens.weight", types.RoleEmbeddingTable},
	{"tok_embeddings.weight", types.RoleEmbeddingTable},
	{"wte.weight", types.RoleEmbeddingTable},
}

func matchSuffixRule(name string) (types.Role, bool) {
	for _, rule := range suffixRoleRules {
		if strings.HasSuffix(name, rule.suffix) {
			return rule.role, true
		}
	}

	return types.RoleUnknown, false
}

/*
classifyWeightByRank applies the generic fallback for ".weight" tensors
the suffix rules did not match. Rank is the strongest signal once the
name doesn't tell us anything specific.
*/
func classifyWeightByRank(shape []int64) types.Role {
	switch len(shape) {
	case 1:
		return types.RoleNormScale
	case 2:
		return types.RoleLinearWeight
	case 3, 4, 5:
		return types.RoleConvKernel
	default:
		return types.RoleUnknown
	}
}

/*
classifyBiasByRank decides whether a ".bias" tensor belongs to a norm
or a linear. Both are rank-1; the distinction is made later by pairing
with the matching ".weight" entry's role at the binding stage.
RoleBias is the safe default that the binding stage refines.
*/
func classifyBiasByRank(shape []int64) types.Role {
	if len(shape) != 1 {
		return types.RoleUnknown
	}

	return types.RoleBias
}
