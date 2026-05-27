package tokenizer

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetadataApplyChatTemplate(test *testing.T) {
	Convey("Given Llama chat metadata", test, func() {
		metadata := &Metadata{
			ChatTemplate: "{{ bos_token }}<|start_header_id|>{{ role }}<|end_header_id|>",
			BOSToken:     "<|begin_of_text|>",
			EOTToken:     "<|eot_id|>",
		}

		Convey("It should render the user message and assistant generation prompt", func() {
			text, err := metadata.ApplyChatTemplate(" hello ")
			todayDate := time.Now().Format("2 Jan 2006")
			expected := "<|begin_of_text|><|start_header_id|>system<|end_header_id|>\n\n" +
				"Cutting Knowledge Date: December 2023\nToday Date: " + todayDate + "\n\n" +
				"<|eot_id|><|start_header_id|>user<|end_header_id|>\n\n" +
				"hello<|eot_id|><|start_header_id|>assistant<|end_header_id|>\n\n"

			So(err, ShouldBeNil)
			So(text, ShouldEqual, expected)
		})

		Convey("It should render a continuation without a new beginning token", func() {
			text, err := metadata.ApplyChatContinuation("what?")

			So(err, ShouldBeNil)
			So(text, ShouldEqual, "<|eot_id|><|start_header_id|>user<|end_header_id|>\n\nwhat?<|eot_id|><|start_header_id|>assistant<|end_header_id|>\n\n")
		})
	})

	Convey("Given metadata without a chat template", test, func() {
		metadata := &Metadata{}

		Convey("It should leave text unchanged", func() {
			text, err := metadata.ApplyChatTemplate("hello")

			So(err, ShouldBeNil)
			So(text, ShouldEqual, "hello")
		})
	})
}
