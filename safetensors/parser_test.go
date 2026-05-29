package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"iter"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/manifesto/dtype"
	"github.com/theapemachine/manifesto/types"
)

func TestParserGenerate(t *testing.T) {
	convey.Convey("Given a minimal safetensors archive", t, func() {
		archive := minimalArchive(t)

		parser, err := NewParser(archive)
		convey.So(err, convey.ShouldBeNil)

		tokens := collectTokens(parser.Generate())

		convey.Convey("It should yield metadata and tensor tokens", func() {
			convey.So(len(tokens), convey.ShouldEqual, 2)

			metadata := findToken(tokens, types.KindMetadata, "format")
			convey.So(metadata.Value, convey.ShouldEqual, "pt")

			weight := findToken(tokens, types.KindTensor, "weight")
			convey.So(weight.Shape, convey.ShouldResemble, []int64{1})
			convey.So(weight.Precision, convey.ShouldEqual, dtype.Float32)
			convey.So(weight.Span.Offset, convey.ShouldEqual, 0)
			convey.So(weight.Span.Length, convey.ShouldEqual, 4)
		})
	})
}

func TestParserGenerateNamedCheckpointTensors(t *testing.T) {
	convey.Convey("Given a safetensors archive with named tensors", t, func() {
		archive := namedTensorArchive(t)

		parser, err := NewParser(archive)
		convey.So(err, convey.ShouldBeNil)

		tokens := collectTokens(parser.Generate())

		convey.Convey("It should emit checkpoint keys without classification", func() {
			projOut := findToken(tokens, types.KindTensor, "proj_out.weight")
			convey.So(projOut.Shape, convey.ShouldResemble, []int64{128, 3072})
			convey.So(projOut.Precision, convey.ShouldEqual, dtype.Float32)

			norm := findToken(tokens, types.KindTensor, "model.norm.weight")
			convey.So(norm.Shape, convey.ShouldResemble, []int64{2048})
			convey.So(norm.Precision, convey.ShouldEqual, dtype.Float32)
		})
	})
}

func TestNewParserRejectsInvalidArchive(t *testing.T) {
	convey.Convey("Given an empty archive", t, func() {
		_, err := NewParser(nil)
		convey.So(err, convey.ShouldNotBeNil)
	})
}

func BenchmarkParserGenerate(b *testing.B) {
	parser, err := NewParser(namedTensorArchive(b))

	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		count := 0

		for range parser.Generate() {
			count++
		}

		if count == 0 {
			b.Fatal("expected tokens")
		}
	}
}

func minimalArchive(tb testing.TB) []byte {
	tb.Helper()

	header := map[string]any{
		"__metadata__": map[string]string{
			"format": "pt",
		},
		"weight": map[string]any{
			"dtype":        "F32",
			"shape":        []int64{1},
			"data_offsets": []int64{0, 4},
		},
	}

	return marshalArchive(tb, header, 4)
}

func namedTensorArchive(tb testing.TB) []byte {
	tb.Helper()

	header := map[string]any{
		"__metadata__": map[string]string{
			"format": "pt",
		},
		"proj_out.weight": map[string]any{
			"dtype":        "F32",
			"shape":        []int64{128, 3072},
			"data_offsets": []int64{0, 128 * 3072 * 4},
		},
		"model.norm.weight": map[string]any{
			"dtype":        "F32",
			"shape":        []int64{2048},
			"data_offsets": []int64{128 * 3072 * 4, 128*3072*4 + 2048*4},
		},
	}

	return marshalArchive(tb, header, 128*3072*4+2048*4)
}

func marshalArchive(tb testing.TB, header map[string]any, dataLength int) []byte {
	tb.Helper()

	headerBytes, err := json.Marshal(header)

	if err != nil {
		tb.Fatal(err)
	}

	archive := make([]byte, 8+len(headerBytes)+dataLength)
	binary.LittleEndian.PutUint64(archive[:8], uint64(len(headerBytes)))
	copy(archive[8:], headerBytes)

	return archive
}

func findToken(tokens []types.Token, kind types.Kind, name string) types.Token {
	for _, token := range tokens {
		if token.Kind == kind && token.Name == name {
			return token
		}
	}

	return types.Token{}
}

func collectTokens(sequence iter.Seq[types.Token]) []types.Token {
	tokens := make([]types.Token, 0, 4)

	for token := range sequence {
		tokens = append(tokens, token)
	}

	return tokens
}
