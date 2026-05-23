package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPadTokenIDFromConfig(test *testing.T) {
	Convey("Given Klein tokenizer_config metadata", test, func() {
		tokenizerConfig := tokenizerConfigDocument{
			PadToken:       "<|endoftext|>",
			ModelMaxLength: 131072,
			AddedTokens: map[string]addedTokenSpec{
				"151643": {Content: "<|endoftext|>"},
			},
		}

		Convey("It should resolve pad_token_id from added_tokens_decoder", func() {
			So(padTokenIDFromConfig(tokenizerConfig), ShouldEqual, 151643)
		})
	})
}
