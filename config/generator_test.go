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

	if !strings.Contains(err.Error(), "no architecture class") {
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

func TestGenerateYAMLDiffusersClassName(t *testing.T) {
	config, err := ParseConfig(strings.NewReader(`{
		"_class_name": "Flux2Transformer2DModel",
		"attention_head_dim": 128,
		"eps": 0.000001,
		"in_channels": 128,
		"joint_attention_dim": 7680,
		"mlp_ratio": 3.0,
		"num_attention_heads": 24,
		"num_layers": 5,
		"num_single_layers": 20,
		"rope_theta": 2000
	}`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	yamlStr, err := GenerateYAML(config, "hf://black-forest-labs/FLUX.2-klein-4B#transformer")
	if err != nil {
		t.Fatalf("GenerateYAML failed: %v", err)
	}

	if !strings.Contains(yamlStr, "op: block.model.flux2transformer2dmodel") {
		t.Errorf("expected generated block wrapper, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "repeat: 5") {
		t.Errorf("expected num_layers substitution, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "T: 4096") {
		t.Errorf("expected topology sequence binding, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "out_features: 9216") {
		t.Errorf("expected mlp_inner substitution, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "inputs: [hidden_states, encoder_hidden_states, timestep, guidance]") {
		t.Errorf("expected FLUX2 guidance boundary, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "time_guidance_embed.guidance_embedder.linear_1") {
		t.Errorf("expected FLUX2 guidance embedder nodes, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "timestep_divisor: 1") {
		t.Errorf("expected unscaled FLUX2 timestep embedding config, got:\n%s", yamlStr)
	}

	if strings.Contains(yamlStr, "${include.") {
		t.Errorf("expected include variables to be resolved, got:\n%s", yamlStr)
	}
}

func TestGenerateYAMLQwenTextEncoder(t *testing.T) {
	config, err := ParseConfig(strings.NewReader(`{
		"architectures": ["Qwen3ForCausalLM"],
		"head_dim": 128,
		"hidden_size": 2560,
		"intermediate_size": 9728,
		"model_type": "qwen3",
		"num_attention_heads": 32,
		"num_hidden_layers": 36,
		"num_key_value_heads": 8,
		"rms_norm_eps": 0.000001,
		"rope_theta": 1000000,
		"vocab_size": 151936
	}`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	yamlStr, err := GenerateYAML(config, "hf://black-forest-labs/FLUX.2-klein-4B#text_encoder")
	if err != nil {
		t.Fatalf("GenerateYAML failed: %v", err)
	}

	if !strings.Contains(yamlStr, "repeat: \"36\"") {
		t.Errorf("expected layer count substitution, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "out_features: 4096") {
		t.Errorf("expected q projection width substitution, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, "in: [\"h_34\", \"h_35\"]") {
		t.Errorf("expected prompt layer substitution, got:\n%s", yamlStr)
	}

	if strings.Contains(yamlStr, "${include.") {
		t.Errorf("expected include variables to be resolved, got:\n%s", yamlStr)
	}
}

func TestGenerateYAMLAutoencoderBindings(t *testing.T) {
	config, err := ParseConfig(strings.NewReader(`{
		"_class_name": "AutoencoderKLFlux2",
		"in_channels": 3,
		"latent_channels": 32,
		"sample_size": 1024
	}`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	yamlStr, err := GenerateYAML(config, "hf://black-forest-labs/FLUX.2-klein-4B#vae")
	if err != nil {
		t.Fatalf("GenerateYAML failed: %v", err)
	}

	if !strings.Contains(yamlStr, "D: 128") {
		t.Errorf("expected latent token width binding, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, `shape: [1, 32, "128", "128"]`) {
		t.Errorf("expected VAE spatial substitution, got:\n%s", yamlStr)
	}

	if strings.Contains(yamlStr, "${vae_spatial}") {
		t.Errorf("expected VAE spatial variable to be resolved, got:\n%s", yamlStr)
	}

	if !strings.Contains(yamlStr, `shape: [1, "16384", 512]`) {
		t.Errorf("expected VAE mid attention token count substitution, got:\n%s", yamlStr)
	}

	if strings.Contains(yamlStr, "${mid_attn_tokens}") {
		t.Errorf("expected VAE mid attention token variable to be resolved, got:\n%s", yamlStr)
	}
}
