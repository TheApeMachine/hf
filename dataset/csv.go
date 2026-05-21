package dataset

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"io"
	"iter"
	"strings"
)

func openCSVShard(
	ctx context.Context,
	opener *streamOpener,
	shardSource shard,
) (iter.Seq2[Row, error], error) {
	reader, closer, err := openShardReader(ctx, opener, shardSource)

	if err != nil {
		return nil, err
	}

	name := shardName(shardSource)

	delimiter := rune(',')

	if strings.HasSuffix(strings.ToLower(name), ".tsv") {
		delimiter = '\t'
	}

	if strings.HasSuffix(strings.ToLower(name), ".gz") {
		gzipReader, err := gzip.NewReader(reader)

		if err != nil {
			closer.Close()
			return nil, err
		}

		reader = gzipReader
		closer = combineClosers(closer, gzipReader)
	}

	csvReader := csv.NewReader(reader)
	csvReader.ReuseRecord = true
	csvReader.FieldsPerRecord = -1
	csvReader.Comma = delimiter

	return func(yield func(Row, error) bool) {
		defer closer.Close()

		header, err := csvReader.Read()

		if err != nil {
			if err == io.EOF {
				return
			}

			yield(nil, err)
			return
		}

		columns := append([]string(nil), header...)

		for {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			record, err := csvReader.Read()

			if err == io.EOF {
				return
			}

			if err != nil {
				yield(nil, err)
				return
			}

			row := make(Row, len(columns))

			for index, column := range columns {
				if index < len(record) {
					row[column] = record[index]
				}
			}

			if !yield(row, nil) {
				return
			}
		}
	}, nil
}
