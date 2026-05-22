package program

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestHostReadLine(testingObject *testing.T) {
	convey.Convey("Given host stdin with multiple buffered lines", testingObject, func() {
		host := NewHost(HostOptions{
			Stdin: strings.NewReader("hello\nwhat?\n"),
		})

		convey.Convey("It should preserve unread buffered input between calls", func() {
			first, err := host.ReadLine(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(first, convey.ShouldEqual, "hello")

			second, err := host.ReadLine(context.Background())
			convey.So(err, convey.ShouldBeNil)
			convey.So(second, convey.ShouldEqual, "what?")

			_, err = host.ReadLine(context.Background())
			convey.So(err, convey.ShouldEqual, io.EOF)
		})
	})
}

func TestHostLoadMetadata(testingObject *testing.T) {
	convey.Convey("Given tokenizer metadata loaded by the program host", testingObject, func() {
		directory := testingObject.TempDir()
		writeMetadataFile(testingObject, directory, "tokenizer_config.json", `{
			"chat_template": "{{ bos_token }}<|start_header_id|>{{ role }}<|end_header_id|>",
			"bos_token": "<|begin_of_text|>",
			"eos_token": "<|eot_id|>"
		}`)
		writeMetadataFile(testingObject, directory, "special_tokens_map.json", `{
			"eos_token": "<|eot_id|>"
		}`)

		host := NewHost(HostOptions{})

		convey.Convey("It should reuse the metadata after the first resolve", func() {
			first, err := host.loadMetadata(context.Background(), directory, "")
			convey.So(err, convey.ShouldBeNil)
			convey.So(first.EOTToken, convey.ShouldEqual, "<|eot_id|>")

			err = os.Remove(filepath.Join(directory, "tokenizer_config.json"))
			convey.So(err, convey.ShouldBeNil)

			second, err := host.loadMetadata(context.Background(), directory, "")
			convey.So(err, convey.ShouldBeNil)
			convey.So(second, convey.ShouldEqual, first)
		})
	})
}

func writeMetadataFile(
	testingObject *testing.T,
	directory string,
	name string,
	content string,
) {
	testingObject.Helper()

	err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600)

	if err != nil {
		testingObject.Fatal(err)
	}
}
