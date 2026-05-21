package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/qpool"
)

const hubDownloadJobTimeout = 24 * time.Hour

type remoteMetadata struct {
	ETag    string
	XetHash string
	Size    int64
}

/*
Download downloads one file and returns its immutable snapshot path.
*/
func (client *Client) Download(
	ctx context.Context, request DownloadRequest,
) (*File, error) {
	request = client.normalizeDownloadRequest(request)

	if err := validateDownloadRequest(request); err != nil {
		return nil, err
	}

	paths := newCachePaths(request.CacheDir, request.RepoType, request.RepoID)
	publishHubProgress(
		"download.resolve",
		fmt.Sprintf("resolving Hub file %s", request.Filename),
		qpool.Field{Key: "repo", Value: request.RepoID},
		qpool.Field{Key: "revision", Value: request.Revision},
		qpool.Field{Key: "file", Value: request.Filename},
	)

	if client.config.Offline {
		return offlineFile(paths, request)
	}

	if file, ok := cachedSnapshotFile(paths, request); ok {
		publishHubProgress(
			"download.cached",
			fmt.Sprintf("using cached Hub snapshot file %s", request.Filename),
			qpool.Field{Key: "repo", Value: request.RepoID},
			qpool.Field{Key: "commit", Value: file.Commit},
			qpool.Field{Key: "file", Value: request.Filename},
			qpool.Field{Key: "path", Value: file.Path},
		)

		return file, nil
	}

	publishHubProgress(
		"repository.resolve",
		"resolving Hub repository metadata",
		qpool.Field{Key: "repo", Value: request.RepoID},
		qpool.Field{Key: "revision", Value: request.Revision},
	)

	repository, err := client.Repository(
		ctx,
		request.RepoType,
		request.RepoID,
		request.Revision,
		request.Token,
	)

	if err != nil {
		return nil, err
	}

	publishHubProgress(
		"repository.resolved",
		"resolved Hub repository metadata",
		qpool.Field{Key: "repo", Value: request.RepoID},
		qpool.Field{Key: "commit", Value: repository.Commit},
	)

	return client.downloadWithRepository(ctx, request, repository, paths)
}

func (client *Client) downloadWithRepository(
	ctx context.Context,
	request DownloadRequest,
	repository *Repository,
	paths cachePaths,
) (*File, error) {
	if err := cacheRepositoryRevision(paths, request.Revision, repository); err != nil {
		return nil, err
	}

	return client.downloadPreparedFile(ctx, request, repository, paths)
}

func (client *Client) downloadPreparedFile(
	ctx context.Context,
	request DownloadRequest,
	repository *Repository,
	paths cachePaths,
) (*File, error) {
	if err := paths.ensure(); err != nil {
		return nil, err
	}

	snapshotPath := paths.snapshotFile(repository.Commit, request.Filename)
	file := &File{
		RepoID:        request.RepoID,
		RepoType:      request.RepoType,
		Revision:      request.Revision,
		Commit:        repository.Commit,
		Filename:      request.Filename,
		Path:          snapshotPath,
		WouldDownload: true,
	}

	if !request.Force {
		if info, err := os.Stat(snapshotPath); err == nil {
			file.Size = info.Size()
			file.Cached = true
			file.WouldDownload = false

			return file, nil
		}
	}

	if request.DryRun {
		if sibling, ok := repository.Find(request.Filename); ok {
			file.Size = sibling.Size
		}

		return file, nil
	}

	tmpPath := filepath.Join(paths.tmp, sanitizeIdentity(request.Filename)+"."+strconv.FormatInt(time.Now().UnixNano(), 10))
	publishHubProgress(
		"download.file",
		fmt.Sprintf("downloading Hub file %s", request.Filename),
		qpool.Field{Key: "repo", Value: request.RepoID},
		qpool.Field{Key: "commit", Value: repository.Commit},
		qpool.Field{Key: "file", Value: request.Filename},
	)

	metadata, err := client.downloadTo(
		ctx,
		request,
		tmpPath,
	)

	if err != nil {
		_ = os.Remove(tmpPath)

		return nil, err
	}

	identity := metadata.XetHash

	if identity == "" {
		identity = metadata.ETag
	}

	blobPath, sha, err := installBlob(paths, tmpPath, identity)

	if err != nil {
		return nil, err
	}

	if err := linkSnapshot(blobPath, snapshotPath); err != nil {
		return nil, err
	}

	info, err := os.Stat(snapshotPath)

	if err != nil {
		return nil, fmt.Errorf("hub: stat snapshot: %w", err)
	}

	file.BlobPath = blobPath
	file.ETag = metadata.ETag
	file.XetHash = metadata.XetHash
	file.SHA256 = sha
	file.Size = info.Size()
	file.DownloadedAt = time.Now().UTC()

	if err := paths.writeMetadata(*file); err != nil {
		return nil, err
	}

	return file, nil
}

func cacheRepositoryRevision(paths cachePaths, revision string, repository *Repository) error {
	if err := paths.ensure(); err != nil {
		return err
	}

	if err := paths.writeRef(revision, repository.Commit); err != nil {
		return err
	}

	return paths.writeInfo(revision, repository)
}

/*
Snapshot downloads all matching files for a repository revision.
*/
func (client *Client) Snapshot(
	ctx context.Context, request SnapshotRequest,
) (*Snapshot, error) {
	request = client.normalizeSnapshotRequest(request)

	if err := validateSnapshotRequest(request); err != nil {
		return nil, err
	}

	paths := newCachePaths(request.CacheDir, request.RepoType, request.RepoID)

	if client.config.Offline {
		return offlineSnapshot(paths, request)
	}

	if snapshot, ok := cachedSnapshot(paths, request); ok {
		publishHubProgress(
			"snapshot.cached",
			fmt.Sprintf("using cached Hub snapshot %s", request.RepoID),
			qpool.Field{Key: "repo", Value: request.RepoID},
			qpool.Field{Key: "revision", Value: request.Revision},
			qpool.Field{Key: "commit", Value: snapshot.Commit},
			qpool.Field{Key: "files", Value: len(snapshot.Files)},
		)

		return snapshot, nil
	}

	repository, err := client.Repository(
		ctx,
		request.RepoType,
		request.RepoID,
		request.Revision,
		request.Token,
	)

	if err != nil {
		return nil, err
	}

	if err := cacheRepositoryRevision(paths, request.Revision, repository); err != nil {
		return nil, err
	}

	matches := repository.Matching(request.Include, request.Exclude)

	files, err := client.downloadSnapshotFiles(ctx, request, repository, paths, matches)

	if err != nil {
		return nil, err
	}

	return &Snapshot{
		RepoID:   request.RepoID,
		RepoType: request.RepoType,
		Revision: request.Revision,
		Commit:   repository.Commit,
		Path:     paths.snapshotDir(repository.Commit),
		Files:    files,
	}, nil
}

func (client *Client) downloadSnapshotFiles(
	ctx context.Context,
	request SnapshotRequest,
	repository *Repository,
	paths cachePaths,
	matches []Sibling,
) ([]File, error) {
	files := make([]File, len(matches))
	poolCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerPool := qpool.NewQ(
		poolCtx,
		request.MaxWorkers,
		request.MaxWorkers,
		&qpool.Config{
			SchedulingTimeout:  hubDownloadJobTimeout,
			JobChannelCapacity: max(1, len(matches)),
			Scaler:             nil,
		},
	)
	defer workerPool.Close()

	results := make([]chan *qpool.QValue[any], len(matches))

	for index, sibling := range matches {
		index := index
		downloadRequest := DownloadRequest{
			RepoID:   request.RepoID,
			RepoType: request.RepoType,
			Revision: request.Revision,
			Filename: sibling.Filename,
			CacheDir: request.CacheDir,
			Token:    request.Token,
			Force:    request.Force,
			DryRun:   request.DryRun,
		}

		results[index] = workerPool.Schedule(
			hubSnapshotJobID(repository.Commit, index, sibling.Filename),

			func(jobCtx context.Context) (any, error) {
				file, err := client.downloadPreparedFile(
					jobCtx,
					downloadRequest,
					repository,
					paths,
				)

				if err != nil {
					return nil, err
				}

				return *file, nil
			},
			qpool.WithExecTimeout(hubDownloadJobTimeout),
		)
	}

	for index, result := range results {
		value, err := waitHubSnapshotJob(ctx, result)

		if err != nil {
			cancel()

			return nil, err
		}

		file, valid := value.Value.(File)

		if !valid {
			cancel()

			return nil, fmt.Errorf("hub: snapshot job %d returned %T", index, value.Value)
		}

		files[index] = file
	}

	return files, nil
}

func waitHubSnapshotJob(ctx context.Context, result <-chan *qpool.QValue[any]) (*qpool.QValue[any], error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value, channelOpen := <-result:
		if !channelOpen {
			return nil, fmt.Errorf("hub: snapshot job result channel closed")
		}

		if value == nil {
			return nil, fmt.Errorf("hub: snapshot job returned nil result")
		}

		if value.Error != nil {
			return nil, value.Error
		}

		return value, nil
	}
}

func hubSnapshotJobID(commit string, index int, filename string) string {
	return "hub.snapshot." +
		sanitizeIdentity(commit) +
		"." +
		strconv.Itoa(index) +
		"." +
		sanitizeIdentity(filename)
}

func (client *Client) normalizeDownloadRequest(
	request DownloadRequest,
) DownloadRequest {
	if request.RepoType == "" {
		request.RepoType = ModelRepo
	}

	request.Revision = normalizeRevision(request.Revision)

	if request.CacheDir == "" {
		request.CacheDir = client.config.CacheDir
	}

	if request.Token == "" {
		request.Token = client.config.Token
	}

	return request
}

func (client *Client) normalizeSnapshotRequest(
	request SnapshotRequest,
) SnapshotRequest {
	if request.RepoType == "" {
		request.RepoType = ModelRepo
	}

	request.Revision = normalizeRevision(request.Revision)

	if request.CacheDir == "" {
		request.CacheDir = client.config.CacheDir
	}

	if request.Token == "" {
		request.Token = client.config.Token
	}

	if request.MaxWorkers <= 0 {
		request.MaxWorkers = client.config.MaxWorkers
	}

	if request.MaxWorkers <= 0 {
		request.MaxWorkers = 1
	}

	return request
}

func validateDownloadRequest(request DownloadRequest) error {
	if request.RepoID == "" {
		return fmt.Errorf("hub: repo id is required")
	}

	if request.Filename == "" {
		return fmt.Errorf("hub: filename is required")
	}

	if _, err := parseRepoType(string(request.RepoType)); err != nil {
		return err
	}

	return nil
}

func validateSnapshotRequest(request SnapshotRequest) error {
	if request.RepoID == "" {
		return fmt.Errorf("hub: repo id is required")
	}

	if _, err := parseRepoType(string(request.RepoType)); err != nil {
		return err
	}

	return nil
}

func offlineFile(paths cachePaths, request DownloadRequest) (*File, error) {
	commit, err := readOfflineCommit(paths, request.Revision)

	if err != nil {
		return nil, err
	}

	path := paths.snapshotFile(commit, request.Filename)
	info, err := os.Stat(path)

	if err != nil {
		return nil, fmt.Errorf("hub: offline file %s is not cached: %w", request.Filename, err)
	}

	return &File{
		RepoID:        request.RepoID,
		RepoType:      request.RepoType,
		Revision:      request.Revision,
		Commit:        commit,
		Filename:      request.Filename,
		Path:          path,
		Size:          info.Size(),
		Cached:        true,
		WouldDownload: false,
	}, nil
}

func cachedSnapshotFile(paths cachePaths, request DownloadRequest) (*File, bool) {
	if request.Force || request.DryRun {
		return nil, false
	}

	commit, err := readOfflineCommit(paths, request.Revision)

	if err != nil {
		return nil, false
	}

	path := paths.snapshotFile(commit, request.Filename)
	info, err := os.Stat(path)

	if err != nil {
		return nil, false
	}

	return &File{
		RepoID:        request.RepoID,
		RepoType:      request.RepoType,
		Revision:      request.Revision,
		Commit:        commit,
		Filename:      request.Filename,
		Path:          path,
		Size:          info.Size(),
		Cached:        true,
		WouldDownload: false,
	}, true
}

func offlineSnapshot(paths cachePaths, request SnapshotRequest) (*Snapshot, error) {
	commit, err := readOfflineCommit(paths, request.Revision)

	if err != nil {
		return nil, err
	}

	root := paths.snapshotDir(commit)
	files := make([]File, 0)

	err = filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}

		filename, err := filepath.Rel(root, filePath)

		if err != nil {
			return err
		}

		filename = filepath.ToSlash(filename)

		if !matchesAny(request.Include, filename, true) {
			return nil
		}

		if matchesAny(request.Exclude, filename, false) {
			return nil
		}

		info, err := entry.Info()

		if err != nil {
			return err
		}

		files = append(files, File{
			RepoID:        request.RepoID,
			RepoType:      request.RepoType,
			Revision:      request.Revision,
			Commit:        commit,
			Filename:      filename,
			Path:          filePath,
			Size:          info.Size(),
			Cached:        true,
			WouldDownload: false,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("hub: offline snapshot: %w", err)
	}

	return &Snapshot{
		RepoID:   request.RepoID,
		RepoType: request.RepoType,
		Revision: request.Revision,
		Commit:   commit,
		Path:     root,
		Files:    files,
	}, nil
}

func cachedSnapshot(paths cachePaths, request SnapshotRequest) (*Snapshot, bool) {
	if request.Force || request.DryRun {
		return nil, false
	}

	repository, err := cachedRepository(paths, request)

	if err != nil {
		return nil, false
	}

	matches := repository.Matching(request.Include, request.Exclude)
	files := make([]File, len(matches))

	for index, sibling := range matches {
		path := paths.snapshotFile(repository.Commit, sibling.Filename)
		info, err := os.Stat(path)

		if err != nil {
			return nil, false
		}

		files[index] = File{
			RepoID:        request.RepoID,
			RepoType:      request.RepoType,
			Revision:      request.Revision,
			Commit:        repository.Commit,
			Filename:      sibling.Filename,
			Path:          path,
			Size:          info.Size(),
			Cached:        true,
			WouldDownload: false,
		}
	}

	return &Snapshot{
		RepoID:   request.RepoID,
		RepoType: request.RepoType,
		Revision: request.Revision,
		Commit:   repository.Commit,
		Path:     paths.snapshotDir(repository.Commit),
		Files:    files,
	}, true
}

func cachedRepository(paths cachePaths, request SnapshotRequest) (*Repository, error) {
	payload, err := readInfoPayload(paths, request.Revision)

	if err != nil {
		return nil, err
	}

	return repositoryFromPayload(request.RepoType, request.Revision, request.RepoID, payload)
}

func readOfflineCommit(paths cachePaths, revision string) (string, error) {
	data, err := os.ReadFile(paths.refFile(revision))

	if err == nil {
		commit := strings.TrimSpace(string(data))

		if commit == "" {
			return "", fmt.Errorf("hub: offline revision %s has an empty ref", revision)
		}

		return commit, nil
	}

	payload, infoErr := readInfoPayload(paths, revision)

	if infoErr == nil && payload.SHA != "" {
		return payload.SHA, nil
	}

	if looksLikeCommitHash(revision) {
		if info, statErr := os.Stat(paths.snapshotDir(revision)); statErr == nil && info.IsDir() {
			return revision, nil
		}
	}

	return "", fmt.Errorf("hub: offline revision %s is not cached: %w", revision, err)
}

func readInfoPayload(paths cachePaths, revision string) (repositoryPayload, error) {
	data, err := os.ReadFile(paths.infoFile(revision))

	if err != nil {
		return repositoryPayload{}, err
	}

	var payload repositoryPayload

	if err := json.Unmarshal(data, &payload); err != nil {
		return repositoryPayload{}, fmt.Errorf("hub: parse cached info %s: %w", revision, err)
	}

	return payload, nil
}

func looksLikeCommitHash(revision string) bool {
	if len(revision) < 7 {
		return false
	}

	for _, value := range revision {
		if value >= '0' && value <= '9' {
			continue
		}

		if value >= 'a' && value <= 'f' {
			continue
		}

		return false
	}

	return true
}

func (client *Client) downloadTo(
	ctx context.Context, request DownloadRequest, tmpPath string,
) (remoteMetadata, error) {
	publishHubProgress(
		"download.probe",
		fmt.Sprintf("probing Hub file transport for %s", request.Filename),
		qpool.Field{Key: "repo", Value: request.RepoID},
		qpool.Field{Key: "file", Value: request.Filename},
	)

	probe, err := client.probe(ctx, request)

	if err != nil {
		return remoteMetadata{}, err
	}

	if probe.XetHash != "" && client.config.Xet.Active {
		publishHubProgress(
			"download.xet",
			fmt.Sprintf("reconstructing Hub file %s through Xet CAS", request.Filename),
			qpool.Field{Key: "repo", Value: request.RepoID},
			qpool.Field{Key: "file", Value: request.Filename},
			qpool.Field{Key: "xet_hash", Value: probe.XetHash},
			qpool.Field{Key: "size", Value: probe.Size},
		)

		return client.downloadXet(ctx, request, probe, tmpPath)
	}

	publishHubProgress(
		"download.http",
		fmt.Sprintf("downloading Hub file %s through HTTP", request.Filename),
		qpool.Field{Key: "repo", Value: request.RepoID},
		qpool.Field{Key: "file", Value: request.Filename},
		qpool.Field{Key: "size", Value: probe.Size},
	)

	return client.downloadHTTP(ctx, request, tmpPath)
}

func (client *Client) probe(
	ctx context.Context, request DownloadRequest,
) (remoteMetadata, error) {
	head, err := client.probeWithMethod(ctx, request, http.MethodHead)

	if err != nil {
		return remoteMetadata{}, err
	}

	if head.Size != statusMethodUnsupported {
		if head.XetHash != "" || head.Size == statusOKNoRedirect {
			return head, nil
		}
	}

	return client.probeWithMethod(ctx, request, http.MethodGet)
}

const (
	statusMethodUnsupported = -2
	statusOKNoRedirect      = -1
)

func (client *Client) probeWithMethod(
	ctx context.Context, request DownloadRequest, method string,
) (remoteMetadata, error) {
	requestURL, err := resolveURL(
		client.config.Endpoint,
		request.RepoType,
		request.RepoID,
		request.Revision,
		request.Filename,
	)

	if err != nil {
		return remoteMetadata{}, err
	}

	resp, err := client.httpClient.Custom(
		requestURL,
		method,
		requestConfig(ctx, request.Token, 0),
	)

	if err != nil {
		return remoteMetadata{}, fmt.Errorf("hub: probe %s: %w", request.Filename, err)
	}

	defer resp.Close()

	if resp.StatusCode() == http.StatusMethodNotAllowed || resp.StatusCode() == http.StatusNotImplemented {
		return remoteMetadata{Size: statusMethodUnsupported}, nil
	}

	if resp.StatusCode() < 200 || resp.StatusCode() >= 400 {
		return remoteMetadata{}, statusError("hub: probe", resp)
	}

	size := responseContentLength(resp)

	if resp.StatusCode() == http.StatusOK {
		size = statusOKNoRedirect
	}

	return remoteMetadata{
		ETag:    resp.Header("ETag"),
		XetHash: resp.Header("X-Xet-Hash"),
		Size:    size,
	}, nil
}

func (client *Client) downloadHTTP(
	ctx context.Context, request DownloadRequest, tmpPath string,
) (remoteMetadata, error) {
	requestURL, err := resolveURL(
		client.config.Endpoint,
		request.RepoType,
		request.RepoID,
		request.Revision,
		request.Filename,
	)

	if err != nil {
		return remoteMetadata{}, err
	}

	resp, err := client.httpClient.Get(
		requestURL,
		requestConfig(ctx, request.Token, 10),
	)

	if err != nil {
		return remoteMetadata{}, fmt.Errorf("hub: download %s: %w", request.Filename, err)
	}

	defer resp.Close()

	if resp.StatusCode() != http.StatusOK {
		return remoteMetadata{}, statusError("hub: download", resp)
	}

	if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
		return remoteMetadata{}, fmt.Errorf("hub: mkdir temp: %w", err)
	}

	file, err := os.Create(tmpPath)

	if err != nil {
		return remoteMetadata{}, fmt.Errorf("hub: create temp: %w", err)
	}

	defer file.Close()

	contentLength := responseContentLength(resp)

	if err := copyHTTPWithProgress(request.Filename, file, resp.BodyStream(), contentLength); err != nil {
		return remoteMetadata{}, fmt.Errorf("hub: write temp: %w", err)
	}

	return remoteMetadata{
		ETag:    resp.Header("ETag"),
		XetHash: resp.Header("X-Xet-Hash"),
		Size:    contentLength,
	}, nil
}

const downloadProgressStride = 64 * 1024 * 1024

func copyHTTPWithProgress(
	filename string,
	writer io.Writer,
	reader io.Reader,
	expected int64,
) error {
	scratch := make([]byte, 4*1024*1024)
	nextProgress := int64(downloadProgressStride)
	var read int64

	for {
		size, readErr := reader.Read(scratch)

		if size > 0 {
			read += int64(size)

			if _, writeErr := writer.Write(scratch[:size]); writeErr != nil {
				return writeErr
			}

			if expected > 0 && read >= nextProgress {
				publishHubProgress(
					"download.http.read",
					fmt.Sprintf(
						"read %d MiB of Hub file %s",
						read/(1024*1024),
						filename,
					),
					qpool.Field{Key: "file", Value: filename},
					qpool.Field{Key: "read_bytes", Value: read},
					qpool.Field{Key: "expected_bytes", Value: expected},
				)
				nextProgress += downloadProgressStride
			}
		}

		if readErr == nil {
			continue
		}

		if readErr == io.EOF {
			break
		}

		return readErr
	}

	publishHubProgress(
		"download.http.fetched",
		fmt.Sprintf("fetched %d MiB of Hub file %s", read/(1024*1024), filename),
		qpool.Field{Key: "file", Value: filename},
		qpool.Field{Key: "read_bytes", Value: read},
		qpool.Field{Key: "expected_bytes", Value: expected},
		qpool.Field{Key: "done", Value: true},
	)

	return nil
}

func (repository *Repository) Find(filename string) (Sibling, bool) {
	for _, sibling := range repository.Siblings {
		if sibling.Filename == filename {
			return sibling, true
		}
	}

	return Sibling{}, false
}

func (repository *Repository) Matching(
	include []string, exclude []string,
) []Sibling {
	matches := make([]Sibling, 0, len(repository.Siblings))

	for _, sibling := range repository.Siblings {
		if !matchesAny(include, sibling.Filename, true) {
			continue
		}

		if matchesAny(exclude, sibling.Filename, false) {
			continue
		}

		matches = append(matches, sibling)
	}

	return matches
}

func matchesAny(patterns []string, filename string, emptyValue bool) bool {
	if len(patterns) == 0 {
		return emptyValue
	}

	for _, pattern := range patterns {
		if glob(pattern, filename) {
			return true
		}
	}

	return false
}
