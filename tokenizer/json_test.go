package tokenizer

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParse(test *testing.T) {
	Convey("Given BPE tokenizer JSON without an explicit model type", test, func() {
		data := []byte(strings.Replace(
			string(testTokenizerJSON()),
			`"type": "BPE",`,
			"",
			1,
		))

		Convey("It should infer the BPE backend from vocab and merges", func() {
			artifact, err := Parse(data)

			So(err, ShouldBeNil)
			So(artifact.Backend, ShouldEqual, "bytelevel_bpe")
		})
	})

	Convey("Given BPE tokenizer JSON with an NFC normalizer", test, func() {
		data := []byte(strings.Replace(
			string(testTokenizerJSON()),
			`"normalizer": null`,
			`"normalizer": {"type": "NFC"}`,
			1,
		))

		Convey("It should preserve the normalizer on the byte-level tokenizer", func() {
			artifact, err := Parse(data)

			So(err, ShouldBeNil)
			tokenizer, ok := artifact.Tokenizer.(*ByteLevelBPE)
			So(ok, ShouldBeTrue)
			So(tokenizer.normalizer, ShouldEqual, "NFC")
		})
	})
}

func BenchmarkParse(benchmark *testing.B) {
	data := testTokenizerJSON()

	for benchmark.Loop() {
		_, _ = Parse(data)
	}
}
