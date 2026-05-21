package hub

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	fiberclient "github.com/gofiber/fiber/v3/client"
)

type repositoryPayload struct {
	ID       string           `json:"id"`
	SHA      string           `json:"sha"`
	Siblings []siblingPayload `json:"siblings"`
}

type siblingPayload struct {
	RFilename string      `json:"rfilename"`
	Size      int64       `json:"size"`
	LFS       *lfsPayload `json:"lfs"`
}

type lfsPayload struct {
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	PointerSize int64  `json:"pointerSize"`
}

func (client *Client) Repository(
	ctx context.Context, repoType RepoType, repoID, revision, token string,
) (*Repository, error) {
	repoType, err := parseRepoType(string(repoType))

	if err != nil {
		return nil, err
	}

	revision = normalizeRevision(revision)
	apiPlural, err := repoType.apiPlural()

	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(client.config.Endpoint, "/")
	requestURL := fmt.Sprintf(
		"%s/api/%s/%s/revision/%s?blobs=true",
		endpoint,
		apiPlural,
		escapeRepoID(repoID),
		url.PathEscape(revision),
	)

	resp, err := client.httpClient.Get(requestURL, requestConfig(ctx, token, 0))

	if err != nil {
		return nil, fmt.Errorf("hub: repository %s: %w", repoID, err)
	}

	defer resp.Close()

	if resp.StatusCode() != http.StatusOK {
		return nil, statusError("hub: repository", resp)
	}

	var payload repositoryPayload

	if err := resp.JSON(&payload); err != nil {
		return nil, fmt.Errorf("hub: decode repository: %w", err)
	}

	return repositoryFromPayload(repoType, revision, repoID, payload)
}

func repositoryFromPayload(
	repoType RepoType,
	revision string,
	fallbackID string,
	payload repositoryPayload,
) (*Repository, error) {
	if payload.SHA == "" {
		return nil, fmt.Errorf("hub: repository %s returned no commit sha", fallbackID)
	}

	siblings := make([]Sibling, 0, len(payload.Siblings))

	for _, sibling := range payload.Siblings {
		entry := Sibling{
			Filename: sibling.RFilename,
			Size:     sibling.Size,
		}

		if sibling.LFS != nil {
			entry.LFS = &LFSMetadata{
				SHA256:      sibling.LFS.SHA256,
				Size:        sibling.LFS.Size,
				PointerSize: sibling.LFS.PointerSize,
			}

			if entry.Size == 0 {
				entry.Size = sibling.LFS.Size
			}
		}

		if entry.Filename != "" {
			siblings = append(siblings, entry)
		}
	}

	repoID := payload.ID

	if repoID == "" {
		repoID = fallbackID
	}

	return &Repository{
		ID:       repoID,
		RepoType: repoType,
		Revision: revision,
		Commit:   payload.SHA,
		Siblings: siblings,
	}, nil
}

func resolveURL(
	endpoint string, repoType RepoType, repoID, revision, filename string,
) (string, error) {
	prefix, err := repoType.resolvePrefix()

	if err != nil {
		return "", err
	}

	parts := []string{strings.TrimRight(endpoint, "/")}

	if prefix != "" {
		parts = append(parts, prefix)
	}

	parts = append(
		parts,
		escapeRepoID(repoID),
		"resolve",
		url.PathEscape(normalizeRevision(revision)),
		escapeFilePath(filename),
	)

	return strings.Join(parts, "/"), nil
}

func escapeRepoID(repoID string) string {
	parts := strings.Split(repoID, "/")

	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}

	return strings.Join(parts, "/")
}

func escapeFilePath(filename string) string {
	parts := strings.Split(path.Clean(filename), "/")

	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}

	return strings.Join(parts, "/")
}

func requestConfig(
	ctx context.Context, token string, maxRedirects int,
) fiberclient.Config {
	cfg := fiberclient.Config{Ctx: ctx}

	if maxRedirects > 0 {
		cfg.MaxRedirects = maxRedirects
	}

	if strings.TrimSpace(token) != "" {
		cfg.Header = map[string]string{
			"Authorization": "Bearer " + token,
		}
	}

	return cfg
}

func statusError(prefix string, resp *fiberclient.Response) error {
	message := strings.TrimSpace(resp.String())

	if message == "" {
		return fmt.Errorf("%s: HTTP %d", prefix, resp.StatusCode())
	}

	return fmt.Errorf("%s: HTTP %d: %s", prefix, resp.StatusCode(), message)
}

func responseContentLength(resp *fiberclient.Response) int64 {
	contentLength := resp.Header("Content-Length")

	if contentLength == "" {
		return -1
	}

	size, err := strconv.ParseInt(contentLength, 10, 64)

	if err != nil {
		return -1
	}

	return size
}
