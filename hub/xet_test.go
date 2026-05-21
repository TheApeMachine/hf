package hub

import (
	"bytes"
	"encoding/binary"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecodeXorbRange(test *testing.T) {
	Convey("Given an uncompressed xorb range", test, func() {
		data := bytes.Join([][]byte{
			xorbChunk(0, []byte("hello ")),
			xorbChunk(0, []byte("world")),
		}, nil)

		Convey("It should return chunks indexed by their xorb position", func() {
			chunks, err := DecodeXorbRange(data, 3, 5)

			So(err, ShouldBeNil)
			So(chunks[3], ShouldResemble, []byte("hello "))
			So(chunks[4], ShouldResemble, []byte("world"))
		})
	})
}

func TestDecodeLZ4Block(test *testing.T) {
	Convey("Given an LZ4 literal-only block", test, func() {
		out, err := decodeLZ4Block([]byte{0x30, 'a', 'b', 'c'}, 3)

		Convey("It should decode the literal bytes", func() {
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []byte("abc"))
		})
	})

	Convey("Given an LZ4 block with an overlapping match", test, func() {
		out, err := decodeLZ4Block([]byte{0x11, 'a', 0x01, 0x00}, 6)

		Convey("It should copy from the decoded window", func() {
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []byte("aaaaaa"))
		})
	})

	Convey("Given an LZ4 frame payload", test, func() {
		frame := lz4Frame([]byte{0x30, 'a', 'b', 'c'})
		out, err := decodeLZ4Payload(frame, 3)

		Convey("It should decode framed blocks instead of treating magic bytes as a raw block", func() {
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []byte("abc"))
		})
	})
}

func TestDecodeXorbRange_LZ4Frame(test *testing.T) {
	Convey("Given a framed LZ4 xorb chunk", test, func() {
		data := xorbChunk(1, lz4Frame([]byte{0x30, 'a', 'b', 'c'}), 3)

		Convey("It should decode the frame payload", func() {
			chunks, err := DecodeXorbRange(data, 0, 1)

			So(err, ShouldBeNil)
			So(chunks[0], ShouldResemble, []byte("abc"))
		})
	})
}

func BenchmarkDecodeXorbRange(benchmark *testing.B) {
	data := xorbChunk(0, bytes.Repeat([]byte("a"), 64*1024))

	for benchmark.Loop() {
		_, _ = DecodeXorbRange(data, 0, 1)
	}
}

func xorbChunk(compression byte, payload []byte, unpacked ...int) []byte {
	uncompressedSize := len(payload)

	if len(unpacked) > 0 {
		uncompressedSize = unpacked[0]
	}

	header := []byte{
		0,
		byte(len(payload)),
		byte(len(payload) >> 8),
		byte(len(payload) >> 16),
		compression,
		byte(uncompressedSize),
		byte(uncompressedSize >> 8),
		byte(uncompressedSize >> 16),
	}

	return append(header, payload...)
}

func lz4Frame(block []byte) []byte {
	frame := []byte{
		0x04, 0x22, 0x4d, 0x18, // magic
		0x60, // version + independent blocks
		0x70, // max block size
		0x00, // ignored header checksum
	}
	var size [4]byte

	binary.LittleEndian.PutUint32(size[:], uint32(len(block)))
	frame = append(frame, size[:]...)
	frame = append(frame, block...)
	frame = append(frame, 0, 0, 0, 0)

	return frame
}
