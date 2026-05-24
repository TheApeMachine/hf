package hfconfig

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/theapemachine/manifesto/asset"
)

/*
GenerateYAML converts a Hugging Face Config into a Manifesto YAML
string by composing the architecture template registered for
config.Architectures[0].

This function intentionally does NOT contain any per-architecture Go
template strings. The template body lives in
manifesto/asset/template/loader/architecture/<ClassName>.yml and is
loaded via asset.ReadFile() at runtime. Adding a new architecture
means:

 1. Write the YAML template at the canonical path.
 2. Register the class name in hf/config/registry.go.

No new Go code per architecture. This is the §11.1 manifest-first
contract for the HF loader; the symmetric anti-pattern (a *Generator
type per model family) is enforced against by hf/scripts/check_banned.sh.
*/
func GenerateYAML(config *Config, source string) (string, error) {
	if len(config.Architectures) == 0 {
		return "", fmt.Errorf("hfconfig: config.json has no architectures field")
	}

	className := config.Architectures[0]

	assetPath, err := ResolveArchitecture(className)

	if err != nil {
		return "", err
	}

	templateBytes, err := asset.ReadFile(assetPath)

	if err != nil {
		return "", fmt.Errorf(
			"hfconfig: cannot read architecture template %q: %w",
			assetPath, err,
		)
	}

	parsedTemplate, err := template.New(className).Parse(string(templateBytes))

	if err != nil {
		return "", fmt.Errorf(
			"hfconfig: cannot parse architecture template %q: %w",
			assetPath, err,
		)
	}

	variables := extractTemplateVariables(config, source)

	var buffer bytes.Buffer

	if err := parsedTemplate.Execute(&buffer, variables); err != nil {
		return "", fmt.Errorf(
			"hfconfig: cannot render architecture template %q: %w",
			assetPath, err,
		)
	}

	return buffer.String(), nil
}

/*
templateVariables is the data passed to the Go template engine. All
fields are populated from the HF Config plus a couple of derived values
that the template needs (HeadDim, KVHiddenSize, IntermediateSizeHalf).

This struct is intentionally a superset of what any single architecture
template needs: each template references only the fields it cares
about, and unreferenced fields are ignored harmlessly. New architecture
templates that need fields not present here can have those fields added
in one place rather than per-architecture.
*/
type templateVariables struct {
	ModelType            string
	ModelName            string
	Source               string
	VocabSize            int
	HiddenSize           int
	NumHiddenLayers      int
	NumAttentionHeads    int
	NumKeyValueHeads     int
	RMSNormEps           float32
	RopeTheta            float32
	HeadDim              int
	KVHiddenSize         int
	IntermediateSize     int
	IntermediateSizeHalf int
	TieWordEmbeddings    bool
}

func extractTemplateVariables(config *Config, source string) templateVariables {
	headDim := 0

	if config.NumAttentionHeads > 0 {
		headDim = config.HiddenSize / config.NumAttentionHeads
	}

	kvHiddenSize := config.NumKeyValueHeads * headDim

	intermediateSizeHalf := 0

	if config.IntermediateSize > 0 {
		intermediateSizeHalf = config.IntermediateSize / 2
	}

	className := ""

	if len(config.Architectures) > 0 {
		className = config.Architectures[0]
	}

	return templateVariables{
		ModelType:            config.ModelType,
		ModelName:            className,
		Source:               source,
		VocabSize:            config.VocabSize,
		HiddenSize:           config.HiddenSize,
		NumHiddenLayers:      config.NumHiddenLayers,
		NumAttentionHeads:    config.NumAttentionHeads,
		NumKeyValueHeads:     config.NumKeyValueHeads,
		RMSNormEps:           config.RMSNormEps,
		RopeTheta:            config.RopeTheta,
		HeadDim:              headDim,
		KVHiddenSize:         kvHiddenSize,
		IntermediateSize:     config.IntermediateSize,
		IntermediateSizeHalf: intermediateSizeHalf,
		TieWordEmbeddings:    config.TieWordEmbeddings,
	}
}
