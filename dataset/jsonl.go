package dataset

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"iter"
	"os"
	"strings"
)

func openJSONLShard(
	ctx context.Context,
	opener *streamOpener,
	shardSource shard,
) (iter.Seq2[Row, error], error) {
	reader, closer, err := openShardReader(ctx, opener, shardSource)

	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(strings.ToLower(shardName(shardSource)), ".gz") {
		gzipReader, err := gzip.NewReader(reader)

		if err != nil {
			closer.Close()
			return nil, err
		}

		reader = gzipReader
		closer = combineClosers(closer, gzipReader)
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	return func(yield func(Row, error) bool) {
		defer closer.Close()

		for scanner.Scan() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			line := strings.TrimSpace(scanner.Text())

			if line == "" {
				continue
			}

			row := make(Row)

			if err := json.Unmarshal([]byte(line), &row); err != nil {
				yield(nil, err)
				return
			}

			if !yield(row, nil) {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			yield(nil, err)
		}
	}, nil
}

func openShardReader(
	ctx context.Context,
	opener *streamOpener,
	shardSource shard,
) (io.Reader, io.Closer, error) {
	if shardSource.path != "" {
		file, err := os.Open(shardSource.path)

		if err != nil {
			return nil, nil, err
		}

		return file, file, nil
	}

	reader, _, err := opener.openRemote(ctx, shardSource.url, shardSource.token)

	if err != nil {
		return nil, nil, err
	}

	return reader, reader, nil
}

func shardName(shardSource shard) string {
	if shardSource.path != "" {
		return shardSource.path
	}

	return shardSource.url
}

type multiCloser struct {
	closers []io.Closer
}

func (closer *multiCloser) Close() error {
	var first error

	for _, item := range closer.closers {
		if err := item.Close(); err != nil && first == nil {
			first = err
		}
	}

	return first
}

func combineClosers(closers ...io.Closer) io.Closer {
	return &multiCloser{closers: closers}
}
