package hfconfig

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/theapemachine/manifesto/asset"
	"github.com/theapemachine/manifesto/ast"
	"gopkg.in/yaml.v3"
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
	className, err := config.ArchitectureName()

	if err != nil {
		return "", err
	}

	_, component := parseSourceComponent(source)
	assetPath, err := ResolveArchitecturePath(className, component)

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

	manifestTemplate, includeVars, err := renderManifestVariables(
		string(templateBytes),
		config,
		className,
	)

	if err != nil {
		return "", err
	}

	parsedTemplate, err := template.New(className).Parse(manifestTemplate)

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

	rendered := buffer.String()

	if strings.HasPrefix(assetPath, "model/architecture/") {
		return wrapTopologyYAML(assetPath, rendered, className, includeVars)
	}

	return injectBlockTopologyBindings(rendered, includeVars)
}

func parseSourceComponent(source string) (repoID, component string) {
	hashIndex := strings.LastIndex(source, "#")

	if hashIndex < 0 {
		return source, ""
	}

	return source[:hashIndex], source[hashIndex+1:]
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
	// TodayDate is the current calendar date in Llama-3 prompt style (e.g. "26 Jul 2024").
	TodayDate string
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

	if resolved, err := config.ArchitectureName(); err == nil {
		className = resolved
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
		TodayDate:            time.Now().Format("2 Jan 2006"),
	}
}

func renderManifestVariables(
	templateBody string,
	config *Config,
	className string,
) (string, map[string]any, error) {
	variables, err := includeVariables(config, className)

	if err != nil {
		return "", nil, err
	}

	pairs := make([]string, 0, len(variables)*4)
	names := make([]string, 0, len(variables))

	for name := range variables {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		literal := formatIncludeScalar(variables[name])
		pairs = append(pairs, "${include."+name+"}", literal)
		pairs = append(pairs, "${"+name+"}", literal)
	}

	rendered := strings.NewReplacer(pairs...).Replace(templateBody)

	if strings.Contains(rendered, "${include.") {
		return "", nil, fmt.Errorf("hfconfig: unresolved include variable in %q", className)
	}

	return rendered, variables, nil
}

func includeVariables(config *Config, className string) (map[string]any, error) {
	variables := make(map[string]any)

	for name, value := range config.Raw {
		variables[name] = value
	}

	seedDerivedVariables(variables, config)

	for name, binding := range ArchitectureVariables(className) {
		value, err := evaluateBinding(binding, variables)

		if err != nil {
			return nil, fmt.Errorf("hfconfig: resolve %s.%s: %w", className, name, err)
		}

		variables[name] = value
	}

	seedDerivedVariables(variables, config)

	return variables, nil
}

func seedDerivedVariables(variables map[string]any, config *Config) {
	setIfMissing(variables, "vocab_size", config.VocabSize)
	setIfMissing(variables, "hidden_size", config.HiddenSize)
	setIfMissing(variables, "intermediate_size", config.IntermediateSize)
	setIfMissing(variables, "num_hidden_layers", config.NumHiddenLayers)
	setIfMissing(variables, "num_attention_heads", config.NumAttentionHeads)
	setIfMissing(variables, "num_key_value_heads", config.NumKeyValueHeads)
	setIfMissing(variables, "rope_theta", config.RopeTheta)
	setIfMissing(variables, "context_seq_len", 1024)
	setIfMissing(variables, "latent_seq_len", 4096)

	if _, exists := variables["eps"]; !exists {
		setEpsVariable(variables, config)
	}

	headDim := firstNumber(
		variables["head_dim"],
		variables["attention_head_dim"],
		quotient(config.HiddenSize, config.NumAttentionHeads),
	)
	setIfMissing(variables, "head_dim", headDim)
	setIfMissing(variables, "attention_head_dim", headDim)

	hiddenSize := firstNumber(
		variables["hidden_size"],
		productOfVariables(variables, "num_attention_heads", "attention_head_dim"),
	)
	setIfMissing(variables, "hidden_size", hiddenSize)

	keyValueHeads := firstNumber(
		variables["num_key_value_heads"],
		variables["num_attention_heads"],
	)
	setIfMissing(variables, "num_key_value_heads", keyValueHeads)

	setIfMissing(variables, "q_proj_out", productOfVariables(
		variables, "num_attention_heads", "head_dim",
	))
	setIfMissing(variables, "kv_proj_out", productOfVariables(
		variables, "num_key_value_heads", "head_dim",
	))
	setIfMissing(variables, "double_hidden_size", productOfVariables(
		variables, "hidden_size", literalNumber(2),
	))
	setIfMissing(variables, "triple_hidden_size", productOfVariables(
		variables, "hidden_size", literalNumber(3),
	))
	setIfMissing(variables, "mlp_inner", productOfVariables(
		variables, "hidden_size", "mlp_ratio",
	))
	setIfMissing(variables, "mlp_packed", productOfVariables(
		variables, "mlp_inner", literalNumber(2),
	))
	setIfMissing(variables, "joint_terminal_index", sumOfVariables(
		variables, "num_layers", "num_single_layers",
	))

	hiddenLayers := intVariable(variables, "num_hidden_layers")
	setIfMissing(variables, "prompt_layer_a", maxInt(hiddenLayers-2, 0))
	setIfMissing(variables, "prompt_layer_b", maxInt(hiddenLayers-1, 0))
	setIfMissing(variables, "prompt_layer_c", hiddenLayers)

	latentSide := firstNumber(
		variables["latent_side"],
		squareSide(variables["latent_seq_len"]),
		literalNumber(64),
	)
	setIfMissing(variables, "latent_side", latentSide)
	setIfMissing(variables, "packed_side", latentSide)
	setIfMissing(variables, "latent_token_dim", 128)
	setIfMissing(variables, "vae_spatial", vaeSpatial(variables))
	setIfMissing(variables, "mid_attn_tokens", productOfVariables(
		variables, "vae_spatial", "vae_spatial",
	))
}

func setEpsVariable(variables map[string]any, config *Config) {
	if value, exists := variables["eps"]; exists && value != nil {
		variables["eps"] = value
		return
	}

	if value, exists := variables["rms_norm_eps"]; exists && value != nil {
		variables["eps"] = value
		return
	}

	if config.RMSNormEps != 0 {
		variables["eps"] = config.RMSNormEps
		return
	}

	if config.LayerNormEps != 0 {
		variables["eps"] = config.LayerNormEps
	}
}

func evaluateBinding(binding ast.Binding, variables map[string]any) (any, error) {
	if binding.Config != "" {
		value, exists := variables[binding.Config]

		if !exists {
			return nil, fmt.Errorf("config field %q is missing", binding.Config)
		}

		return value, nil
	}

	if len(binding.Product) > 0 {
		return evaluateProduct(binding.Product, variables)
	}

	if len(binding.Sum) > 0 {
		return evaluateSum(binding.Sum, variables)
	}

	if binding.Literal != nil {
		return binding.Literal, nil
	}

	return nil, fmt.Errorf("empty binding")
}

func evaluateProduct(bindings []ast.Binding, variables map[string]any) (any, error) {
	total := 1.0

	for _, binding := range bindings {
		value, err := evaluateBinding(binding, variables)

		if err != nil {
			return nil, err
		}

		number, ok := numberValue(value)

		if !ok {
			return nil, fmt.Errorf("non-numeric product value %v", value)
		}

		total *= number
	}

	return compactNumber(total), nil
}

func evaluateSum(bindings []ast.Binding, variables map[string]any) (any, error) {
	total := 0.0

	for _, binding := range bindings {
		value, err := evaluateBinding(binding, variables)

		if err != nil {
			return nil, err
		}

		number, ok := numberValue(value)

		if !ok {
			return nil, fmt.Errorf("non-numeric sum value %v", value)
		}

		total += number
	}

	return compactNumber(total), nil
}

func wrapTopologyYAML(
	assetPath string,
	rendered string,
	className string,
	variables map[string]any,
) (string, error) {
	if !strings.HasPrefix(assetPath, "model/architecture/") {
		return rendered, nil
	}

	outputNames, err := topologyOutputNames(rendered)

	if err != nil {
		return "", err
	}

	var buffer strings.Builder

	fmt.Fprintf(&buffer, "kind: Block\n")
	fmt.Fprintf(&buffer, "category: model\n")
	fmt.Fprintf(&buffer, "op: block.model.%s\n", strings.ToLower(className))
	fmt.Fprintf(&buffer, "name: %s\n", className)
	fmt.Fprintf(&buffer, "label: %s\n", className)
	fmt.Fprintf(&buffer, "outputs:\n")

	for _, name := range outputNames {
		fmt.Fprintf(&buffer, "  - name: %s\n", name)
	}

	fmt.Fprintf(&buffer, "system:\n")
	fmt.Fprintf(&buffer, "  topology:\n")
	writeTopologyBindings(&buffer, rendered, variables)
	buffer.WriteString(indentYAML(rendered, 4))

	return buffer.String(), nil
}

func injectBlockTopologyBindings(rendered string, variables map[string]any) (string, error) {
	bindings := topologyBindings(rendered, variables)

	if len(bindings) == 0 {
		return rendered, nil
	}

	marker := "  topology:\n"
	index := strings.Index(rendered, marker)

	if index < 0 {
		return "", fmt.Errorf("hfconfig: generated block has topology bindings but no topology section")
	}

	var buffer strings.Builder

	buffer.WriteString(rendered[:index])
	buffer.WriteString(marker)
	writeBindingMap(&buffer, bindings, 4)
	buffer.WriteString(rendered[index+len(marker):])

	return buffer.String(), nil
}

func writeTopologyBindings(
	buffer *strings.Builder,
	rendered string,
	variables map[string]any,
) {
	bindings := topologyBindings(rendered, variables)

	if len(bindings) == 0 {
		return
	}

	writeBindingMap(buffer, bindings, 4)
}

func writeBindingMap(buffer *strings.Builder, bindings map[string]int64, spaces int) {
	prefix := strings.Repeat(" ", spaces)

	keys := make([]string, 0, len(bindings))

	for key := range bindings {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	fmt.Fprintf(buffer, "%sbindings:\n", prefix)

	for _, key := range keys {
		fmt.Fprintf(buffer, "%s  %s: %d\n", prefix, key, bindings[key])
	}
}

func topologyBindings(rendered string, variables map[string]any) map[string]int64 {
	bindings := make(map[string]int64)
	inputs := topologyInputSet(rendered)

	if inputs["hidden_states"] || inputs["latents"] || inputs["timestep"] {
		setBinding(bindings, "B", firstNumber(variables["batch_size"], literalNumber(1)))
	}

	if inputs["hidden_states"] || inputs["latents"] {
		setBinding(bindings, "T", variables["latent_seq_len"])
	}

	if inputs["hidden_states"] {
		setBinding(bindings, "D", firstNumber(
			variables["in_channels"],
			variables["latent_token_dim"],
			literalNumber(128),
		))
	}

	if inputs["latents"] {
		setBinding(bindings, "D", firstNumber(
			variables["latent_token_dim"],
			literalNumber(128),
		))
	}

	if inputs["encoder_hidden_states"] || inputs["text_embedding"] {
		setBinding(bindings, "C", variables["context_seq_len"])
		setBinding(bindings, "E", variables["joint_attention_dim"])
	}

	return bindings
}

func topologyInputSet(rendered string) map[string]bool {
	document := struct {
		Inputs []any `yaml:"inputs"`
		System struct {
			Topology struct {
				Inputs []any `yaml:"inputs"`
			} `yaml:"topology"`
		} `yaml:"system"`
	}{}

	inputs := make(map[string]bool)

	if err := yaml.Unmarshal([]byte(rendered), &document); err != nil {
		return inputs
	}

	collectTopologyInputs(inputs, document.Inputs)
	collectTopologyInputs(inputs, document.System.Topology.Inputs)

	return inputs
}

func collectTopologyInputs(inputs map[string]bool, rawInputs []any) {
	for _, rawInput := range rawInputs {
		switch typed := rawInput.(type) {
		case string:
			inputs[typed] = true
		case map[string]any:
			if name, ok := typed["name"].(string); ok && name != "" {
				inputs[name] = true
			}
		}
	}
}

func setBinding(bindings map[string]int64, symbol string, value any) {
	number, ok := numberValue(value)

	if !ok || number == 0 {
		return
	}

	bindings[symbol] = int64(number)
}

func topologyOutputNames(rendered string) ([]string, error) {
	topology := &ast.Topology{}

	if err := yaml.Unmarshal([]byte(rendered), topology); err != nil {
		return nil, fmt.Errorf("hfconfig: parse generated topology outputs: %w", err)
	}

	if len(topology.Outputs) > 0 {
		names := make([]string, 0, len(topology.Outputs))

		for name := range topology.Outputs {
			names = append(names, name)
		}

		sort.Strings(names)

		return names, nil
	}

	if len(topology.Nodes) == 0 {
		return nil, fmt.Errorf("hfconfig: generated topology has no nodes")
	}

	finalNode := topology.Nodes[len(topology.Nodes)-1]

	if len(finalNode.Out) == 0 {
		return nil, fmt.Errorf("hfconfig: generated topology final node has no output")
	}

	return []string{finalNode.Out[0]}, nil
}

func indentYAML(rendered string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(rendered, "\n")

	for lineIndex, line := range lines {
		if line == "" {
			continue
		}

		lines[lineIndex] = prefix + line
	}

	return strings.Join(lines, "\n")
}

func setIfMissing(variables map[string]any, name string, value any) {
	if _, exists := variables[name]; exists {
		return
	}

	if value == nil {
		return
	}

	if number, ok := numberValue(value); ok && number == 0 {
		return
	}

	variables[name] = value
}

func productOfVariables(variables map[string]any, left any, right any) any {
	leftNumber, leftOK := variableNumber(variables, left)
	rightNumber, rightOK := variableNumber(variables, right)

	if !leftOK || !rightOK {
		return nil
	}

	return compactNumber(leftNumber * rightNumber)
}

func sumOfVariables(variables map[string]any, left string, right string) any {
	leftNumber, leftOK := variableNumber(variables, left)
	rightNumber, rightOK := variableNumber(variables, right)

	if !leftOK || !rightOK {
		return nil
	}

	return compactNumber(leftNumber + rightNumber)
}

func variableNumber(variables map[string]any, name any) (float64, bool) {
	switch typed := name.(type) {
	case string:
		return numberValue(variables[typed])
	default:
		return numberValue(typed)
	}
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func intVariable(variables map[string]any, name string) int {
	number, ok := numberValue(variables[name])

	if !ok {
		return 0
	}

	return int(number)
}

func firstNumber(values ...any) any {
	for _, value := range values {
		number, ok := numberValue(value)

		if !ok || number == 0 {
			continue
		}

		return compactNumber(number)
	}

	return nil
}

func quotient(numerator int, denominator int) any {
	if numerator == 0 || denominator == 0 {
		return nil
	}

	return compactNumber(float64(numerator) / float64(denominator))
}

func squareSide(value any) any {
	number, ok := numberValue(value)

	if !ok || number <= 0 {
		return nil
	}

	side := math.Sqrt(number)

	if math.Round(side) != side {
		return nil
	}

	return compactNumber(side)
}

func vaeSpatial(variables map[string]any) any {
	if sampleSize, ok := numberValue(variables["sample_size"]); ok && sampleSize > 0 {
		return compactNumber(sampleSize / 8)
	}

	if packedSide, ok := numberValue(variables["packed_side"]); ok && packedSide > 0 {
		return compactNumber(packedSide * 2)
	}

	return nil
}

func literalNumber(value float64) any {
	return compactNumber(value)
}

func compactNumber(value float64) any {
	rounded := math.Round(value)

	if math.Abs(value-rounded) < 1e-9 {
		return int64(rounded)
	}

	return value
}

func formatIncludeScalar(value any) string {
	switch typed := value.(type) {
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}

	return right
}
