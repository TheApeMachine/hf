package hfconfig

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSeedAxisDimsVariables(testingObject *testing.T) {
	convey.Convey("Given HF rotary axis dimensions", testingObject, func() {
		variables := map[string]any{
			"axes_dims_rope": []any{float64(32), float64(32), float64(32), float64(32)},
		}

		err := seedAxisDimsVariables(variables)

		convey.Convey("It should expose manifest variables derived from the HF config", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(variables["rope_axis_count"], convey.ShouldEqual, 4)
			convey.So(variables["rope_axis_dim_0"], convey.ShouldEqual, 32)
			convey.So(variables["rope_axis_dim_1"], convey.ShouldEqual, 32)
			convey.So(variables["rope_axis_dim_2"], convey.ShouldEqual, 32)
			convey.So(variables["rope_axis_dim_3"], convey.ShouldEqual, 32)
		})
	})
}

func TestSeedAxisDimsVariablesRejectsInvalidAxis(testingObject *testing.T) {
	convey.Convey("Given a malformed HF rotary axis dimension", testingObject, func() {
		variables := map[string]any{
			"axes_dims_rope": []any{float64(32), float64(31.5)},
		}

		err := seedAxisDimsVariables(variables)

		convey.Convey("It should return a configuration error", func() {
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "axes_dims_rope[1]")
		})
	})
}
