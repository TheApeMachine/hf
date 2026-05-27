package program

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/theapemachine/manifesto/dtype/convert"
	"github.com/theapemachine/manifesto/runtime"
	"github.com/theapemachine/manifesto/tensor"
)

/*
WriteImage persists a generated image tensor to disk.
*/
func (host *Host) WriteImage(ctx context.Context, request runtime.WriteImageRequest) error {
	_ = ctx

	values, err := float32ImageValues(request.Tensor)

	if err != nil {
		return err
	}

	width := request.Width
	height := request.Height
	channels := request.Channels

	if width <= 0 || height <= 0 || channels <= 0 {
		return fmt.Errorf("program host: image width, height, and channels are required")
	}

	expectedCount := width * height * channels

	if len(values) < expectedCount {
		return fmt.Errorf(
			"program host: image tensor has %d values, expected %d",
			len(values),
			expectedCount,
		)
	}

	rgba := image.NewRGBA(image.Rect(0, 0, width, height))

	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			red := sampleImageChannel(values, width, height, channels, request.Layout, 0, row, col)
			green := sampleImageChannel(values, width, height, channels, request.Layout, 1, row, col)
			blue := sampleImageChannel(values, width, height, channels, request.Layout, 2, row, col)

			rgba.SetRGBA(
				col,
				row,
				color.RGBA{
					R: encodeImageChannel(red, request.Range),
					G: encodeImageChannel(green, request.Range),
					B: encodeImageChannel(blue, request.Range),
					A: 255,
				},
			)
		}
	}

	outputPath := request.Path

	if outputPath == "" {
		outputPath = "output.png"
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil && filepath.Dir(outputPath) != "." {
		return fmt.Errorf("program host: create image directory: %w", err)
	}

	outputFile, err := os.Create(outputPath)

	if err != nil {
		return fmt.Errorf("program host: create image file: %w", err)
	}

	defer outputFile.Close()

	if err := png.Encode(outputFile, rgba); err != nil {
		return fmt.Errorf("program host: encode png: %w", err)
	}

	return nil
}

func float32ImageValues(value any) ([]float32, error) {
	switch typed := value.(type) {
	case []float32:
		return typed, nil
	case []float64:
		values := make([]float32, len(typed))

		for index, element := range typed {
			values[index] = float32(element)
		}

		return values, nil
	case tensor.Tensor:
		dataType, rawBytes, err := typed.RawBytes()

		if err != nil {
			return nil, fmt.Errorf("program host: read image tensor bytes: %w", err)
		}

		values, err := convert.BytesToFloat32(dataType, rawBytes)

		if err != nil {
			return nil, fmt.Errorf("program host: decode image tensor bytes: %w", err)
		}

		return values, nil
	default:
		return nil, fmt.Errorf("program host: image tensor has unsupported type %T", value)
	}
}

func sampleImageChannel(
	values []float32,
	width int,
	height int,
	channels int,
	layout string,
	channel int,
	row int,
	col int,
) float32 {
	if channel >= channels {
		return 0
	}

	switch layout {
	case "channel_planar", "":
		offset := channel*width*height + row*width + col

		return values[offset]
	default:
		offset := row*width*channels + col*channels + channel

		return values[offset]
	}
}

func encodeImageChannel(value float32, valueRange string) uint8 {
	switch valueRange {
	case "neg_one_one", "":
		scaled := (value + 1) * 0.5 * 255

		if scaled < 0 {
			return 0
		}

		if scaled > 255 {
			return 255
		}

		return uint8(math.Round(float64(scaled)))
	case "zero_one":
		scaled := value * 255

		if scaled < 0 {
			return 0
		}

		if scaled > 255 {
			return 255
		}

		return uint8(math.Round(float64(scaled)))
	default:
		return uint8(math.Round(float64(value)))
	}
}
