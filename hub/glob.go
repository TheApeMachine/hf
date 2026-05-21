package hub

import (
	"path"
	"regexp"
	"strings"
)

func glob(pattern, filename string) bool {
	pattern = strings.TrimSpace(pattern)

	if pattern == "" {
		return false
	}

	if matchGlobPattern(pattern, filename) {
		return true
	}

	if !strings.Contains(pattern, "/") {
		return matchGlobPattern(pattern, path.Base(filename))
	}

	return false
}

func matchGlobPattern(pattern, filename string) bool {
	re, err := regexp.Compile("^" + globRegexp(pattern) + "$")

	if err != nil {
		return false
	}

	return re.MatchString(filename)
}

func globRegexp(pattern string) string {
	var builder strings.Builder

	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				builder.WriteString(".*")
				index++
				continue
			}

			builder.WriteString("[^/]*")
		case '?':
			builder.WriteString("[^/]")
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[index])))
		}
	}

	return builder.String()
}
