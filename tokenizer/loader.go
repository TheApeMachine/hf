package tokenizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theapemachine/hf/hub"
)

/*
Load resolves, parses, and registers a tokenizer artifact.
*/
func Load(ctx context.Context, source Source) (*Artifact, error) {
	source = source.WithDefaults()

	if artifact, ok := GlobalRegistry().Get(source); ok {
		return artifact, nil
	}

	path, err := source.Resolve(ctx)

	if err != nil {
		return nil, err
	}

	artifact, err := Read(path)

	if err != nil {
		return nil, err
	}

	artifact.Source = source
	artifact.Path = path
	GlobalRegistry().Store(artifact)

	return artifact, nil
}

/*
Resolve returns the local path for a tokenizer source.
*/
func (source Source) Resolve(ctx context.Context) (string, error) {
	source = source.WithDefaults()

	if filepath.IsAbs(source.Source) || strings.HasPrefix(source.Source, "./") {
		return filepath.Join(source.Source, source.File), nil
	}

	if _, err := os.Stat(source.Source); err == nil {
		return filepath.Join(source.Source, source.File), nil
	}

	location, err := hub.ParseLocator(source.Source)

	if err != nil {
		return "", err
	}

	if source.Revision != "" {
		location.Revision = source.Revision
	}

	if source.RepoType != "" {
		repoType, err := parseHubRepoType(source.RepoType)

		if err != nil {
			return "", err
		}

		location.RepoType = repoType
	}

	hubConfig := hub.DefaultHubConfig()

	if source.Cache != "" {
		hubConfig.CacheDir = source.Cache
	}

	file, err := hub.NewClient(hubConfig).Download(
		ctx,
		hub.DownloadRequest{
			RepoID:   location.RepoID,
			RepoType: location.RepoType,
			Revision: location.Revision,
			Filename: source.File,
		},
	)

	if err != nil {
		return "", fmt.Errorf("tokenizer: hub download: %w", err)
	}

	return file.Path, nil
}

func parseHubRepoType(value string) (hub.RepoType, error) {
	switch hub.RepoType(strings.TrimSpace(value)) {
	case "", hub.ModelRepo:
		return hub.ModelRepo, nil
	case hub.DatasetRepo:
		return hub.DatasetRepo, nil
	case hub.SpaceRepo:
		return hub.SpaceRepo, nil
	default:
		return "", fmt.Errorf("tokenizer: unsupported repo_type %q", value)
	}
}
