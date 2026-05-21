package dataset

import (
	"math/rand/v2"
)

type shuffleConfig struct {
	bufferSize    int
	shuffleShards bool
}

type shuffleBuffer struct {
	cfg  *shuffleConfig
	rng  *rand.Rand
	rows []Row
}

func newShuffleBuffer(cfg *shuffleConfig, seed int64, epoch int) *shuffleBuffer {
	if cfg == nil {
		return nil
	}

	return &shuffleBuffer{
		cfg:  cfg,
		rng:  rand.New(rand.NewPCG(uint64(seed+int64(epoch)), uint64(seed>>32))),
		rows: make([]Row, 0, cfg.bufferSize),
	}
}

func (buffer *shuffleBuffer) Add(row Row, yield func(Row, error) bool) bool {
	buffer.rows = append(buffer.rows, row)

	if len(buffer.rows) < buffer.cfg.bufferSize {
		return true
	}

	index := buffer.rng.IntN(len(buffer.rows))
	selected := buffer.rows[index]
	buffer.rows[index] = buffer.rows[len(buffer.rows)-1]
	buffer.rows = buffer.rows[:len(buffer.rows)-1]

	return yield(selected, nil)
}

func (buffer *shuffleBuffer) Flush(yield func(Row, error) bool) {
	for len(buffer.rows) > 0 {
		index := buffer.rng.IntN(len(buffer.rows))
		selected := buffer.rows[index]
		buffer.rows[index] = buffer.rows[len(buffer.rows)-1]
		buffer.rows = buffer.rows[:len(buffer.rows)-1]

		if !yield(selected, nil) {
			return
		}
	}
}

func shuffleStringsShards(shards []shard, seed int64) []shard {
	if len(shards) <= 1 {
		return shards
	}

	cloned := append([]shard(nil), shards...)
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed>>32)))

	for index := len(cloned) - 1; index > 0; index-- {
		swap := rng.IntN(index + 1)
		cloned[index], cloned[swap] = cloned[swap], cloned[index]
	}

	return cloned
}
