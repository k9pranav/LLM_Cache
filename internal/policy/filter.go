package policy

import (
	"strings"

	"github.com/k9pranav/LLM_Cache/internal/normalizer"
	"github.com/k9pranav/LLM_Cache/pkg/types"
)

type Policy struct {
	min_char   int
	min_tokens int
	phrases    [][]string
}

func NewPolicy(min_char int, min_tokens int, hedges []string) *Policy {
	p := &Policy{
		min_char:   min_char,
		min_tokens: min_tokens,
		phrases:    make([][]string, 0, len(hedges)),
	}

	for _, phrase := range hedges {
		normalized := normalizer.NormalizeBasic(phrase) // I'm not sure -> im not sure
		words := strings.Fields(normalized)             //im not sure -> {'im', 'not', 'sure'}

		if len(words) == 0 {
			continue
		}

		p.phrases = append(p.phrases, words)
	}

	return p

}

func (p *Policy) ShouldCache(resp types.LLMResponse) bool {
	content := strings.TrimSpace(resp.Content) //Only getting the content from the LLM response

	if content == "" {
		return false
	}

	if p.isTooShort(content) {
		return false
	}

	if p.containsHedging(content) {
		return false
	}

	if resp.FinishReason == "content_filter" || resp.FinishReason == "error" {
		return false
	}

	return true
}

func (p *Policy) isTooShort(query string) bool {
	if len([]rune(query)) < p.min_char || len(strings.Fields(query)) < p.min_tokens {
		return true
	} else {
		return false
	}

}

func (p *Policy) containsHedging(query string) bool {
	normalized := normalizer.NormalizeBasic(query)
	words := strings.Fields(normalized)

	if len(words) == 0 {
		return false
	}

	for i := 0; i < len(words); i++ {
		for _, phrase := range p.phrases {
			if len(phrase) == 0 {
				continue
			}

			if i+len(phrase) > len(words) {
				continue
			}

			if matchesPhrase(words, i, phrase) {
				return true
			}
		}
	}

	return false
}

func matchesPhrase(words []string, start int, phrase []string) bool {
	for i := range phrase {
		if words[start+i] != phrase[i] {
			return false
		}
	}

	return true
}
