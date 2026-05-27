package hfconfig

import (
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestResolveArchitectureLlama(t *testing.T) {
	convey.Convey("Given the registry", t, func() {
		convey.Convey("LlamaForCausalLM resolves to its loader template path", func() {
			assetPath, err := ResolveArchitecture("LlamaForCausalLM")

			convey.So(err, convey.ShouldBeNil)
			convey.So(assetPath, convey.ShouldEqual, "loader/architecture/LlamaForCausalLM.yml")
		})

		convey.Convey("LlamaForCausalLM paged_decode variant resolves to its template path", func() {
			assetPath, err := ResolveArchitecturePath("LlamaForCausalLM", "paged_decode")

			convey.So(err, convey.ShouldBeNil)
			convey.So(assetPath, convey.ShouldEqual, "loader/architecture/LlamaForCausalLM_paged_decode.yml")
		})
	})
}

func TestResolveArchitectureUnknownReturnsHelpfulError(t *testing.T) {
	convey.Convey("Given the registry", t, func() {
		convey.Convey("An unknown architecture returns an error that lists supported ones", func() {
			_, err := ResolveArchitecture("UnknownArchForCausalLM")

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "UnknownArchForCausalLM")
			convey.So(err.Error(), convey.ShouldContainSubstring, "LlamaForCausalLM")
			convey.So(err.Error(), convey.ShouldContainSubstring, "manifesto/asset/template/loader/architecture/")
		})
	})
}

func TestSupportedArchitecturesIsSorted(t *testing.T) {
	convey.Convey("Given the registry", t, func() {
		convey.Convey("SupportedArchitectures returns a sorted list", func() {
			supported := SupportedArchitectures()

			convey.So(len(supported), convey.ShouldBeGreaterThan, 0)

			for index := 1; index < len(supported); index++ {
				convey.So(supported[index-1], convey.ShouldBeLessThan, supported[index])
			}
		})
	})
}

func TestRegisterArchitectureAddsEntry(t *testing.T) {
	convey.Convey("Given RegisterArchitecture", t, func() {
		// Restore after the test to keep the global registry clean.
		originalPath, originalExists := architectureRegistry["TestOnlyArchRegister"]
		defer func() {
			if originalExists {
				architectureRegistry["TestOnlyArchRegister"] = originalPath
			} else {
				delete(architectureRegistry, "TestOnlyArchRegister")
			}
		}()

		RegisterArchitecture("TestOnlyArchRegister", "loader/architecture/TestOnly.yml")

		convey.Convey("The added entry resolves", func() {
			assetPath, err := ResolveArchitecture("TestOnlyArchRegister")

			convey.So(err, convey.ShouldBeNil)
			convey.So(assetPath, convey.ShouldEqual, "loader/architecture/TestOnly.yml")
		})
	})
}

func TestSupportedArchitecturesNamesAreCanonicalHFFormat(t *testing.T) {
	convey.Convey("Given supported architectures", t, func() {
		convey.Convey("All names look like HF class names (CamelCase with no spaces)", func() {
			for _, name := range SupportedArchitectures() {
				convey.So(name, convey.ShouldNotContainSubstring, " ")
				convey.So(name, convey.ShouldNotContainSubstring, "_")
				convey.So(strings.ToUpper(name[:1]), convey.ShouldEqual, name[:1])
			}
		})
	})
}
