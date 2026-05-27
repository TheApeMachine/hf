package program

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	hfconfig "github.com/theapemachine/hf/config"
	"github.com/theapemachine/hf/hub"
	"github.com/theapemachine/manifesto/compiler"
	"github.com/theapemachine/manifesto/resolve"
)

/*
IncludeResolver materializes block YAML for `hf://...` include directives
on a manifest program. It composes the HuggingFace Hub client with the
hfconfig architecture-template generator so manifesto can compile any
registered HF architecture without per-model Go.
*/
type IncludeResolver struct {
	hub      resolve.Hub
	cacheDir string
	revision string
	token    string
}

/*
IncludeResolverOptions configures one HF-backed include resolver.
*/
type IncludeResolverOptions struct {
	Hub      resolve.Hub
	CacheDir string
	Revision string
	Token    string
}

/*
NewIncludeResolver constructs a resolver that downloads config.json from the
hub for each include and renders the registered architecture template via
hfconfig.GenerateYAML.
*/
func NewIncludeResolver(options IncludeResolverOptions) (*IncludeResolver, error) {
	if options.Hub == nil {
		return nil, fmt.Errorf("hf include resolver: hub is required")
	}

	return &IncludeResolver{
		hub:      options.Hub,
		cacheDir: options.CacheDir,
		revision: options.Revision,
		token:    options.Token,
	}, nil
}

/*
ResolveInclude implements compiler.IncludeResolver.
Currently supported sources:

  - "hf://<repo-id>"           — download config.json, render the architecture
    template registered in hf/config/registry.go.
  - "hf://<repo-id>#<component>" — same as above, with the component name
    forwarded to the template generator (used by multi-component pipelines
    like diffusion models).
*/
func (resolver *IncludeResolver) ResolveInclude(
	ctx context.Context,
	include compiler.IncludeSource,
) ([]byte, error) {
	repoID, component, ok := compiler.ParseHFReference(include.Source)

	if !ok {
		return nil, fmt.Errorf(
			"hf include resolver: unsupported source %q (expected hf://...)",
			include.Source,
		)
	}

	location := hub.ManifestRepoLocation(repoID, resolver.revision, resolver.token)

	reader, configPath, err := resolver.openConfig(ctx, location, component)

	if err != nil {
		return nil, fmt.Errorf(
			"hf include resolver: open config for %q: %w",
			repoID, err,
		)
	}

	defer reader.Close()

	config, err := hfconfig.ParseConfig(reader)

	if err != nil {
		return nil, fmt.Errorf(
			"hf include resolver: parse %s for %q: %w",
			configPath, repoID, err,
		)
	}

	blockYAML, err := hfconfig.GenerateYAML(config, include.Source)

	if err != nil {
		return nil, fmt.Errorf(
			"hf include resolver: generate YAML for %q: %w",
			repoID, err,
		)
	}

	return []byte(blockYAML), nil
}

var _ compiler.IncludeResolver = (*IncludeResolver)(nil)

func (resolver *IncludeResolver) openConfig(
	ctx context.Context,
	location resolve.RepoLocation,
	component string,
) (io.ReadCloser, string, error) {
	configPaths := configPathCandidates(component)
	openErrors := make([]error, 0, len(configPaths))

	for _, configPath := range configPaths {
		reader, _, err := resolver.hub.Open(ctx, location, configPath, resolver.cacheDir)

		if err == nil {
			return reader, configPath, nil
		}

		openErrors = append(openErrors, fmt.Errorf("%s: %w", configPath, err))
	}

	return nil, "", errors.Join(openErrors...)
}

func configPathCandidates(component string) []string {
	component = strings.TrimSpace(component)

	if isPipelineComponent(component) {
		return []string{path.Join(component, "config.json")}
	}

	return []string{"config.json"}
}

func isPipelineComponent(component string) bool {
	switch component {
	case "text_encoder", "text_encoder_2", "transformer", "vae":
		return true
	}

	return false
}

/*
NewBlockYAMLReader is a small helper that wraps already-resolved block YAML
in a reader, mainly for use in tests.
*/
func NewBlockYAMLReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
