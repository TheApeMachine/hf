package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/theapemachine/manifesto/resolve"
)

/*
ResolveAdapter implements manifesto/resolve.Hub using the caramba Hub client.
*/
type ResolveAdapter struct {
	client *Client
}

/*
NewResolveAdapter constructs a Hub adapter for manifest compilation.
*/
func NewResolveAdapter(client *Client) *ResolveAdapter {
	if client == nil {
		client = NewClient(nil)
	}

	return &ResolveAdapter{client: client}
}

func (adapter *ResolveAdapter) Download(
	ctx context.Context,
	request resolve.DownloadRequest,
) (*resolve.File, error) {
	file, err := adapter.client.Download(ctx, DownloadRequest{
		RepoID:   request.Location.RepoID,
		RepoType: repoTypeFromManifest(request.Location.RepoType),
		Revision: request.Location.Revision,
		Filename: request.Filename,
		CacheDir: request.CacheDir,
		Token:    request.Location.Token,
	})

	if err != nil {
		return nil, err
	}

	return &resolve.File{
		Path:   file.Path,
		Commit: file.Commit,
		Size:   file.Size,
	}, nil
}

func (adapter *ResolveAdapter) ReadJSON(
	ctx context.Context,
	location resolve.RepoLocation,
	filename string,
	cacheDir string,
	target any,
) error {
	file, err := adapter.client.Download(ctx, DownloadRequest{
		RepoID:   location.RepoID,
		RepoType: repoTypeFromManifest(location.RepoType),
		Revision: location.Revision,
		Filename: filename,
		CacheDir: cacheDir,
		Token:    location.Token,
	})

	if err != nil {
		return err
	}

	raw, err := os.ReadFile(file.Path)

	if err != nil {
		return fmt.Errorf("hub read json %q: %w", filename, err)
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("hub read json %q: %w", filename, err)
	}

	return nil
}

func (adapter *ResolveAdapter) Open(
	ctx context.Context,
	location resolve.RepoLocation,
	filename string,
	cacheDir string,
) (io.ReadCloser, *resolve.File, error) {
	file, err := adapter.client.Download(ctx, DownloadRequest{
		RepoID:   location.RepoID,
		RepoType: repoTypeFromManifest(location.RepoType),
		Revision: location.Revision,
		Filename: filename,
		CacheDir: cacheDir,
		Token:    location.Token,
	})

	if err != nil {
		return nil, nil, err
	}

	reader, err := os.Open(file.Path)

	if err != nil {
		return nil, nil, fmt.Errorf("hub open %q: %w", filename, err)
	}

	return reader, &resolve.File{
		Path:   file.Path,
		Commit: file.Commit,
		Size:   file.Size,
	}, nil
}

func (adapter *ResolveAdapter) Glob(
	ctx context.Context,
	location resolve.RepoLocation,
	pattern string,
	cacheDir string,
) ([]string, error) {
	repository, err := adapter.client.Repository(
		ctx,
		repoTypeFromManifest(location.RepoType),
		location.RepoID,
		location.Revision,
		location.Token,
	)

	if err != nil {
		return nil, err
	}

	matches := make([]string, 0)

	for _, sibling := range repository.Siblings {
		if glob(pattern, sibling.Filename) {
			matches = append(matches, sibling.Filename)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("hub glob %q: no matches in %s", pattern, location.RepoID)
	}

	for _, filename := range matches {
		_, err := adapter.client.Download(ctx, DownloadRequest{
			RepoID:   location.RepoID,
			RepoType: repoTypeFromManifest(location.RepoType),
			Revision: location.Revision,
			Filename: filename,
			CacheDir: cacheDir,
			Token:    location.Token,
		})

		if err != nil {
			return nil, err
		}
	}

	return matches, nil
}

func repoTypeFromManifest(repoType resolve.RepoType) RepoType {
	switch repoType {
	case resolve.DatasetRepo:
		return DatasetRepo
	case resolve.SpaceRepo:
		return SpaceRepo
	default:
		return ModelRepo
	}
}

var _ resolve.Hub = (*ResolveAdapter)(nil)

// RepoLocationFromManifest converts manifesto resolve locations for caramba helpers.
func RepoLocationFromManifest(location resolve.RepoLocation) (RepoType, string, string) {
	return repoTypeFromManifest(location.RepoType), location.RepoID, location.Revision
}

// ManifestRepoLocation builds a manifesto resolve location from a Hub repo id.
func ManifestRepoLocation(repoID, revision, token string) resolve.RepoLocation {
	revision = strings.TrimSpace(revision)

	if revision == "" {
		revision = defaultRevision
	}

	return resolve.RepoLocation{
		RepoID:   repoID,
		RepoType: resolve.ModelRepo,
		Revision: revision,
		Token:    token,
	}
}
