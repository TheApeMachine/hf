package hfconfig

import (
	"strings"
	"testing"
)

func TestGenerateYAML(t *testing.T) {
	config := &Config{
		Architectures:     []string{"LlamaForCausalLM"},
		ModelType:         "llama",
		VocabSize:         128256,
		HiddenSize:        2048,
		IntermediateSize:  8192,
		NumHiddenLayers:   16,
		NumAttentionHeads: 32,
		NumKeyValueHeads:  8,
		RMSNormEps:        1e-5,
		RopeTheta:         500000.0,
	}

	yamlStr, err := GenerateYAML(config, "meta-llama/Llama-3.2-1B-Instruct")
	if err != nil {
		t.Fatalf("GenerateYAML failed: %v", err)
	}

	if !strings.Contains(yamlStr, "op: block.model.llama") {
		t.Errorf("Expected op: block.model.llama, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "source: meta-llama/Llama-3.2-1B-Instruct") {
		t.Errorf("Expected source: meta-llama/Llama-3.2-1B-Instruct, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "repeat: 16") {
		t.Errorf("Expected repeat: 16, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "vocab_size: 128256") {
		t.Errorf("Expected vocab_size: 128256, got:\n%s", yamlStr)
	}
}

func TestGenerateYAMLNoArchitecturesFails(t *testing.T) {
	config := &Config{
		Architectures: nil,
	}

	_, err := GenerateYAML(config, "irrelevant/source")
	if err == nil {
		t.Fatal("expected an error for a config with no architectures")
	}

	if !strings.Contains(err.Error(), "no architectures field") {
		t.Errorf("expected error to mention missing architectures, got: %v", err)
	}
}

func TestGenerateYAMLUnknownArchitectureFails(t *testing.T) {
	config := &Config{
		Architectures: []string{"DefinitelyNotAnArchitectureForCausalLM"},
	}

	_, err := GenerateYAML(config, "irrelevant/source")
	if err == nil {
		t.Fatal("expected an error for an unknown architecture")
	}

	if !strings.Contains(err.Error(), "DefinitelyNotAnArchitectureForCausalLM") {
		t.Errorf("expected error to mention the unknown architecture, got: %v", err)
	}
}
