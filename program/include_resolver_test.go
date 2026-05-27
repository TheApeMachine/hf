package program

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/manifesto/compiler"
	"github.com/theapemachine/manifesto/resolve"
)

func TestIncludeResolverResolveInclude(testingObject *testing.T) {
	convey.Convey("Given an HF include resolver", testingObject, func() {
		convey.Convey("It should open component configs from component directories", func() {
			testHub := &includeResolverHub{
				files: map[string]string{
					"text_encoder/config.json": includeResolverLlamaConfig,
				},
			}

			resolver, err := NewIncludeResolver(IncludeResolverOptions{Hub: testHub})
			convey.So(err, convey.ShouldBeNil)

			blockYAML, err := resolver.ResolveInclude(
				context.Background(),
				compiler.IncludeSource{
					Name:   "text_encoder",
					Source: "hf://owner/model#text_encoder",
				},
			)
			convey.So(err, convey.ShouldBeNil)
			convey.So(string(blockYAML), convey.ShouldContainSubstring, "op: block.model.llama")
			convey.So(testHub.opened, convey.ShouldResemble, []string{
				"text_encoder/config.json",
			})
		})

		convey.Convey("It should keep architecture variants on the root config", func() {
			testHub := &includeResolverHub{
				files: map[string]string{
					"config.json": includeResolverLlamaConfig,
				},
			}

			resolver, err := NewIncludeResolver(IncludeResolverOptions{Hub: testHub})
			convey.So(err, convey.ShouldBeNil)

			_, err = resolver.ResolveInclude(
				context.Background(),
				compiler.IncludeSource{
					Name:   "model",
					Source: "hf://meta-llama/Llama-3.2-1B-Instruct#paged_decode",
				},
			)
			convey.So(err, convey.ShouldBeNil)
			convey.So(testHub.opened, convey.ShouldResemble, []string{"config.json"})
		})
	})
}

type includeResolverHub struct {
	files  map[string]string
	opened []string
}

func (testHub *includeResolverHub) Download(
	ctx context.Context,
	request resolve.DownloadRequest,
) (*resolve.File, error) {
	return nil, fmt.Errorf("test hub: unexpected Download")
}

func (testHub *includeResolverHub) ReadJSON(
	ctx context.Context,
	location resolve.RepoLocation,
	filename string,
	cacheDir string,
	target any,
) error {
	contents, exists := testHub.files[filename]

	if !exists {
		return fmt.Errorf("test hub: missing %s", filename)
	}

	if err := json.Unmarshal([]byte(contents), target); err != nil {
		return fmt.Errorf("test hub: read json %s: %w", filename, err)
	}

	return nil
}

func (testHub *includeResolverHub) Open(
	ctx context.Context,
	location resolve.RepoLocation,
	filename string,
	cacheDir string,
) (io.ReadCloser, *resolve.File, error) {
	testHub.opened = append(testHub.opened, filename)

	contents, exists := testHub.files[filename]

	if !exists {
		return nil, nil, fmt.Errorf("test hub: missing %s", filename)
	}

	return io.NopCloser(strings.NewReader(contents)), &resolve.File{
		Path: filename,
		Size: int64(len(contents)),
	}, nil
}

func (testHub *includeResolverHub) Glob(
	ctx context.Context,
	location resolve.RepoLocation,
	pattern string,
	cacheDir string,
) ([]string, error) {
	return nil, fmt.Errorf("test hub: unexpected Glob")
}

const includeResolverLlamaConfig = `{
  "architectures": ["LlamaForCausalLM"],
  "model_type": "llama",
  "vocab_size": 128256,
  "hidden_size": 2048,
  "intermediate_size": 8192,
  "num_hidden_layers": 2,
  "num_attention_heads": 32,
  "num_key_value_heads": 8,
  "rms_norm_eps": 0.00001,
  "rope_theta": 500000.0
}`
