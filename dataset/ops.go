package dataset

import (
	"context"
	"fmt"
	"iter"
)

func (dataset *IterableDataset) Take(n int) *IterableDataset {
	cloned := dataset.clone()
	cloned.take = n
	return cloned
}

/*
Skip returns a dataset view that omits the first n examples.
*/
func (dataset *IterableDataset) Skip(n int) *IterableDataset {
	cloned := dataset.clone()
	cloned.skip = n
	return cloned
}

/*
Shard splits the dataset into numShards views and returns the selected shard.
*/
func (dataset *IterableDataset) Shard(numShards, index int) (*IterableDataset, error) {
	if numShards <= 0 {
		return nil, fmt.Errorf("dataset: num_shards must be positive")
	}

	if index < 0 || index >= numShards {
		return nil, fmt.Errorf("dataset: shard index %d out of range for num_shards=%d", index, numShards)
	}

	cloned := dataset.clone()
	cloned.numShards = numShards
	cloned.shardIndex = index
	return cloned, nil
}

/*
Shuffle returns a shuffled dataset view using an in-memory buffer.
*/
func (dataset *IterableDataset) Shuffle(seed int64, bufferSize int) *IterableDataset {
	cloned := dataset.clone()

	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}

	cloned.seed = seed
	cloned.shuffle = &shuffleConfig{
		bufferSize:    bufferSize,
		shuffleShards: true,
	}

	return cloned
}

/*
SelectColumns returns a dataset view that keeps only the requested columns.
*/
func (dataset *IterableDataset) SelectColumns(columns ...string) *IterableDataset {
	cloned := dataset.clone()
	cloned.selectCols = append([]string(nil), columns...)
	cloned.features = Features{Columns: append([]string(nil), columns...)}
	return cloned
}

/*
Map applies fn to each row as it is streamed.
*/
func (dataset *IterableDataset) Map(fn func(Row) (Row, error)) *IterableDataset {
	cloned := dataset.clone()
	cloned.mapFn = fn
	return cloned
}

/*
Filter keeps rows for which fn returns true.
*/
func (dataset *IterableDataset) Filter(fn func(Row) (bool, error)) *IterableDataset {
	cloned := dataset.clone()
	cloned.filterFn = fn
	return cloned
}

/*
Column returns an iterator over one column, similar to dataset["text"] in Python.
*/
func (dataset *IterableDataset) Column(ctx context.Context, name string) iter.Seq2[any, error] {
	return func(yield func(any, error) bool) {
		for row, err := range dataset.All(ctx) {
			if err != nil {
				yield(nil, err)
				return
			}

			if !yield(row[name], nil) {
				return
			}
		}
	}
}
