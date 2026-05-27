package program

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/manifesto/dtype"
	"github.com/theapemachine/manifesto/dtype/convert"
	"github.com/theapemachine/manifesto/runtime"
	"github.com/theapemachine/manifesto/tensor"
)

func TestHostWriteImage(testingObject *testing.T) {
	convey.Convey("Given WriteImage receives a resident tensor", testingObject, func() {
		shape, err := tensor.NewShape([]int{3, 1, 1})
		convey.So(err, convey.ShouldBeNil)

		values := []float32{-1, 0, 1}
		resident, err := tensor.NewHostBackend().Upload(
			shape,
			dtype.Float32,
			convert.Float32ToBytes(values),
		)
		convey.So(err, convey.ShouldBeNil)
		defer resident.Close()

		outputPath := filepath.Join(testingObject.TempDir(), "image.png")
		host := NewHost(HostOptions{})

		convey.Convey("It should download bytes through the tensor interface and encode PNG", func() {
			err := host.WriteImage(context.Background(), runtime.WriteImageRequest{
				Path:     outputPath,
				Tensor:   resident,
				Width:    1,
				Height:   1,
				Channels: 3,
				Layout:   "channel_planar",
				Range:    "neg_one_one",
			})
			convey.So(err, convey.ShouldBeNil)

			outputFile, err := os.Open(outputPath)
			convey.So(err, convey.ShouldBeNil)
			defer outputFile.Close()

			decoded, err := png.Decode(outputFile)
			convey.So(err, convey.ShouldBeNil)

			red, green, blue, alpha := decoded.At(0, 0).RGBA()
			convey.So(red>>8, convey.ShouldEqual, 0)
			convey.So(green>>8, convey.ShouldEqual, 128)
			convey.So(blue>>8, convey.ShouldEqual, 255)
			convey.So(alpha>>8, convey.ShouldEqual, 255)
		})
	})
}
