package dataset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"

	"github.com/parquet-go/parquet-go"
)

func openParquetShard(
	ctx context.Context,
	opener *streamOpener,
	shardSource shard,
) (iter.Seq2[Row, error], error) {
	if shardSource.path != "" {
		return streamLocalParquet(shardSource)
	}

	size := shardSource.size

	if size <= 0 {
		var err error

		size, err = opener.objectSize(ctx, shardSource.url, shardSource.token)

		if err != nil {
			return nil, err
		}
	}

	if size <= 0 {
		return nil, fmt.Errorf("dataset: parquet object size unavailable for %s", shardSource.url)
	}

	reader := &rangeHTTP{
		opener: opener,
		ctx:    ctx,
		url:    shardSource.url,
		token:  shardSource.token,
		size:   size,
	}

	return streamParquetReader(parquet.NewGenericReader[map[string]any](reader), shardSource.columns)
}

func streamLocalParquet(shardSource shard) (iter.Seq2[Row, error], error) {
	file, err := os.Open(shardSource.path)

	if err != nil {
		return nil, fmt.Errorf("dataset: open local parquet: %w", err)
	}

	reader := parquet.NewGenericReader[map[string]any](file)

	return func(yield func(Row, error) bool) {
		defer file.Close()
		defer reader.Close()

		buffer := make([]map[string]any, 1)

		for {
			count, err := reader.Read(buffer)

			if count == 0 {
				if err != nil && !errors.Is(err, io.EOF) {
					yield(nil, err)
				}

				return
			}

			row := Row(buffer[0])

			if len(shardSource.columns) > 0 {
				row = projectColumns(row, shardSource.columns)
			}

			if !yield(row, nil) {
				return
			}
		}
	}, nil
}

func streamParquetReader(
	reader *parquet.GenericReader[map[string]any],
	columns []string,
) (iter.Seq2[Row, error], error) {
	return func(yield func(Row, error) bool) {
		defer reader.Close()

		buffer := make([]map[string]any, 1)

		for {
			count, err := reader.Read(buffer)

			if count == 0 {
				if err != nil && !errors.Is(err, io.EOF) {
					yield(nil, err)
				}

				return
			}

			row := Row(buffer[0])

			if len(columns) > 0 {
				row = projectColumns(row, columns)
			}

			if !yield(row, nil) {
				return
			}
		}
	}, nil
}
