package safetensors

import (
	"encoding/binary"
	"encoding/json"
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

		sequence, err := parser.Generate()
		convey.So(err, convey.ShouldBeNil)

		tokens := collectTokens(sequence)

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

func minimalArchive(t *testing.T) []byte {
	t.Helper()

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

	headerBytes, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}

	archive := make([]byte, 8+len(headerBytes)+4)
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

func collectTokens(sequence func(yield func(types.Token) bool)) []types.Token {
	tokens := make([]types.Token, 0, 4)

	for token := range sequence {
		tokens = append(tokens, token)
	}

	return tokens
}
