package safetensors

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestOperationLookupSplitNodeParam(t *testing.T) {
	convey.Convey("Given an operation lookup table", t, func() {
		operationLookup := NewOperationLookup()

		convey.Convey("It should split checkpoint tensor keys", func() {
			nodeName, paramSuffix, ok := operationLookup.SplitNodeParam("x_embedder.weight")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(nodeName, convey.ShouldEqual, "x_embedder")
			convey.So(paramSuffix, convey.ShouldEqual, ".weight")
		})
	})
}

func BenchmarkOperationLookupResolve(b *testing.B) {
	operationLookup := NewOperationLookup()

	for b.Loop() {
		operationLookup.Resolve("transformer_blocks.0.attn.norm_q", 1)
	}
}
