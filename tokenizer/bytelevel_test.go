package tokenizer

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestByteLevelBPE_Encode(test *testing.T) {
	Convey("Given a byte-level BPE tokenizer", test, func() {
		artifact, err := Parse(testTokenizerJSON())

		So(err, ShouldBeNil)

		tokenizer := artifact.Tokenizer

		Convey("It should encode text into merged token IDs", func() {
			tokenIDs, err := tokenizer.Encode("hello hello!")

			So(err, ShouldBeNil)
			So(tokenIDs, ShouldResemble, []int{7, 8, 9})
		})

		Convey("It should preserve special tokens as atomic IDs", func() {
			tokenIDs, err := tokenizer.Encode("<|endoftext|>hello")

			So(err, ShouldBeNil)
			So(tokenIDs, ShouldResemble, []int{10, 7})
		})

		Convey("It should preserve repeated leading spaces", func() {
			tokenIDs, err := tokenizer.Encode("  hello")

			So(err, ShouldBeNil)
			So(tokenIDs, ShouldResemble, []int{4, 8})
		})

		Convey("It should group newline runs before following text", func() {
			tokenIDs, err := tokenizer.Encode("\n\nhello")

			So(err, ShouldBeNil)
			So(tokenIDs, ShouldResemble, []int{13, 7})
		})
	})
}

func TestByteLevelBPE_EncodeIgnoreMerges(test *testing.T) {
	Convey("Given a byte-level BPE tokenizer that checks vocab before merges", test, func() {
		artifact, err := Parse(ignoreMergesTokenizerJSON())

		So(err, ShouldBeNil)

		tokenizer := artifact.Tokenizer

		Convey("It should emit the full pre-token when it already exists in vocab", func() {
			tokenIDs, err := tokenizer.Encode("cat")

			So(err, ShouldBeNil)
			So(tokenIDs, ShouldResemble, []int{3})
		})
	})
}

func TestByteLevelBPE_Decode(test *testing.T) {
	Convey("Given byte-level BPE token IDs", test, func() {
		artifact, err := Parse(testTokenizerJSON())

		So(err, ShouldBeNil)

		tokenizer := artifact.Tokenizer

		Convey("It should decode IDs back to text", func() {
			text, err := tokenizer.Decode([]int{7, 8, 9}, false)

			So(err, ShouldBeNil)
			So(text, ShouldEqual, "hello hello!")
		})

		Convey("It should optionally skip special tokens", func() {
			text, err := tokenizer.Decode([]int{10, 7}, true)

			So(err, ShouldBeNil)
			So(text, ShouldEqual, "hello")
		})

		Convey("It should expose special token IDs", func() {
			So(tokenizer.SpecialTokenIDs(), ShouldResemble, []int{10})
		})
	})
}

func TestRead(test *testing.T) {
	Convey("Given a tokenizer.json file", test, func() {
		path := filepath.Join(test.TempDir(), "tokenizer.json")
		err := os.WriteFile(path, testTokenizerJSON(), 0o644)

		So(err, ShouldBeNil)

		Convey("It should parse the tokenizer artifact", func() {
			artifact, err := Read(path)

			So(err, ShouldBeNil)
			So(artifact.Backend, ShouldEqual, "bytelevel_bpe")
			So(artifact.Tokenizer.VocabSize(), ShouldEqual, 14)
		})
	})
}

func BenchmarkByteLevelBPE_Encode(benchmark *testing.B) {
	artifact, err := Parse(testTokenizerJSON())

	if err != nil {
		benchmark.Fatal(err)
	}

	for benchmark.Loop() {
		_, _ = artifact.Tokenizer.Encode("hello hello!")
	}
}

func BenchmarkByteLevelBPE_EncodeIgnoreMerges(benchmark *testing.B) {
	artifact, err := Parse(ignoreMergesTokenizerJSON())

	if err != nil {
		benchmark.Fatal(err)
	}

	for benchmark.Loop() {
		_, _ = artifact.Tokenizer.Encode("cat")
	}
}

func testTokenizerJSON() []byte {
	return []byte(`{
  "version": "1.0",
  "normalizer": null,
  "pre_tokenizer": {
    "type": "ByteLevel",
    "add_prefix_space": false,
    "trim_offsets": true
  },
  "model": {
    "type": "BPE",
    "vocab": {
      "h": 0,
      "e": 1,
      "l": 2,
      "o": 3,
      "Ġ": 4,
      "he": 5,
      "hel": 6,
      "hello": 7,
      "Ġhello": 8,
      "!": 9,
      "<|endoftext|>": 10,
      "hell": 11,
      "Ċ": 12,
      "ĊĊ": 13
    },
    "merges": [
      "h e",
      "he l",
      "hel l",
      "hell o",
      "Ġ hello",
      "Ċ Ċ"
    ]
  },
  "decoder": {
    "type": "ByteLevel"
  },
  "added_tokens": [
    {
      "id": 10,
      "content": "<|endoftext|>",
      "single_word": false,
      "lstrip": false,
      "rstrip": false,
      "normalized": false,
      "special": true
    }
  ]
}`)
}

func ignoreMergesTokenizerJSON() []byte {
	return []byte(`{
  "version": "1.0",
  "normalizer": null,
  "pre_tokenizer": {
    "type": "ByteLevel",
    "add_prefix_space": false,
    "trim_offsets": true
  },
  "model": {
    "type": "BPE",
    "ignore_merges": true,
    "vocab": {
      "c": 0,
      "a": 1,
      "t": 2,
      "cat": 3,
      "ca": 4
    },
    "merges": [
      "c a"
    ]
  },
  "decoder": {
    "type": "ByteLevel"
  },
  "added_tokens": []
}`)
}
