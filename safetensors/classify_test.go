package safetensors

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/manifesto/dtype"
	"github.com/theapemachine/manifesto/types"
)

/*
TestClassifyFLUX2 enumerates the tensor names from a real FLUX-2
safetensors header (the rows visible in the FLUX.2-Klein-4B archive)
and asserts each gets the role the dispatcher needs to wire it into
the right device.Backend method. If this passes we know the classifier
covers the FLUX-2 surface; if it fails on a row the rule for that row
is the only thing that needs fixing.

Every row is (name, shape, expected_role). Precision is BF16 across
the board in the actual file but Classify does not consult precision
today, so the dtype argument is fixed at BFloat16 for documentation
value only.
*/
func TestClassifyFLUX2(t *testing.T) {
	cases := []struct {
		name  string
		shape []int64
		role  types.Role
	}{
		// Top-level embedders / projections — all rank-2 linears.
		{"context_embedder.weight", []int64{3072, 7680}, types.RoleLinearWeight},
		{"time_guidance_embed.timestep_embedder.linear_1.weight", []int64{3072, 256}, types.RoleLinearWeight},
		{"time_guidance_embed.timestep_embedder.linear_2.weight", []int64{3072, 3072}, types.RoleLinearWeight},
		{"x_embedder.weight", []int64{3072, 128}, types.RoleLinearWeight},

		// Modulation projections — distinguished from generic linears
		// because the dispatcher splits their output into shift/scale/gate
		// triples (AdaLN).
		{"double_stream_modulation_img.linear.weight", []int64{18432, 3072}, types.RoleModulation},
		{"double_stream_modulation_txt.linear.weight", []int64{18432, 3072}, types.RoleModulation},
		{"single_stream_modulation.linear.weight", []int64{9216, 3072}, types.RoleModulation},

		// Final norm-out linear remains a generic Matmul; only proj_out
		// is the model's terminal output projection.
		{"norm_out.linear.weight", []int64{6144, 3072}, types.RoleLinearWeight},
		{"proj_out.weight", []int64{128, 3072}, types.RoleProjectionOut},

		// Per-block attention tensors — sampled at block 0 here; the
		// suffix rules are block-index agnostic so the same role applies
		// to every block.
		{"single_transformer_blocks.0.attn.norm_k.weight", []int64{128}, types.RoleAttentionKNorm},
		{"single_transformer_blocks.0.attn.norm_q.weight", []int64{128}, types.RoleAttentionQNorm},
		{"single_transformer_blocks.0.attn.to_out.weight", []int64{3072, 12288}, types.RoleAttentionOut},
		{"single_transformer_blocks.0.attn.to_qkv_mlp_proj.weight", []int64{27648, 3072}, types.RoleAttentionQKVMLP},

		// Same shapes at a different block index — must classify the same way.
		{"single_transformer_blocks.7.attn.norm_q.weight", []int64{128}, types.RoleAttentionQNorm},
		{"single_transformer_blocks.7.attn.to_qkv_mlp_proj.weight", []int64{27648, 3072}, types.RoleAttentionQKVMLP},
	}

	convey.Convey("Given the FLUX-2 tensor names from the safetensors header", t, func() {
		for _, c := range cases {
			c := c

			convey.Convey("Classify("+c.name+")", func() {
				got := Classify(c.name, c.shape, dtype.BFloat16)
				convey.So(got.String(), convey.ShouldEqual, c.role.String())
			})
		}
	})
}

/*
TestClassifyLlama covers the Llama-family suffix patterns so the same
classifier doubles for the chat.yml path the user is trying to run.
Shapes are taken from Llama-3.2-1B-Instruct (hidden=2048, vocab=128256,
heads=32 q + 8 kv, intermediate=8192).
*/
func TestClassifyLlama(t *testing.T) {
	cases := []struct {
		name  string
		shape []int64
		role  types.Role
	}{
		{"model.embed_tokens.weight", []int64{128256, 2048}, types.RoleEmbeddingTable},
		{"lm_head.weight", []int64{128256, 2048}, types.RoleProjectionOut},
		{"model.norm.weight", []int64{2048}, types.RoleNormScale},

		// Per-layer Llama tensors. The suffix rules cover q/k/v/o split
		// projections plus the MLP triple; norm scales fall through to
		// the rank-1 fallback.
		{"model.layers.0.self_attn.o_proj.weight", []int64{2048, 2048}, types.RoleAttentionOut},
		{"model.layers.0.input_layernorm.weight", []int64{2048}, types.RoleNormScale},
		{"model.layers.0.post_attention_layernorm.weight", []int64{2048}, types.RoleNormScale},
		{"model.layers.0.mlp.gate_proj.weight", []int64{8192, 2048}, types.RoleLinearWeight},
		{"model.layers.0.mlp.up_proj.weight", []int64{8192, 2048}, types.RoleLinearWeight},
		{"model.layers.0.mlp.down_proj.weight", []int64{2048, 8192}, types.RoleLinearWeight},

		// q_proj/k_proj/v_proj are split (not fused) in Llama; for v1
		// they fall through to the generic LinearWeight role. The
		// dispatcher's attention wiring stage refines them by name
		// match — adding dedicated roles is a follow-up if the wiring
		// needs the distinction at classify time.
		{"model.layers.0.self_attn.q_proj.weight", []int64{2048, 2048}, types.RoleLinearWeight},
		{"model.layers.0.self_attn.k_proj.weight", []int64{512, 2048}, types.RoleLinearWeight},
		{"model.layers.0.self_attn.v_proj.weight", []int64{512, 2048}, types.RoleLinearWeight},
	}

	convey.Convey("Given Llama-3.2-1B tensor names", t, func() {
		for _, c := range cases {
			c := c

			convey.Convey("Classify("+c.name+")", func() {
				got := Classify(c.name, c.shape, dtype.BFloat16)
				convey.So(got.String(), convey.ShouldEqual, c.role.String())
			})
		}
	})
}

/*
TestClassifyUnknownNamePassesThroughRankFallback verifies that names
the rule table doesn't recognise still classify reasonably by rank:
rank-2 weight → LinearWeight, rank-1 weight → NormScale, rank-4 →
ConvKernel. This keeps the classifier useful for architectures whose
naming we haven't catalogued yet.
*/
func TestClassifyUnknownNamePassesThroughRankFallback(t *testing.T) {
	convey.Convey("Unknown name with rank-2 weight", t, func() {
		role := Classify("some.brand.new.architecture.layer.weight", []int64{4096, 1024}, dtype.BFloat16)
		convey.So(role, convey.ShouldEqual, types.RoleLinearWeight)
	})

	convey.Convey("Unknown name with rank-1 weight", t, func() {
		role := Classify("some.brand.new.architecture.layer.weight", []int64{4096}, dtype.BFloat16)
		convey.So(role, convey.ShouldEqual, types.RoleNormScale)
	})

	convey.Convey("Unknown name with rank-4 weight", t, func() {
		role := Classify("some.conv.layer.weight", []int64{64, 3, 7, 7}, dtype.BFloat16)
		convey.So(role, convey.ShouldEqual, types.RoleConvKernel)
	})

	convey.Convey("Bias falls through to RoleBias", t, func() {
		role := Classify("model.layers.0.self_attn.q_proj.bias", []int64{2048}, dtype.BFloat16)
		convey.So(role, convey.ShouldEqual, types.RoleBias)
	})

	convey.Convey("Name with no recognised suffix returns Unknown", t, func() {
		role := Classify("random.metadata.entry", nil, dtype.BFloat16)
		convey.So(role, convey.ShouldEqual, types.RoleUnknown)
	})
}
