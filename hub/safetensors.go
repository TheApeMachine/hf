package hub

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"syscall"
)

type TensorMeta struct {
	DType       string   `json:"dtype"`
	Shape       []int64  `json:"shape"`
	DataOffsets [2]int64 `json:"data_offsets"`
}

type SafeTensorsFile struct {
	file     *os.File
	data     []byte
	Index    map[string]TensorMeta
	DataBase int64
}

func OpenSafeTensors(path string) (*SafeTensorsFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open safetensors: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat safetensors: %w", err)
	}

	// mmap the entire file
	data, err := syscall.Mmap(int(file.Fd()), 0, int(info.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("mmap safetensors: %w", err)
	}

	if len(data) < 8 {
		syscall.Munmap(data)
		file.Close()
		return nil, fmt.Errorf("safetensors file too small")
	}

	headerLength := binary.LittleEndian.Uint64(data[:8])
	if uint64(len(data)) < 8+headerLength {
		syscall.Munmap(data)
		file.Close()
		return nil, fmt.Errorf("safetensors file truncated")
	}

	headerBytes := data[8 : 8+headerLength]
	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(headerBytes, &raw); err != nil {
		syscall.Munmap(data)
		file.Close()
		return nil, fmt.Errorf("safetensors parse header: %w", err)
	}

	index := make(map[string]TensorMeta, len(raw))
	for name, rawMeta := range raw {
		if name == "__metadata__" {
			continue
		}

		var meta TensorMeta
		if err := json.Unmarshal(rawMeta, &meta); err != nil {
			syscall.Munmap(data)
			file.Close()
			return nil, fmt.Errorf("safetensors parse tensor %q: %w", name, err)
		}
		index[name] = meta
	}

	return &SafeTensorsFile{
		file:     file,
		data:     data,
		Index:    index,
		DataBase: int64(8 + headerLength),
	}, nil
}

func (st *SafeTensorsFile) Close() error {
	if st.data != nil {
		syscall.Munmap(st.data)
		st.data = nil
	}
	if st.file != nil {
		st.file.Close()
		st.file = nil
	}
	return nil
}

func (st *SafeTensorsFile) Tensor(name string) ([]byte, TensorMeta, error) {
	meta, ok := st.Index[name]
	if !ok {
		return nil, TensorMeta{}, fmt.Errorf("tensor %q not found", name)
	}

	start := st.DataBase + meta.DataOffsets[0]
	end := st.DataBase + meta.DataOffsets[1]

	if start < 0 || end > int64(len(st.data)) || start > end {
		return nil, TensorMeta{}, fmt.Errorf("tensor %q offsets out of bounds", name)
	}

	return st.data[start:end], meta, nil
}
