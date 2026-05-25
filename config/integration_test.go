package hfconfig

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/manifesto/compiler"
	"github.com/theapemachine/manifesto/parse"
)

/*
TestLlamaArchitectureCompilesEndToEnd renders the LlamaForCausalLM template
against a small synthetic config, parses it as a block model, lowers the
topology, and runs the full compiler pipeline (typer + optimizer +
codegen). It's the regression test for `caramba chat` so the user is
never handed a build that compiles in isolation but breaks against a
real HF-loaded model.

The config dimensions mirror the smallest realistic Llama-shaped
checkpoint: 2 layers, 8 heads, 64 head dim, 256 hidden, 32k vocab.
*/
func TestLlamaArchitectureCompilesEndToEnd(t *testing.T) {
	convey.Convey("Given a synthetic Llama-shaped HF config and the LlamaForCausalLM template", t, func() {
		config := &Config{
			Architectures:     []string{"LlamaForCausalLM"},
			ModelType:         "llama",
			VocabSize:         32000,
			HiddenSize:        256,
			IntermediateSize:  512,
			NumHiddenLayers:   2,
			NumAttentionHeads: 8,
			NumKeyValueHeads:  8,
			MaxPositionEmbeds: 4096,
			RMSNormEps:        1e-5,
			RopeTheta:         500000,
			HiddenAct:         "silu",
			TieWordEmbeddings: false,
		}

		blockYAML, err := GenerateYAML(config, "hf://meta-llama/Llama-3.2-1B-Instruct")

		convey.So(err, convey.ShouldBeNil)
		convey.So(len(blockYAML), convey.ShouldBeGreaterThan, 100)

		convey.Convey("Then the rendered block parses as a BlockModel", func() {
			block, err := parse.BlockModelFromYAML([]byte(blockYAML))
			convey.So(err, convey.ShouldBeNil)
			convey.So(block, convey.ShouldNotBeNil)

			topology, err := block.TopologyAST()
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(topology.Nodes), convey.ShouldBeGreaterThan, 0)

			convey.Convey("And LowerTopology + typer accept the full graph without unresolved edge errors", func() {
				lowered, err := compiler.LowerTopology(topology)
				convey.So(err, convey.ShouldBeNil)
				convey.So(len(lowered.AST.Nodes), convey.ShouldBeGreaterThan, 0)

				// The whole pipeline drives off ProgramCompiler.CompileAssets in
				// production. Run the typer + optimizer + codegen explicitly so
				// any failure surfaces with a specific stage error.
				output, err := compileLoweredForTest(blockYAML)

				convey.So(err, convey.ShouldBeNil)
				convey.So(output, convey.ShouldNotBeNil)
				convey.So(output.Graphs["model"], convey.ShouldNotBeNil)
			})
		})
	})
}
