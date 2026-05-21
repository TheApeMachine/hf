package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSplitByteLevel(test *testing.T) {
	Convey("Given a Llama byte-level split pattern", test, func() {
		Convey("It should attach a single leading space to letter runs", func() {
			So(splitByteLevel("hello hello"), ShouldResemble, []string{
				"hello",
				" hello",
			})
		})

		Convey("It should leave repeated leading spaces available to BPE", func() {
			So(splitByteLevel("  hello"), ShouldResemble, []string{
				" ",
				" hello",
			})
		})

		Convey("It should split number runs into groups of three", func() {
			So(splitByteLevel("1234567"), ShouldResemble, []string{
				"123",
				"456",
				"7",
			})
		})

		Convey("It should attach one leading space to punctuation runs", func() {
			So(splitByteLevel(" --\n\nhello"), ShouldResemble, []string{
				" --\n\n",
				"hello",
			})
		})

		Convey("It should split whitespace through the last newline", func() {
			So(splitByteLevel(" \n  hello"), ShouldResemble, []string{
				" \n",
				" ",
				" hello",
			})
		})

		Convey("It should isolate case-insensitive contractions", func() {
			So(splitByteLevel("DON'T"), ShouldResemble, []string{
				"DON",
				"'T",
			})
		})

		Convey("It should preserve trailing whitespace as a run", func() {
			So(splitByteLevel("hello   "), ShouldResemble, []string{
				"hello",
				"   ",
			})
		})
	})
}

func BenchmarkSplitByteLevel(benchmark *testing.B) {
	prompt := "<|begin_of_text|><|start_header_id|>system<|end_header_id|>\n\n" +
		"Cutting Knowledge Date: December 2023\nToday Date: 26 Jul 2024\n\n" +
		"<|eot_id|><|start_header_id|>user<|end_header_id|>\n\n" +
		"Tell me a story about SIMD kernels in 2026.<|eot_id|>" +
		"<|start_header_id|>assistant<|end_header_id|>\n\n"

	for benchmark.Loop() {
		_ = splitByteLevel(prompt)
	}
}
