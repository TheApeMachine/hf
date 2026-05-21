package hub

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

/*
Endpoint returns the configured Hugging Face Hub base URL.
*/
func (client *Client) Endpoint() string {
	if client.config == nil {
		return "https://huggingface.co"
	}

	return client.config.Endpoint
}

/*
HubToken returns the configured Hub authentication token.
*/
func (client *Client) HubToken() string {
	if client.config == nil {
		return ""
	}

	return client.config.Token
}

/*
OpenStream opens an HTTP response body for a Hub file without writing it to
the local cache. The returned size is the Content-Length when available, or -1.
*/
func (client *Client) OpenStream(
	ctx context.Context, request DownloadRequest,
) (io.ReadCloser, int64, error) {
	requestURL, err := ResolveURL(
		client.Endpoint(),
		request.RepoType,
		request.RepoID,
		request.Revision,
		request.Filename,
	)

	if err != nil {
		return nil, -1, err
	}

	token := request.Token

	if token == "" {
		token = client.HubToken()
	}

	resp, err := client.httpClient.Get(requestURL, requestConfig(ctx, token, 10))

	if err != nil {
		return nil, -1, fmt.Errorf("hub: stream %s: %w", request.Filename, err)
	}

	if resp.StatusCode() != http.StatusOK {
		defer resp.Close()
		return nil, -1, statusError("hub: stream", resp)
	}

	return &streamBody{
		reader: resp.BodyStream(),
		close: func() error {
			resp.Close()
			return nil
		},
	}, responseContentLength(resp), nil
}

/*
ResolveURL builds the Hub resolve URL for a repository file.
*/
func ResolveURL(
	endpoint string, repoType RepoType, repoID, revision, filename string,
) (string, error) {
	return resolveURL(endpoint, repoType, repoID, revision, filename)
}

type streamBody struct {
	reader io.Reader
	close  func() error
}

func (body *streamBody) Read(p []byte) (int, error) {
	return body.reader.Read(p)
}

func (body *streamBody) Close() error {
	return body.close()
}
