package hfconfig

import (
	"fmt"
	"sort"
)

/*
The registry maps a HuggingFace architecture class name (the entry that
appears in config.json's "architectures" array, e.g. "LlamaForCausalLM",
"MistralForCausalLM", "BertModel") to the relative path under
manifesto/asset/template/loader/architecture/ that holds the Go-template
YAML for that architecture.

Adding a new HF architecture means writing a new YAML file under
manifesto/asset/template/loader/architecture/ and registering its class
name here. No new Go template strings, no new per-architecture
Generator types. This is the §11.1 manifest-first contract:
hf/config/generator.go composes manifests from external recipes, it
does not inline them.

Multiple class names may map to the same template path when HF labels
several architectures as variants of the same underlying topology
(e.g. Llama and a Llama-derivative both use LlamaForCausalLM.yml).
*/
var architectureRegistry = map[string]string{
	// Llama-family. Many HF models in the wild label themselves
	// LlamaForCausalLM regardless of the specific fine-tune.
	"LlamaForCausalLM": "loader/architecture/LlamaForCausalLM.yml",
}

/*
RegisterArchitecture adds or replaces a registry entry. Intended for
out-of-tree extensions; in-tree architectures should be added to the
literal above so the registry stays grep-able.

assetPath is the path relative to the manifesto asset template root,
e.g. "loader/architecture/MyArchForCausalLM.yml".
*/
func RegisterArchitecture(className, assetPath string) {
	architectureRegistry[className] = assetPath
}

/*
ResolveArchitecture looks up an asset path for a HF class name. Returns
an error listing the supported architectures when the lookup misses,
which is the diagnostic users need to know whether they should:
  - register the architecture themselves, or
  - file an issue for the class name to be added in-tree.
*/
func ResolveArchitecture(className string) (string, error) {
	if assetPath, exists := architectureRegistry[className]; exists {
		return assetPath, nil
	}

	supported := make([]string, 0, len(architectureRegistry))

	for name := range architectureRegistry {
		supported = append(supported, name)
	}

	sort.Strings(supported)

	return "", fmt.Errorf(
		"hfconfig: architecture %q is not registered (supported: %v); "+
			"add a YAML template to manifesto/asset/template/loader/architecture/ "+
			"and register the class name in hf/config/registry.go",
		className,
		supported,
	)
}

/*
SupportedArchitectures returns the sorted list of HF architecture class
names the registry knows about. Useful for diagnostics and for tests
that want to assert coverage.
*/
func SupportedArchitectures() []string {
	names := make([]string, 0, len(architectureRegistry))

	for name := range architectureRegistry {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
