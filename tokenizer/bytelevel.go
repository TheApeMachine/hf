package tokenizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var ErrInvalidUTF8 = errors.New("tokenizer: decoded bytes are not valid UTF-8")

type ByteLevelBPE struct {
	vocab             map[string]int
	idToToken         map[int]string
	mergeRanks        map[tokenPair]int
	specialTokenIDs   map[string]int
	specialIDTokens   map[int]string
	specialTokens     []string
	byteToRune        [256]rune
	runeToByte        map[rune]byte
	addPrefixSpace    bool
	trimOffsets       bool
	continuingSubword string
	ignoreMerges      bool
	normalizer        string
}

type tokenPair struct {
	left  string
	right string
}

type byteLevelConfig struct {
	Type           string `json:"type"`
	AddPrefixSpace bool   `json:"add_prefix_space"`
	TrimOffsets    bool   `json:"trim_offsets"`
}

/*
NewByteLevelBPE constructs a GPT-style byte-level BPE tokenizer.
*/
func NewByteLevelBPE(document document) (*ByteLevelBPE, error) {
	if len(document.Model.Vocab) == 0 {
		return nil, fmt.Errorf("tokenizer: BPE vocab is required")
	}

	mergeRanks, err := parseMerges(document.Model.Merges)

	if err != nil {
		return nil, err
	}

	byteToRune, runeToByte := byteTables()
	tokenizer := &ByteLevelBPE{
		vocab:           document.Model.Vocab,
		idToToken:       make(map[int]string, len(document.Model.Vocab)),
		mergeRanks:      mergeRanks,
		specialTokenIDs: make(map[string]int),
		specialIDTokens: make(map[int]string),
		byteToRune:      byteToRune,
		runeToByte:      runeToByte,
		ignoreMerges:    document.Model.IgnoreMerges,
		normalizer:      normalizerType(document.Normalizer),
	}

	for token, tokenID := range document.Model.Vocab {
		tokenizer.idToToken[tokenID] = token
	}

	for _, token := range document.AddedTokens {
		if !token.Special {
			continue
		}

		tokenizer.specialTokenIDs[token.Content] = token.ID
		tokenizer.specialIDTokens[token.ID] = token.Content
		tokenizer.specialTokens = append(tokenizer.specialTokens, token.Content)
	}

	sort.Slice(tokenizer.specialTokens, func(leftIndex, rightIndex int) bool {
		return len(tokenizer.specialTokens[leftIndex]) >
			len(tokenizer.specialTokens[rightIndex])
	})

	tokenizer.configureByteLevel(document.PreTokenizer)

	return tokenizer, nil
}

/*
Encode converts text to token IDs.
*/
func (tokenizer *ByteLevelBPE) Encode(text string) ([]int, error) {
	if tokenizer.normalizer == "NFC" {
		text = norm.NFC.String(text)
	}

	if tokenizer.addPrefixSpace && text != "" && !startsWithSpace(text) {
		text = " " + text
	}

	segments := tokenizer.segmentSpecialTokens(text)
	tokenIDs := make([]int, 0, len(text))

	for _, segment := range segments {
		if segment.special {
			tokenID, ok := tokenizer.specialTokenIDs[segment.text]

			if !ok {
				return nil, fmt.Errorf("tokenizer: unknown special token %q", segment.text)
			}

			tokenIDs = append(tokenIDs, tokenID)

			continue
		}

		encoded, err := tokenizer.encodeOrdinary(segment.text)

		if err != nil {
			return nil, err
		}

		tokenIDs = append(tokenIDs, encoded...)
	}

	return tokenIDs, nil
}

/*
Decode converts token IDs back to text.
*/
func (tokenizer *ByteLevelBPE) Decode(
	tokenIDs []int, skipSpecialTokens bool,
) (string, error) {
	var encoded strings.Builder

	for _, tokenID := range tokenIDs {
		if specialToken, ok := tokenizer.specialIDTokens[tokenID]; ok {
			if !skipSpecialTokens {
				encoded.WriteString(specialToken)
			}

			continue
		}

		token, ok := tokenizer.idToToken[tokenID]

		if !ok {
			return "", fmt.Errorf("tokenizer: unknown token id %d", tokenID)
		}

		encoded.WriteString(token)
	}

	return tokenizer.decodeByteLevel(encoded.String())
}

/*
VocabSize returns the tokenizer vocabulary size.
*/
func (tokenizer *ByteLevelBPE) VocabSize() int {
	return len(tokenizer.vocab)
}

/*
SpecialTokenIDs returns tokenizer-level special token IDs.
*/
func (tokenizer *ByteLevelBPE) SpecialTokenIDs() []int {
	tokenIDs := make([]int, 0, len(tokenizer.specialIDTokens))

	for tokenID := range tokenizer.specialIDTokens {
		tokenIDs = append(tokenIDs, tokenID)
	}

	sort.Ints(tokenIDs)

	return tokenIDs
}

func (tokenizer *ByteLevelBPE) encodeOrdinary(text string) ([]int, error) {
	preTokens := splitByteLevel(text)
	tokenIDs := make([]int, 0, len(preTokens))

	for _, preToken := range preTokens {
		byteEncoded := tokenizer.encodeBytes(preToken)

		if tokenizer.ignoreMerges {
			if tokenID, ok := tokenizer.vocab[byteEncoded]; ok {
				tokenIDs = append(tokenIDs, tokenID)

				continue
			}
		}

		pieces := tokenizer.applyBPE(byteEncoded)

		for _, piece := range pieces {
			tokenID, ok := tokenizer.vocab[piece]

			if !ok {
				return nil, fmt.Errorf("tokenizer: token %q is not in vocab", piece)
			}

			tokenIDs = append(tokenIDs, tokenID)
		}
	}

	return tokenIDs, nil
}

func (tokenizer *ByteLevelBPE) encodeBytes(text string) string {
	var builder strings.Builder

	for _, value := range []byte(text) {
		builder.WriteRune(tokenizer.byteToRune[value])
	}

	return builder.String()
}

func (tokenizer *ByteLevelBPE) decodeByteLevel(text string) (string, error) {
	bytes := make([]byte, 0, len(text))

	for _, encodedRune := range text {
		value, ok := tokenizer.runeToByte[encodedRune]

		if !ok {
			bytes = append(bytes, string(encodedRune)...)
			continue
		}

		bytes = append(bytes, value)
	}

	if !utf8.Valid(bytes) {
		return "", ErrInvalidUTF8
	}

	return string(bytes), nil
}

func (tokenizer *ByteLevelBPE) applyBPE(token string) []string {
	pieces := runePieces(token)

	for {
		bestIndex := -1
		bestRank := len(tokenizer.mergeRanks) + 1

		for index := 0; index < len(pieces)-1; index++ {
			pair := tokenPair{
				left:  pieces[index],
				right: pieces[index+1],
			}
			rank, ok := tokenizer.mergeRanks[pair]

			if !ok || rank >= bestRank {
				continue
			}

			bestIndex = index
			bestRank = rank
		}

		if bestIndex < 0 {
			return pieces
		}

		merged := pieces[bestIndex] + pieces[bestIndex+1]
		next := make([]string, 0, len(pieces)-1)
		next = append(next, pieces[:bestIndex]...)
		next = append(next, merged)
		next = append(next, pieces[bestIndex+2:]...)
		pieces = next
	}
}

type textSegment struct {
	text    string
	special bool
}

func (tokenizer *ByteLevelBPE) segmentSpecialTokens(text string) []textSegment {
	if len(tokenizer.specialTokens) == 0 || text == "" {
		return []textSegment{{text: text}}
	}

	segments := make([]textSegment, 0)
	start := 0
	index := 0

	for index < len(text) {
		matched := ""

		for _, specialToken := range tokenizer.specialTokens {
			if strings.HasPrefix(text[index:], specialToken) {
				matched = specialToken
				break
			}
		}

		if matched == "" {
			_, width := utf8.DecodeRuneInString(text[index:])
			index += width
			continue
		}

		if start < index {
			segments = append(segments, textSegment{text: text[start:index]})
		}

		segments = append(segments, textSegment{text: matched, special: true})
		index += len(matched)
		start = index
	}

	if start < len(text) {
		segments = append(segments, textSegment{text: text[start:]})
	}

	return segments
}

func (tokenizer *ByteLevelBPE) configureByteLevel(raw json.RawMessage) {
	config, ok := findByteLevelConfig(raw)

	if !ok {
		return
	}

	tokenizer.addPrefixSpace = config.AddPrefixSpace
	tokenizer.trimOffsets = config.TrimOffsets
}

func parseMerges(raw json.RawMessage) (map[tokenPair]int, error) {
	var stringMerges []string

	if err := json.Unmarshal(raw, &stringMerges); err == nil {
		return rankStringMerges(stringMerges)
	}

	var tupleMerges [][]string

	if err := json.Unmarshal(raw, &tupleMerges); err == nil {
		merges := make([]string, 0, len(tupleMerges))

		for _, merge := range tupleMerges {
			if len(merge) != 2 {
				return nil, fmt.Errorf("tokenizer: BPE merge tuple must have length 2")
			}

			merges = append(merges, merge[0]+" "+merge[1])
		}

		return rankStringMerges(merges)
	}

	return nil, fmt.Errorf("tokenizer: BPE merges must be strings or pairs")
}

func rankStringMerges(merges []string) (map[tokenPair]int, error) {
	ranks := make(map[tokenPair]int, len(merges))

	for rank, merge := range merges {
		parts := strings.Split(merge, " ")

		if len(parts) != 2 {
			return nil, fmt.Errorf("tokenizer: invalid BPE merge %q", merge)
		}

		ranks[tokenPair{left: parts[0], right: parts[1]}] = rank
	}

	return ranks, nil
}

func findByteLevelConfig(raw json.RawMessage) (byteLevelConfig, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return byteLevelConfig{}, false
	}

	var typed struct {
		Type          string            `json:"type"`
		PreTokenizers []json.RawMessage `json:"pretokenizers"`
		PreTokenizer  []json.RawMessage `json:"pre_tokenizers"`
	}

	if err := json.Unmarshal(raw, &typed); err != nil {
		return byteLevelConfig{}, false
	}

	if typed.Type == "ByteLevel" {
		var config byteLevelConfig

		if err := json.Unmarshal(raw, &config); err != nil {
			return byteLevelConfig{}, false
		}

		return config, true
	}

	for _, child := range append(typed.PreTokenizers, typed.PreTokenizer...) {
		config, ok := findByteLevelConfig(child)

		if ok {
			return config, true
		}
	}

	return byteLevelConfig{}, false
}

func runePieces(text string) []string {
	pieces := make([]string, 0, len(text))

	for _, piece := range text {
		pieces = append(pieces, string(piece))
	}

	return pieces
}

func startsWithSpace(text string) bool {
	firstRune, _ := utf8.DecodeRuneInString(text)

	return unicode.IsSpace(firstRune)
}
