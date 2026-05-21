package dataset

import (
	"context"
	"fmt"
	"iter"
	"path/filepath"
	"strings"

	"github.com/theapemachine/hf/hub"
)

/*
Load opens a streaming IterableDataset from the Hugging Face Hub without
downloading all files up front.
*/
func Load(
	ctx context.Context,
	client *hub.Client,
	repoID string,
	opts LoadOptions,
) (*IterableDataset, error) {
	if client == nil {
		client = hub.NewClient(nil)
	}

	opts = normalizeLoadOptions(opts)

	token := opts.Token

	if token == "" {
		token = client.HubToken()
	}

	server := newDatasetServer()
	configNames, err := server.configNames(ctx, repoID, token)

	if err != nil {
		return nil, err
	}

	config, err := normalizeConfig(opts.Config, configNames)

	if err != nil {
		return nil, err
	}

	shards, err := resolveHubShards(ctx, client, server, repoID, config, opts, token)

	if err != nil {
		return nil, err
	}

	if len(shards) == 0 {
		return nil, fmt.Errorf("dataset: no streaming files found for %s config=%s split=%s", repoID, config, opts.Split)
	}

	features, err := server.features(ctx, repoID, config, token)

	if err != nil {
		features = inferFeatures(shards)
	}

	if len(opts.Columns) > 0 {
		features.Columns = append([]string(nil), opts.Columns...)
	}

	return &IterableDataset{
		shards:   shards,
		features: features,
	}, nil
}

/*
LoadFiles opens a streaming IterableDataset from local files.
*/
func LoadFiles(
	_ context.Context,
	format string,
	files map[string][]string,
	split string,
) (*IterableDataset, error) {
	paths, ok := files[split]

	if !ok {
		return nil, fmt.Errorf("dataset: split %q not found in data files", split)
	}

	shards := make([]shard, 0, len(paths))

	for _, path := range paths {
		kind, err := formatFromName(format, path)

		if err != nil {
			return nil, err
		}

		shards = append(shards, shard{
			path:   path,
			format: kind,
		})
	}

	return &IterableDataset{
		shards:   shards,
		features: inferFeatures(shards),
	}, nil
}

func normalizeLoadOptions(opts LoadOptions) LoadOptions {
	if strings.TrimSpace(opts.Config) == "" {
		opts.Config = defaultConfig
	}

	if strings.TrimSpace(opts.Split) == "" {
		opts.Split = defaultSplit
	}

	if strings.TrimSpace(opts.Revision) == "" {
		opts.Revision = defaultRevision
	}

	return opts
}

func resolveHubShards(
	ctx context.Context,
	client *hub.Client,
	server *datasetServer,
	repoID string,
	config string,
	opts LoadOptions,
	token string,
) ([]shard, error) {
	if len(opts.DataFiles) > 0 {
		return resolveDataFileShards(ctx, client, repoID, opts, token)
	}

	parquetFiles, err := server.parquetFiles(ctx, repoID, config, opts.Split, token)

	if err != nil {
		return nil, err
	}

	if len(parquetFiles) > 0 {
		shards := make([]shard, 0, len(parquetFiles))

		for _, entry := range parquetFiles {
			shards = append(shards, shard{
				url:     entry.URL,
				format:  formatParquet,
				size:    entry.Size,
				token:   token,
				columns: append([]string(nil), opts.Columns...),
			})
		}

		return shards, nil
	}

	return resolveRepositoryShards(ctx, client, repoID, opts, token)
}

func resolveDataFileShards(
	ctx context.Context,
	client *hub.Client,
	repoID string,
	opts LoadOptions,
	token string,
) ([]shard, error) {
	patterns, ok := opts.DataFiles[opts.Split]

	if !ok {
		return nil, fmt.Errorf("dataset: split %q not found in data_files", opts.Split)
	}

	repository, err := client.Repository(ctx, hub.DatasetRepo, repoID, opts.Revision, token)

	if err != nil {
		return nil, err
	}

	matches := repository.Matching(patterns, nil)
	shards := make([]shard, 0, len(matches))

	for _, sibling := range matches {
		kind := detectFormat(sibling.Filename)

		if kind == formatUnknown {
			continue
		}

		url, err := hub.ResolveURL(
			client.Endpoint(),
			hub.DatasetRepo,
			repoID,
			opts.Revision,
			sibling.Filename,
		)

		if err != nil {
			return nil, err
		}

		shards = append(shards, shard{
			url:     url,
			format:  kind,
			size:    sibling.Size,
			token:   token,
			columns: append([]string(nil), opts.Columns...),
		})
	}

	_ = ctx

	return shards, nil
}

func resolveRepositoryShards(
	ctx context.Context,
	client *hub.Client,
	repoID string,
	opts LoadOptions,
	token string,
) ([]shard, error) {
	repository, err := client.Repository(ctx, hub.DatasetRepo, repoID, opts.Revision, token)

	if err != nil {
		return nil, err
	}

	patterns := splitFilePatterns(opts.Split)
	matches := repository.Matching(patterns, nil)
	shards := make([]shard, 0, len(matches))

	for _, sibling := range matches {
		kind := detectFormat(sibling.Filename)

		if kind == formatUnknown {
			continue
		}

		url, err := hub.ResolveURL(
			client.Endpoint(),
			hub.DatasetRepo,
			repoID,
			opts.Revision,
			sibling.Filename,
		)

		if err != nil {
			return nil, err
		}

		shards = append(shards, shard{
			url:     url,
			format:  kind,
			size:    sibling.Size,
			token:   token,
			columns: append([]string(nil), opts.Columns...),
		})
	}

	return shards, nil
}

func formatFromName(format string, path string) (fileFormat, error) {
	if strings.TrimSpace(format) != "" {
		switch strings.ToLower(format) {
		case "json", "jsonl":
			return formatJSONL, nil
		case "csv":
			return formatCSV, nil
		case "parquet":
			return formatParquet, nil
		default:
			return formatUnknown, fmt.Errorf("dataset: unsupported format %q", format)
		}
	}

	kind := detectFormat(path)

	if kind == formatUnknown {
		return formatUnknown, fmt.Errorf("dataset: unsupported file %q", path)
	}

	return kind, nil
}

func inferFeatures(shards []shard) Features {
	if len(shards) == 0 {
		return Features{}
	}

	name := shards[0].path

	if name == "" {
		name = shards[0].url
	}

	return Features{Columns: []string{filepath.Ext(name)}}
}

func (dataset *IterableDataset) rowStream(ctx context.Context) iter.Seq2[Row, error] {
	return func(yield func(Row, error) bool) {
		shards := dataset.activeShards()
		opener := newStreamOpener()
		seen := dataset.skip
		emitted := 0

		for _, shardSource := range shards {
			rows, err := openShardRows(ctx, opener, shardSource)

			if err != nil {
				yield(nil, err)
				return
			}

			for row, err := range rows {
				if err != nil {
					yield(nil, err)
					return
				}

				if seen > 0 {
					seen--
					continue
				}

				if dataset.take > 0 {
					if emitted >= dataset.take {
						return
					}

					emitted++
				}

				if !yield(row, nil) {
					return
				}
			}
		}
	}
}

func (dataset *IterableDataset) activeShards() []shard {
	shards := append([]shard(nil), dataset.shards...)

	if dataset.numShards > 1 {
		filtered := make([]shard, 0)

		for index, shardSource := range shards {
			if index%dataset.numShards == dataset.shardIndex {
				filtered = append(filtered, shardSource)
			}
		}

		shards = filtered
	}

	if dataset.shuffle != nil && dataset.shuffle.shuffleShards {
		shards = shuffleStringsShards(shards, dataset.seed+int64(dataset.epoch))
	}

	return shards
}

func openShardRows(
	ctx context.Context,
	opener *streamOpener,
	shardSource shard,
) (iter.Seq2[Row, error], error) {
	switch shardSource.format {
	case formatJSONL:
		return openJSONLShard(ctx, opener, shardSource)
	case formatCSV:
		return openCSVShard(ctx, opener, shardSource)
	case formatParquet:
		return openParquetShard(ctx, opener, shardSource)
	default:
		return nil, fmt.Errorf("dataset: unsupported shard format")
	}
}
