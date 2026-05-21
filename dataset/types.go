package dataset

import (
	"context"
	"iter"
)

const (
	defaultConfig     = "default"
	defaultSplit      = "train"
	defaultRevision   = "main"
	defaultBufferSize = 1000
)

/*
Row is one dataset example. Keys correspond to column names.
*/
type Row map[string]any

/*
Features describes the columns exposed by an iterable dataset.
*/
type Features struct {
	Columns []string
}

/*
LoadOptions configures streaming dataset loading from the Hugging Face Hub.
*/
type LoadOptions struct {
	Config    string
	Split     string
	Revision  string
	Token     string
	Columns   []string
	DataFiles map[string][]string
}

/*
IterableDataset streams examples without downloading the full dataset first.
It mirrors the Python datasets IterableDataset API for training workflows.
*/
type IterableDataset struct {
	shards     []shard
	features   Features
	epoch      int
	seed       int64
	skip       int
	take       int
	shuffle    *shuffleConfig
	shardIndex int
	numShards  int
	selectCols []string
	mapFn      func(Row) (Row, error)
	filterFn   func(Row) (bool, error)
}

/*
NumShards returns the number of underlying file shards.
*/
func (dataset *IterableDataset) NumShards() int {
	return len(dataset.shards)
}

/*
FeaturesInfo returns the dataset column names when known.
*/
func (dataset *IterableDataset) FeaturesInfo() Features {
	return dataset.features
}

/*
SetEpoch updates the shuffle epoch, matching IterableDataset.set_epoch in Python.
*/
func (dataset *IterableDataset) SetEpoch(epoch int) *IterableDataset {
	cloned := dataset.clone()
	cloned.epoch = epoch
	return cloned
}

/*
All iterates over dataset rows. Iteration stops on the first error.
*/
func (dataset *IterableDataset) All(ctx context.Context) iter.Seq2[Row, error] {
	return func(yield func(Row, error) bool) {
		stream := dataset.rowStream(ctx)
		buffer := newShuffleBuffer(dataset.shuffle, dataset.seed, dataset.epoch)

		for row, err := range stream {
			if err != nil {
				yield(nil, err)
				return
			}

			row, err = dataset.transform(row)

			if err != nil {
				yield(nil, err)
				return
			}

			if row == nil {
				continue
			}

			if buffer != nil {
				if !buffer.Add(row, yield) {
					return
				}

				continue
			}

			if !yield(row, nil) {
				return
			}
		}

		if buffer != nil {
			buffer.Flush(yield)
		}
	}
}

func (dataset *IterableDataset) clone() *IterableDataset {
	cloned := *dataset

	if dataset.shuffle != nil {
		value := *dataset.shuffle
		cloned.shuffle = &value
	}

	if len(dataset.selectCols) > 0 {
		cloned.selectCols = append([]string(nil), dataset.selectCols...)
	}

	if len(dataset.shards) > 0 {
		cloned.shards = append([]shard(nil), dataset.shards...)
	}

	return &cloned
}

func (dataset *IterableDataset) transform(row Row) (Row, error) {
	if dataset.mapFn != nil {
		var err error

		row, err = dataset.mapFn(row)

		if err != nil {
			return nil, err
		}
	}

	if dataset.filterFn != nil {
		keep, err := dataset.filterFn(row)

		if err != nil {
			return nil, err
		}

		if !keep {
			return nil, nil
		}
	}

	if len(dataset.selectCols) > 0 {
		row = projectColumns(row, dataset.selectCols)
	}

	return row, nil
}

func projectColumns(row Row, columns []string) Row {
	projected := make(Row, len(columns))

	for _, column := range columns {
		projected[column] = row[column]
	}

	return projected
}
