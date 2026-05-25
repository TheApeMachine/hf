package program

import (
	"bytes"
	"context"
	"fmt"

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
	repoID, _, ok := compiler.ParseHFReference(include.Source)

	if !ok {
		return nil, fmt.Errorf(
			"hf include resolver: unsupported source %q (expected hf://...)",
			include.Source,
		)
	}

	location := hub.ManifestRepoLocation(repoID, resolver.revision, resolver.token)

	reader, _, err := resolver.hub.Open(ctx, location, "config.json", resolver.cacheDir)

	if err != nil {
		return nil, fmt.Errorf(
			"hf include resolver: open config.json for %q: %w",
			repoID, err,
		)
	}

	defer reader.Close()

	config, err := hfconfig.ParseConfig(reader)

	if err != nil {
		return nil, fmt.Errorf(
			"hf include resolver: parse config.json for %q: %w",
			repoID, err,
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

/*
NewBlockYAMLReader is a small helper that wraps already-resolved block YAML
in a reader, mainly for use in tests.
*/
func NewBlockYAMLReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
