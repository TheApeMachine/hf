package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/theapemachine/manifesto/dtype"
	"github.com/theapemachine/manifesto/types"
)

/*
Parser walks a safetensors archive index and yields normalized tokens.
*/
type Parser struct {
	archive []byte
}

/*
NewParser constructs a parser over an in-memory safetensors archive.
*/
func NewParser(archive []byte) (*Parser, error) {
	if len(archive) == 0 {
		return nil, fmt.Errorf("safetensors: archive is required")
	}

	return &Parser{archive: archive}, nil
}

/*
Generate returns an iterator over header tokens.
*/
func (parser *Parser) Generate() (iter.Seq[types.Token], error) {
	tokens, err := parser.tokens()

	if err != nil {
		return nil, err
	}

	return func(yield func(types.Token) bool) {
		for _, token := range tokens {
			if !yield(token) {
				return
			}
		}
	}, nil
}

func (parser *Parser) tokens() ([]types.Token, error) {
	if len(parser.archive) < 8 {
		return nil, fmt.Errorf("safetensors: file too small")
	}

	headerLength := binary.LittleEndian.Uint64(parser.archive[:8])

	if uint64(len(parser.archive)) < 8+headerLength {
		return nil, fmt.Errorf("safetensors: truncated header")
	}

	headerBytes := parser.archive[8 : 8+headerLength]

	if len(headerBytes) == 0 || headerBytes[0] != '{' {
		return nil, fmt.Errorf("safetensors: header must begin with '{'")
	}

	fields := make(map[string]json.RawMessage)

	if err := json.Unmarshal(headerBytes, &fields); err != nil {
		return nil, fmt.Errorf("safetensors: parse header: %w", err)
	}

	dataLength := int64(len(parser.archive)) - int64(8+headerLength)
	tokens := make([]types.Token, 0, len(fields))

	for name, rawField := range fields {
		if name == "__metadata__" {
			metadataTokens, err := parser.metadataTokens(rawField)

			if err != nil {
				return nil, err
			}

			tokens = append(tokens, metadataTokens...)

			continue
		}

		token, err := parser.tensorToken(name, rawField, dataLength)

		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}

func (parser *Parser) metadataTokens(raw json.RawMessage) ([]types.Token, error) {
	var metadata map[string]string

	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("safetensors: parse __metadata__: %w", err)
	}

	tokens := make([]types.Token, 0, len(metadata))

	for key, value := range metadata {
		tokens = append(tokens, types.Token{
			Kind:  types.KindMetadata,
			Name:  key,
			Value: value,
		})
	}

	return tokens, nil
}

func (parser *Parser) tensorToken(
	name string,
	raw json.RawMessage,
	dataLength int64,
) (types.Token, error) {
	var entry struct {
		DType       string   `json:"dtype"`
		Shape       []int64  `json:"shape"`
		DataOffsets [2]int64 `json:"data_offsets"`
	}

	if err := json.Unmarshal(raw, &entry); err != nil {
		return types.Token{}, fmt.Errorf("safetensors: parse tensor %q: %w", name, err)
	}

	if entry.DType == "" {
		return types.Token{}, fmt.Errorf("safetensors: tensor %q missing dtype", name)
	}

	precision, err := dtype.Parse(entry.DType)

	if err != nil {
		return types.Token{}, fmt.Errorf("safetensors: tensor %q dtype: %w", name, err)
	}

	if len(entry.Shape) == 0 {
		return types.Token{}, fmt.Errorf("safetensors: tensor %q missing shape", name)
	}

	offset := entry.DataOffsets[0]
	end := entry.DataOffsets[1]

	if offset < 0 || end < offset || end > dataLength {
		return types.Token{}, fmt.Errorf("safetensors: tensor %q offsets out of bounds", name)
	}

	return types.Token{
		Kind:      types.KindTensor,
		Name:      name,
		Shape:     append([]int64(nil), entry.Shape...),
		Precision: precision,
		Span: types.Span{
			Offset: offset,
			Length: end - offset,
		},
	}, nil
}
