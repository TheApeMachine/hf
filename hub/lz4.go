package hub

import (
	"encoding/binary"
	"fmt"
)

const (
	lz4FrameMagic       = 0x184d2204
	lz4FrameBlockRawBit = 0x80000000
)

func decodeLZ4Payload(src []byte, uncompressedSize int) ([]byte, error) {
	if isLZ4Frame(src) {
		return decodeLZ4Frame(src, uncompressedSize)
	}

	return decodeLZ4Block(src, uncompressedSize)
}

func decodeLZ4Block(src []byte, uncompressedSize int) ([]byte, error) {
	return decodeLZ4BlockWithPrefix(src, uncompressedSize, nil)
}

func decodeLZ4BlockWithPrefix(
	src []byte, uncompressedSize int, prefix []byte,
) ([]byte, error) {
	capacity := uncompressedSize

	if capacity < 0 {
		capacity = len(src)
	}

	dst := make([]byte, 0, capacity)
	position := 0

	for position < len(src) {
		token := src[position]
		position++

		literalLength, nextPosition, err := lz4Length(src, position, int(token>>4))

		if err != nil {
			return nil, err
		}

		position = nextPosition

		if position+literalLength > len(src) {
			return nil, fmt.Errorf("lz4 literal length exceeds input")
		}

		dst = append(dst, src[position:position+literalLength]...)
		position += literalLength

		if position == len(src) {
			break
		}

		if position+2 > len(src) {
			return nil, fmt.Errorf("lz4 match offset missing")
		}

		offset := int(src[position]) | int(src[position+1])<<8
		position += 2

		if offset <= 0 || offset > len(prefix)+len(dst) {
			return nil, fmt.Errorf("lz4 invalid offset %d", offset)
		}

		matchLength, nextPosition, err := lz4Length(src, position, int(token&0x0f))

		if err != nil {
			return nil, err
		}

		position = nextPosition
		matchLength += 4

		for range matchLength {
			value, err := lz4HistoryByte(prefix, dst, offset)

			if err != nil {
				return nil, err
			}

			dst = append(dst, value)
		}
	}

	if uncompressedSize >= 0 && len(dst) != uncompressedSize {
		return nil, fmt.Errorf("lz4 size %d != %d", len(dst), uncompressedSize)
	}

	return dst, nil
}

func lz4HistoryByte(prefix []byte, dst []byte, offset int) (byte, error) {
	sourceIndex := len(dst) - offset

	if sourceIndex >= 0 {
		return dst[sourceIndex], nil
	}

	prefixIndex := len(prefix) + sourceIndex

	if prefixIndex < 0 || prefixIndex >= len(prefix) {
		return 0, fmt.Errorf("lz4 invalid offset %d", offset)
	}

	return prefix[prefixIndex], nil
}

func decodeLZ4Frame(src []byte, uncompressedSize int) ([]byte, error) {
	if len(src) < 7 {
		return nil, fmt.Errorf("lz4 frame header is truncated")
	}

	if !isLZ4Frame(src) {
		return nil, fmt.Errorf("lz4 frame magic missing")
	}

	position := 4
	flags := src[position]
	position++
	position++ // block descriptor

	if flags&0xc0 != 0x40 {
		return nil, fmt.Errorf("lz4 frame version is unsupported")
	}

	blockIndependent := flags&0x20 != 0
	blockChecksum := flags&0x10 != 0
	contentSize := flags&0x08 != 0
	contentChecksum := flags&0x04 != 0
	dictID := flags&0x01 != 0

	if contentSize {
		if position+8 > len(src) {
			return nil, fmt.Errorf("lz4 frame content size is truncated")
		}

		position += 8
	}

	if dictID {
		if position+4 > len(src) {
			return nil, fmt.Errorf("lz4 frame dictionary id is truncated")
		}

		position += 4
	}

	if position >= len(src) {
		return nil, fmt.Errorf("lz4 frame header checksum is missing")
	}

	position++ // header checksum
	out := make([]byte, 0, uncompressedSize)

	for {
		if position+4 > len(src) {
			return nil, fmt.Errorf("lz4 frame block size is truncated")
		}

		blockSize := binary.LittleEndian.Uint32(src[position : position+4])
		position += 4

		if blockSize == 0 {
			break
		}

		rawBlock := blockSize&lz4FrameBlockRawBit != 0
		blockLength := int(blockSize &^ lz4FrameBlockRawBit)

		if blockLength < 0 || position+blockLength > len(src) {
			return nil, fmt.Errorf("lz4 frame block payload is truncated")
		}

		block := src[position : position+blockLength]
		position += blockLength

		var decoded []byte
		var err error

		if rawBlock {
			decoded = append([]byte(nil), block...)
		} else {
			prefix := out

			if blockIndependent {
				prefix = nil
			}

			decoded, err = decodeLZ4BlockWithPrefix(block, -1, prefix)

			if err != nil {
				return nil, err
			}
		}

		out = append(out, decoded...)

		if !blockChecksum {
			continue
		}

		if position+4 > len(src) {
			return nil, fmt.Errorf("lz4 frame block checksum is truncated")
		}

		position += 4
	}

	if contentChecksum {
		if position+4 > len(src) {
			return nil, fmt.Errorf("lz4 frame content checksum is truncated")
		}

		position += 4
	}

	if position != len(src) {
		return nil, fmt.Errorf("lz4 frame has %d trailing bytes", len(src)-position)
	}

	if len(out) != uncompressedSize {
		return nil, fmt.Errorf("lz4 size %d != %d", len(out), uncompressedSize)
	}

	return out, nil
}

func isLZ4Frame(src []byte) bool {
	return len(src) >= 4 && binary.LittleEndian.Uint32(src[:4]) == lz4FrameMagic
}

func lz4Length(src []byte, position, value int) (int, int, error) {
	if value != 15 {
		return value, position, nil
	}

	for {
		if position >= len(src) {
			return 0, 0, fmt.Errorf("lz4 length extension exceeds input")
		}

		extension := int(src[position])
		position++
		value += extension

		if extension != 255 {
			return value, position, nil
		}
	}
}

func ungroup4(grouped []byte) []byte {
	length := len(grouped)
	base := length / 4
	remainder := length % 4
	sizes := [4]int{base, base, base, base}

	for index := range remainder {
		sizes[index]++
	}

	starts := [4]int{
		0,
		sizes[0],
		sizes[0] + sizes[1],
		sizes[0] + sizes[1] + sizes[2],
	}

	out := make([]byte, length)

	for index := range length {
		group := index % 4
		groupOffset := index / 4
		out[index] = grouped[starts[group]+groupOffset]
	}

	return out
}
