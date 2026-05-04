//Helper functions to clean queries

package normalizer

import (
	"strings"
	"unicode"
)

type FillerStripper struct {
	phrases [][]string
}

// Initializes Filler Stripper by reading the YAML
func NewFillerStripper(fillers []string) *FillerStripper {
	fs := &FillerStripper{
		phrases: make([][]string, 0, len(fillers)),
	}

	for _, phrase := range fillers {
		normalized := NormalizeBasic(phrase)
		words := strings.Fields(normalized)

		if len(words) == 0 {
			continue
		}

		fs.phrases = append(fs.phrases, words)
	}

	return fs
}

// Helper function to strip whitespace and punctuation
// Performance version inspired from: https://stackoverflow.com/questions/32081808/strip-all-whitespace-from-a-string
func RemovePuncAndWhitespace(query string) string {
	var b strings.Builder
	b.Grow(len(query))

	lastWasSpace := true

	for _, ch := range query {
		if unicode.IsPunct(ch) || unicode.IsSymbol(ch) {
			continue
		}

		if unicode.IsSpace(ch) {
			if lastWasSpace == false {
				b.WriteByte(' ')
				lastWasSpace = true
			}

			continue
		}

		b.WriteRune(ch)
		lastWasSpace = false
	}

	result := b.String()

	if len(result) > 0 && result[len(result)-1] == ' ' {
		return result[:len(result)-1]
	}

	return result

}

// Lowecase, trim whitespace, strip punctuation, collapse spaces
func NormalizeBasic(query string) string {
	lower_and_trimed := RemovePuncAndWhitespace(strings.ToLower(query))
	return lower_and_trimed
}

// Removes filler words like please, could you, I want to, etc
func (fs *FillerStripper) NormalizeQuery(query string) string {
	query = NormalizeBasic(query)
	words := strings.Fields(query)

	if len(words) == 0 {
		return ""
	}

	result := make([]string, 0, len(words))

	for i := 0; i < len(words); {
		matchedLen := fs.longestMatch(words, i)

		if matchedLen > 0 {
			i += matchedLen
			continue
		}

		result = append(result, words[i])
		i++
	}

	return strings.Join(result, " ")
}

func (fs *FillerStripper) longestMatch(words []string, start int) int {
	bestLen := 0

	for _, phrase := range fs.phrases {
		phraseLen := len(phrase)

		if phraseLen <= bestLen {
			continue
		}

		if start+phraseLen > len(words) {
			continue
		}

		if matchesPhrase(words, start, phrase) {
			bestLen = phraseLen
		}
	}

	return bestLen
}

func matchesPhrase(words []string, start int, phrase []string) bool {
	for i := range phrase {
		if words[start+i] != phrase[i] {
			return false
		}
	}

	return true
}
