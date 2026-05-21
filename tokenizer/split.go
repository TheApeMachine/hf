package tokenizer

import (
	"unicode"
	"unicode/utf8"
)

func splitByteLevel(text string) []string {
	tokens := make([]string, 0)
	index := 0

	for index < len(text) {
		nextIndex, token := nextByteLevelToken(text, index)

		if token != "" {
			tokens = append(tokens, token)
		}

		index = nextIndex
	}

	return tokens
}

func nextByteLevelToken(text string, index int) (int, string) {
	if contraction, ok := nextContraction(text, index); ok {
		return index + len(contraction), contraction
	}

	if nextIndex, token, ok := nextLetterToken(text, index); ok {
		return nextIndex, token
	}

	if nextIndex, token, ok := nextNumberToken(text, index); ok {
		return nextIndex, token
	}

	if nextIndex, token, ok := nextPunctuationToken(text, index); ok {
		return nextIndex, token
	}

	if nextIndex, token, ok := nextNewlineToken(text, index); ok {
		return nextIndex, token
	}

	if nextIndex, token, ok := nextTrailingWhitespaceToken(text, index); ok {
		return nextIndex, token
	}

	return nextWhitespaceToken(text, index)
}

func nextLetterToken(text string, index int) (int, string, bool) {
	currentRune, width := utf8.DecodeRuneInString(text[index:])

	if unicode.IsLetter(currentRune) {
		nextIndex := consumeLetters(text, index+width)

		return nextIndex, text[index:nextIndex], true
	}

	if !isLetterPrefix(currentRune) {
		return index, "", false
	}

	nextIndex := index + width

	if nextIndex >= len(text) {
		return index, "", false
	}

	nextRune, nextWidth := utf8.DecodeRuneInString(text[nextIndex:])

	if !unicode.IsLetter(nextRune) {
		return index, "", false
	}

	nextIndex = consumeLetters(text, nextIndex+nextWidth)

	return nextIndex, text[index:nextIndex], true
}

func nextNumberToken(text string, index int) (int, string, bool) {
	currentRune, width := utf8.DecodeRuneInString(text[index:])

	if !unicode.IsNumber(currentRune) {
		return index, "", false
	}

	nextIndex := index + width
	count := 1

	for nextIndex < len(text) && count < 3 {
		nextRune, nextWidth := utf8.DecodeRuneInString(text[nextIndex:])

		if !unicode.IsNumber(nextRune) {
			break
		}

		nextIndex += nextWidth
		count++
	}

	return nextIndex, text[index:nextIndex], true
}

func nextPunctuationToken(text string, index int) (int, string, bool) {
	nextIndex := index

	if text[index] == ' ' {
		nextIndex++
	}

	if nextIndex >= len(text) {
		return index, "", false
	}

	currentRune, width := utf8.DecodeRuneInString(text[nextIndex:])

	if !isPunctuation(currentRune) {
		return index, "", false
	}

	nextIndex += width

	for nextIndex < len(text) {
		nextRune, nextWidth := utf8.DecodeRuneInString(text[nextIndex:])

		if !isPunctuation(nextRune) {
			break
		}

		nextIndex += nextWidth
	}

	nextIndex = consumeNewlines(text, nextIndex)

	return nextIndex, text[index:nextIndex], true
}

func nextNewlineToken(text string, index int) (int, string, bool) {
	currentRune, currentWidth := utf8.DecodeRuneInString(text[index:])

	if !unicode.IsSpace(currentRune) {
		return index, "", false
	}

	nextIndex := index + currentWidth
	lastNewlineEnd := index

	if isNewline(currentRune) {
		lastNewlineEnd = nextIndex
	}

	for nextIndex < len(text) {
		nextRune, nextWidth := utf8.DecodeRuneInString(text[nextIndex:])

		if !unicode.IsSpace(nextRune) {
			break
		}

		nextIndex += nextWidth

		if isNewline(nextRune) {
			lastNewlineEnd = nextIndex
		}
	}

	if lastNewlineEnd == index {
		return index, "", false
	}

	return lastNewlineEnd, text[index:lastNewlineEnd], true
}

func nextTrailingWhitespaceToken(text string, index int) (int, string, bool) {
	currentRune, currentWidth := utf8.DecodeRuneInString(text[index:])

	if !unicode.IsSpace(currentRune) {
		return index, "", false
	}

	nextIndex := index + currentWidth
	lastSpaceStart := index

	for nextIndex < len(text) {
		nextRune, nextWidth := utf8.DecodeRuneInString(text[nextIndex:])

		if !unicode.IsSpace(nextRune) {
			break
		}

		lastSpaceStart = nextIndex
		nextIndex += nextWidth
	}

	if nextIndex >= len(text) {
		return nextIndex, text[index:nextIndex], true
	}

	if lastSpaceStart > index {
		return lastSpaceStart, text[index:lastSpaceStart], true
	}

	return index, "", false
}

func nextWhitespaceToken(text string, index int) (int, string) {
	nextIndex := index

	for nextIndex < len(text) {
		currentRune, width := utf8.DecodeRuneInString(text[nextIndex:])

		if !unicode.IsSpace(currentRune) {
			break
		}

		nextIndex += width
	}

	if nextIndex > index {
		return nextIndex, text[index:nextIndex]
	}

	currentRune, width := utf8.DecodeRuneInString(text[index:])

	return index + width, string(currentRune)
}

func consumeLetters(text string, index int) int {
	for index < len(text) {
		currentRune, width := utf8.DecodeRuneInString(text[index:])

		if !unicode.IsLetter(currentRune) {
			break
		}

		index += width
	}

	return index
}

func consumeNewlines(text string, index int) int {
	for index < len(text) {
		currentRune, width := utf8.DecodeRuneInString(text[index:])

		if !isNewline(currentRune) {
			break
		}

		index += width
	}

	return index
}

func isLetterPrefix(value rune) bool {
	return !isNewline(value) &&
		!unicode.IsLetter(value) &&
		!unicode.IsNumber(value)
}

func isPunctuation(value rune) bool {
	return !unicode.IsSpace(value) &&
		!unicode.IsLetter(value) &&
		!unicode.IsNumber(value)
}

func isNewline(value rune) bool {
	return value == '\n' || value == '\r'
}

func nextContraction(text string, index int) (string, bool) {
	for _, contraction := range []string{
		"'s",
		"'t",
		"'re",
		"'ve",
		"'m",
		"'ll",
		"'d",
	} {
		if hasPrefixFold(text[index:], contraction) {
			return text[index : index+len(contraction)], true
		}
	}

	return "", false
}

func hasPrefixFold(text string, prefix string) bool {
	if len(text) < len(prefix) {
		return false
	}

	for index := range prefix {
		left := rune(text[index])
		right := rune(prefix[index])

		if unicode.ToLower(left) != unicode.ToLower(right) {
			return false
		}
	}

	return true
}
