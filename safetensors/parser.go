package safetensors

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/theapemachine/manifesto/dtype"
	"github.com/theapemachine/manifesto/types"
)

var _ types.Parser = (*Parser)(nil)

/*
Parser walks a safetensors archive index and yields normalized tokens.
It implements manifesto/types.Parser and performs no operation or role
classification — checkpoint keys are emitted as-is for downstream
topology binding.
*/
type Parser struct {
	tokens []types.Token
}

/*
NewParser parses a safetensors archive header and constructs a token stream.
*/
func NewParser(archive []byte) (*Parser, error) {
	tokens, err := parseArchive(archive)

	if err != nil {
		return nil, err
	}

	return &Parser{tokens: tokens}, nil
}

/*
Generate yields every metadata and tensor token from the archive index.
*/
func (parser *Parser) Generate() iter.Seq[types.Token] {
	return func(yield func(types.Token) bool) {
		if parser == nil {
			return
		}

		for _, token := range parser.tokens {
			if !yield(token) {
				return
			}
		}
	}
}

func parseArchive(archive []byte) ([]types.Token, error) {
	if len(archive) == 0 {
		return nil, fmt.Errorf("safetensors: archive is required")
	}

	if len(archive) < 8 {
		return nil, fmt.Errorf("safetensors: file too small")
	}

	headerLength := binary.LittleEndian.Uint64(archive[:8])

	if uint64(len(archive)) < 8+headerLength {
		return nil, fmt.Errorf("safetensors: truncated header")
	}

	headerBytes := archive[8 : 8+headerLength]

	if len(headerBytes) == 0 || headerBytes[0] != '{' {
		return nil, fmt.Errorf("safetensors: header must begin with '{'")
	}

	fields := make(map[string]json.RawMessage)

	if err := json.Unmarshal(headerBytes, &fields); err != nil {
		return nil, fmt.Errorf("safetensors: parse header: %w", err)
	}

	dataLength := int64(len(archive)) - int64(8+headerLength)
	tokens := make([]types.Token, 0, len(fields))

	for name, rawField := range fields {
		if name == "__metadata__" {
			metadataTokens, err := metadataTokens(rawField)

			if err != nil {
				return nil, err
			}

			tokens = append(tokens, metadataTokens...)

			continue
		}

		token, skip, err := tensorToken(name, rawField, dataLength)

		if err != nil {
			return nil, err
		}

		if skip {
			continue
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}

func metadataTokens(raw json.RawMessage) ([]types.Token, error) {
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

func tensorToken(
	name string,
	raw json.RawMessage,
	dataLength int64,
) (types.Token, bool, error) {
	var entry struct {
		DType       string   `json:"dtype"`
		Shape       []int64  `json:"shape"`
		DataOffsets [2]int64 `json:"data_offsets"`
	}

	if err := json.Unmarshal(raw, &entry); err != nil {
		return types.Token{}, false, fmt.Errorf("safetensors: parse tensor %q: %w", name, err)
	}

	if entry.DType == "" {
		return types.Token{}, false, fmt.Errorf("safetensors: tensor %q missing dtype", name)
	}

	precision, err := dtype.Parse(entry.DType)

	if err != nil {
		return types.Token{}, false, fmt.Errorf("safetensors: tensor %q dtype: %w", name, err)
	}

	if len(entry.Shape) == 0 {
		return types.Token{}, true, nil
	}

	offset := entry.DataOffsets[0]
	end := entry.DataOffsets[1]

	if offset < 0 || end < offset || end > dataLength {
		return types.Token{}, false, fmt.Errorf("safetensors: tensor %q offsets out of bounds", name)
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
	}, false, nil
}
