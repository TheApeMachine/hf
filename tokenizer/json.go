package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type document struct {
	Version      string          `json:"version"`
	Normalizer   json.RawMessage `json:"normalizer"`
	PreTokenizer json.RawMessage `json:"pre_tokenizer"`
	Model        modelDocument   `json:"model"`
	Decoder      json.RawMessage `json:"decoder"`
	AddedTokens  []addedToken    `json:"added_tokens"`
}

type modelDocument struct {
	Type         string          `json:"type"`
	Vocab        map[string]int  `json:"vocab"`
	Merges       json.RawMessage `json:"merges"`
	IgnoreMerges bool            `json:"ignore_merges"`
}

type addedToken struct {
	ID      int    `json:"id"`
	Content string `json:"content"`
	Special bool   `json:"special"`
}

/*
Read parses a Hugging Face tokenizer.json file.
*/
func Read(path string) (*Artifact, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("tokenizer: read %s: %w", path, err)
	}

	return Parse(data)
}

/*
Parse creates a tokenizer artifact from tokenizer.json bytes.
*/
func Parse(data []byte) (*Artifact, error) {
	var document document

	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("tokenizer: parse tokenizer.json: %w", err)
	}

	if err := validateNormalizer(document.Normalizer); err != nil {
		return nil, err
	}

	switch document.Model.TypeName() {
	case "BPE":
		tokenizer, err := NewByteLevelBPE(document)

		if err != nil {
			return nil, err
		}

		return &Artifact{
			Backend:   "bytelevel_bpe",
			Tokenizer: tokenizer,
		}, nil
	default:
		return nil, fmt.Errorf(
			"tokenizer: model type %q is not supported",
			document.Model.Type,
		)
	}
}

func (model modelDocument) TypeName() string {
	modelType := strings.TrimSpace(model.Type)

	if modelType != "" {
		return modelType
	}

	if len(model.Vocab) > 0 && model.HasMerges() {
		return "BPE"
	}

	return ""
}

func (model modelDocument) HasMerges() bool {
	merges := strings.TrimSpace(string(model.Merges))

	return merges != "" && merges != "null" && merges != "[]"
}

func validateNormalizer(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var typed struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(raw, &typed); err != nil {
		return fmt.Errorf("tokenizer: parse normalizer: %w", err)
	}

	if typed.Type == "" {
		return nil
	}

	if typed.Type == "NFC" {
		return nil
	}

	return fmt.Errorf("tokenizer: normalizer %q is not supported", typed.Type)
}

func normalizerType(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var typed struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(raw, &typed); err != nil {
		return ""
	}

	return typed.Type
}
