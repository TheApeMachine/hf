package hfconfig

import "fmt"

func seedAxisDimsVariables(variables map[string]any) error {
	rawDims, exists := variables["axes_dims_rope"]

	if !exists {
		return nil
	}

	axisDims, err := parseAxisDims(rawDims)

	if err != nil {
		return err
	}

	if len(axisDims) == 0 || len(axisDims) > 4 {
		return fmt.Errorf("hfconfig: axes_dims_rope must contain 1 to 4 axes, got %d", len(axisDims))
	}

	setRawIfMissing(variables, "rope_axis_count", len(axisDims))

	for axisIndex := 0; axisIndex < 4; axisIndex++ {
		axisDim := 0

		if axisIndex < len(axisDims) {
			axisDim = axisDims[axisIndex]
		}

		setRawIfMissing(variables, fmt.Sprintf("rope_axis_dim_%d", axisIndex), axisDim)
	}

	return nil
}

func parseAxisDims(rawDims any) ([]int, error) {
	switch typedDims := rawDims.(type) {
	case []any:
		return parseAnyAxisDims(typedDims)
	case []int:
		return parseIntAxisDims(typedDims)
	case []int64:
		return parseInt64AxisDims(typedDims)
	case []float64:
		return parseFloat64AxisDims(typedDims)
	default:
		return nil, fmt.Errorf("hfconfig: axes_dims_rope has unsupported type %T", rawDims)
	}
}

func parseAnyAxisDims(rawDims []any) ([]int, error) {
	axisDims := make([]int, 0, len(rawDims))

	for axisIndex, rawDim := range rawDims {
		axisDim, err := parseAxisDim(rawDim)

		if err != nil {
			return nil, fmt.Errorf("hfconfig: axes_dims_rope[%d]: %w", axisIndex, err)
		}

		axisDims = append(axisDims, axisDim)
	}

	return axisDims, nil
}

func parseIntAxisDims(rawDims []int) ([]int, error) {
	axisDims := make([]int, 0, len(rawDims))

	for axisIndex, rawDim := range rawDims {
		if rawDim <= 0 {
			return nil, fmt.Errorf("axis dimension %d must be positive, got %d", axisIndex, rawDim)
		}

		axisDims = append(axisDims, rawDim)
	}

	return axisDims, nil
}

func parseInt64AxisDims(rawDims []int64) ([]int, error) {
	axisDims := make([]int, 0, len(rawDims))

	for axisIndex, rawDim := range rawDims {
		if rawDim <= 0 {
			return nil, fmt.Errorf("axis dimension must be positive, got %d", rawDim)
		}

		if int64(int(rawDim)) != rawDim {
			return nil, fmt.Errorf("axis dimension %d overflows int", axisIndex)
		}

		axisDims = append(axisDims, int(rawDim))
	}

	return axisDims, nil
}

func parseFloat64AxisDims(rawDims []float64) ([]int, error) {
	axisDims := make([]int, 0, len(rawDims))

	for axisIndex, rawDim := range rawDims {
		if rawDim <= 0 || rawDim != float64(int(rawDim)) {
			return nil, fmt.Errorf("axis dimension %d must be a positive integer, got %g", axisIndex, rawDim)
		}

		axisDims = append(axisDims, int(rawDim))
	}

	return axisDims, nil
}

func parseAxisDim(rawDim any) (int, error) {
	switch typedDim := rawDim.(type) {
	case int:
		if typedDim <= 0 {
			return 0, fmt.Errorf("axis dimension must be positive, got %d", typedDim)
		}

		return typedDim, nil
	case int64:
		if typedDim <= 0 {
			return 0, fmt.Errorf("axis dimension must be positive, got %d", typedDim)
		}

		if int64(int(typedDim)) != typedDim {
			return 0, fmt.Errorf("axis dimension overflows int")
		}

		return int(typedDim), nil
	case float64:
		if typedDim <= 0 || typedDim != float64(int(typedDim)) {
			return 0, fmt.Errorf("axis dimension must be a positive integer, got %g", typedDim)
		}

		return int(typedDim), nil
	default:
		return 0, fmt.Errorf("unsupported axis dimension type %T", rawDim)
	}
}

func setRawIfMissing(variables map[string]any, name string, value any) {
	if _, exists := variables[name]; exists {
		return
	}

	variables[name] = value
}
