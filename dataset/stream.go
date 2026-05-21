package dataset

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	fiberclient "github.com/gofiber/fiber/v3/client"
)

type shard struct {
	url     string
	path    string
	format  fileFormat
	size    int64
	token   string
	columns []string
}

type fileFormat int

const (
	formatUnknown fileFormat = iota
	formatJSONL
	formatCSV
	formatParquet
)

type streamOpener struct {
	client *fiberclient.Client
}

func newStreamOpener() *streamOpener {
	client := fiberclient.New()
	client.SetStreamResponseBody(true)

	return &streamOpener{client: client}
}

func (opener *streamOpener) openRemote(
	ctx context.Context,
	url string,
	token string,
) (io.ReadCloser, int64, error) {
	resp, err := opener.client.Get(url, fiberclient.Config{
		Ctx:    ctx,
		Header: authHeader(token),
	})

	if err != nil {
		return nil, -1, fmt.Errorf("dataset: open stream: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		defer resp.Close()
		return nil, -1, httpStatusError("dataset: open stream", resp.StatusCode(), resp.String())
	}

	return &remoteStream{
		reader: resp.BodyStream(),
		close: func() error {
			resp.Close()
			return nil
		},
	}, responseContentLength(resp), nil
}

func (opener *streamOpener) objectSize(
	ctx context.Context,
	url string,
	token string,
) (int64, error) {
	resp, err := opener.client.Head(url, fiberclient.Config{
		Ctx:    ctx,
		Header: authHeader(token),
	})

	if err != nil {
		return -1, err
	}

	defer resp.Close()

	if resp.StatusCode() != http.StatusOK {
		return -1, httpStatusError("dataset: head", resp.StatusCode(), resp.String())
	}

	return responseContentLength(resp), nil
}

type remoteStream struct {
	reader io.Reader
	close  func() error
}

func (stream *remoteStream) Read(p []byte) (int, error) {
	return stream.reader.Read(p)
}

func (stream *remoteStream) Close() error {
	return stream.close()
}

type rangeHTTP struct {
	opener *streamOpener
	ctx    context.Context
	url    string
	token  string
	size   int64
}

func (reader *rangeHTTP) Size() int64 {
	return reader.size
}

func (reader *rangeHTTP) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("dataset: negative read offset")
	}

	if reader.size >= 0 && off >= reader.size {
		return 0, io.EOF
	}

	end := off + int64(len(p)) - 1

	if reader.size >= 0 && end >= reader.size {
		end = reader.size - 1
	}

	resp, err := reader.opener.client.Get(reader.url, fiberclient.Config{
		Ctx: reader.ctx,
		Header: mergeHeaders(authHeader(reader.token), map[string]string{
			"Range": fmt.Sprintf("bytes=%d-%d", off, end),
		}),
	})

	if err != nil {
		return 0, err
	}

	defer resp.Close()

	if resp.StatusCode() != http.StatusPartialContent && resp.StatusCode() != http.StatusOK {
		return 0, httpStatusError("dataset: range read", resp.StatusCode(), resp.String())
	}

	body := resp.Body()

	if int64(len(body)) > int64(len(p)) {
		body = body[:len(p)]
	}

	copy(p, body)

	if len(body) == 0 {
		return 0, io.EOF
	}

	return len(body), nil
}

func mergeHeaders(headers ...map[string]string) map[string]string {
	merged := make(map[string]string)

	for _, header := range headers {
		for key, value := range header {
			merged[key] = value
		}
	}

	return merged
}

func responseContentLength(resp interface{ Header(string) string }) int64 {
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

func detectFormat(filename string) fileFormat {
	lower := strings.ToLower(filename)

	switch {
	case strings.HasSuffix(lower, ".jsonl"), strings.HasSuffix(lower, ".jsonl.gz"):
		return formatJSONL
	case strings.HasSuffix(lower, ".csv"), strings.HasSuffix(lower, ".csv.gz"), strings.HasSuffix(lower, ".tsv"):
		return formatCSV
	case strings.HasSuffix(lower, ".parquet"):
		return formatParquet
	default:
		return formatUnknown
	}
}

func splitFilePatterns(split string) []string {
	return []string{
		fmt.Sprintf("**/%s/**", split),
		fmt.Sprintf("**/%s.*", split),
		fmt.Sprintf("**/%s-*", split),
		fmt.Sprintf("**/%s_*", split),
		fmt.Sprintf("**/*/%s/**", split),
	}
}
